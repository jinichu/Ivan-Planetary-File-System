package server

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"log"
	"net"
	"proj2_f5w9a_h6v9a_q7w9a_r8u8_w1c0b/config"
	"proj2_f5w9a_h6v9a_q7w9a_r8u8_w1c0b/serverpb"
	"strconv"
	"time"

	"github.com/fatih/color"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

const (
	dialTimeout       = 2 * time.Second
	heartBeatInterval = 2 * time.Second
)

func (s *Server) Hello(ctx context.Context, req *serverpb.HelloRequest) (*serverpb.HelloResponse, error) {
	meta, err := s.NodeMeta()
	if err != nil {
		return nil, err
	}

	resp := serverpb.HelloResponse{
		Meta: &meta,
	}

	reqMeta := req.GetMeta()
	if reqMeta == nil {
		return nil, errors.Errorf("Meta field required")
	}
	if err := s.AddNode(*reqMeta); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for id := range s.mu.peers {
		meta := s.mu.peerMeta[id]
		resp.ConnectedPeers = append(resp.ConnectedPeers, &meta)
	}
	for id, meta := range s.mu.peerMeta {
		if _, ok := s.mu.peers[id]; ok {
			continue
		}
		meta := meta
		resp.KnownPeers = append(resp.KnownPeers, &meta)
	}

	return &resp, nil
}

func (s *Server) connectNode(ctx context.Context, meta serverpb.NodeMeta) (*grpc.ClientConn, error) {
	roots := x509.NewCertPool()
	ok := roots.AppendCertsFromPEM([]byte(meta.Cert))
	if !ok {
		return nil, errors.Errorf("failed to parse certificate for node %+v", meta)
	}

	creds := credentials.NewClientTLSFromCert(roots, "")
	var err error
	var conn *grpc.ClientConn
	for _, addr := range meta.Addrs {
		if err := validateAddr(addr); err != nil {
			return nil, err
		}
		ctx, _ := context.WithTimeout(ctx, dialTimeout)
		conn, err = grpc.DialContext(ctx, addr,
			grpc.WithTransportCredentials(creds),
			grpc.WithBlock(),
			grpc.WithDefaultCallOptions(
				grpc.MaxCallRecvMsgSize(int(config.GRPCMsgSize)),
				grpc.MaxCallSendMsgSize(int(config.GRPCMsgSize)),
			),
		)
		if err != nil {
			s.log.Printf("error dialing %+v: %+v", addr, err)
			continue
		}
		break
	}
	if err != nil {
		return nil, errors.Wrapf(err, "dialing %s", meta.Id)
	}
	return conn, nil
}

// getOutboundIP sets up a UDP connection (but doesn't send anything) and uses
// the local IP addressed assigned.
func getOutboundIP() net.IP {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	return conn.LocalAddr().(*net.UDPAddr).IP
}

func (s *Server) NumConnections() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return len(s.mu.peers)
}

// AddNode adds a node to the server.
func (s *Server) AddNode(meta serverpb.NodeMeta) error {
	localMeta, err := s.NodeMeta()
	if err != nil {
		return err
	}

	if localMeta.Id == meta.Id {
		return nil
	}

	if err := validateNodeMeta(meta); err != nil {
		return err
	}

	s.log.Printf("AddNode %s", color.RedString(meta.Id))

	new := s.addNodeMeta(meta)
	if err := s.persistNodeMeta(meta); err != nil {
		return err
	}
	if !new {
		return nil
	}

	if s.NumConnections() >= int(s.config.MaxPeers) {
		return nil
	}

	ctx := context.TODO()
	conn, err := s.connectNode(ctx, meta)
	if err != nil {
		return err
	}
	client := serverpb.NewNodeClient(conn)
	resp, err := client.Hello(ctx, &serverpb.HelloRequest{
		Meta: &localMeta,
	})
	if err != nil {
		return errors.Wrapf(err, "Hello")
	}
	if resp.Meta.Id != meta.Id {
		return errors.Errorf("expected node with ID %+v; got %+v", meta, resp.Meta)
	}

	s.mu.Lock()
	// make sure there isn't a duplicate connection
	_, ok := s.mu.peers[meta.Id]
	if !ok {
		s.mu.peers[meta.Id] = client
		s.mu.peerConns[meta.Id] = conn
	}
	s.mu.Unlock()

	// race condition to add connections, don't overwrite and close this
	// connection.
	if ok {
		s.log.Printf("found duplicate connection to: %s", color.RedString(meta.Id))
		if err := conn.Close(); err != nil {
			return err
		}
	}

	go func() {
		ticker := time.NewTicker(heartBeatInterval)
		for {
			select {
			case <-ticker.C:
			case <-s.stopper.ShouldStop():
				return
			}

			ctx, _ := context.WithTimeout(ctx, dialTimeout)
			if _, err := client.HeartBeat(ctx, &serverpb.HeartBeatRequest{}); err != nil {
				s.log.Printf("heartbeat error: %s: %+v", color.RedString(meta.Id), err)
				s.mu.Lock()
				delete(s.mu.peers, meta.Id)
				delete(s.mu.peerConns, meta.Id)
				s.mu.Unlock()
				if err := conn.Close(); err != nil {
					s.log.Printf("failed to close connection: %s: %+v", color.RedString(meta.Id), err)
				}
				return
			}
		}
	}()

	if err := s.AddNodes(resp.ConnectedPeers, resp.KnownPeers); err != nil {
		return err
	}

	return nil
}

// AddNodes adds a list of connected and known peers. Connected means that one
// of our peers is connected to them and known just means we know they exist.
// The server should prefer to connect to known first since that maximizes
// the cross section bandwidth of the graph.
func (s *Server) AddNodes(connected []*serverpb.NodeMeta, known []*serverpb.NodeMeta) error {
	for _, meta := range known {
		if err := s.AddNode(*meta); err != nil {
			return err
		}
	}
	for _, meta := range connected {
		if err := s.AddNode(*meta); err != nil {
			return err
		}
	}
	return nil
}

func validateAddr(s string) error {
	host, port, err := net.SplitHostPort(s)
	if err != nil {
		return err
	}
	if _, err := strconv.Atoi(port); err != nil {
		return err
	}
	if len(host) == 0 {
		return errors.Errorf("host empty in address %q", s)
	}
	return nil
}

func (s *Server) TestConn() (*grpc.ClientConn, error) {
	meta, err := s.NodeMeta()
	if err != nil {
		return nil, err
	}
	addr := meta.Addrs[0]
	creds := credentials.NewTLS(&tls.Config{
		Rand:               rand.Reader,
		InsecureSkipVerify: true,
	})
	ctx := context.Background()
	ctxDial, _ := context.WithTimeout(ctx, dialTimeout)
	return grpc.DialContext(ctxDial, addr,
		grpc.WithTransportCredentials(creds),
		grpc.WithBlock(),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(int(config.GRPCMsgSize)),
			grpc.MaxCallSendMsgSize(int(config.GRPCMsgSize)),
		),
	)
}

// BootstrapAddNode adds a node by using an address to do an insecure connection
// to a node, fetch node metadata and then reconnect via an encrypted
// connection.
func (s *Server) BootstrapAddNode(addr string) error {
	if err := validateAddr(addr); err != nil {
		return err
	}

	creds := credentials.NewTLS(&tls.Config{
		Rand:               rand.Reader,
		InsecureSkipVerify: true,
	})
	ctx := context.TODO()
	ctxDial, _ := context.WithTimeout(ctx, dialTimeout)
	conn, err := grpc.DialContext(ctxDial, addr,
		grpc.WithTransportCredentials(creds),
		grpc.WithBlock(),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(int(config.GRPCMsgSize)),
			grpc.MaxCallSendMsgSize(int(config.GRPCMsgSize)),
		),
	)
	if err != nil {
		return err
	}
	defer conn.Close()

	client := serverpb.NewNodeClient(conn)
	meta, err := client.Meta(ctx, nil)
	if err != nil {
		return err
	}
	return s.AddNode(*meta)
}
