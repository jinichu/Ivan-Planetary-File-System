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

	// Force connection back if we got a hello request. We want to avoid one way
	// links.
	if err := s.AddNode(*reqMeta, true); err != nil {
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

// AddNode adds a node to the server. force disables new/MaxPeers checking.
func (s *Server) AddNode(meta serverpb.NodeMeta, force bool) error {
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

	s.mu.Lock()
	_, alreadyPeer := s.mu.peers[meta.Id]
	s.mu.Unlock()

	if (!new || s.NumConnections() >= int(s.config.MaxPeers)) && !force || alreadyPeer {
		return nil
	}

	// avoid duplicate connections
	s.mu.Lock()
	_, connecting := s.mu.connecting[meta.Id]
	s.mu.connecting[meta.Id] = struct{}{}
	s.mu.Unlock()

	if connecting {
		return nil
	}

	defer func() {
		s.mu.Lock()
		defer s.mu.Unlock()

		delete(s.mu.connecting, meta.Id)
	}()

	ctx, cancel := context.WithCancel(s.ctx)
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

	peer := &peer{
		ctx:    ctx,
		cancel: cancel,
		client: client,
		conn:   conn,
		s:      s,
		meta:   meta,
	}

	s.mu.Lock()
	// make sure there isn't a duplicate connection
	_, ok := s.mu.peers[meta.Id]
	if !ok {
		s.mu.peers[meta.Id] = peer
	}
	s.mu.Unlock()

	// race condition to add connections, don't overwrite and close this
	// connection.
	if ok {
		s.log.Printf("found duplicate connection to: %s", color.RedString(meta.Id))
		return conn.Close()
	}

	go peer.heartbeat()

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
	for _, meta := range append(known, connected...) {
		if err := s.AddNode(*meta, false); err != nil {
			s.log.Printf("failed to AddNode: %+v", err)
			continue
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

func (s *Server) LocalConn() (*grpc.ClientConn, error) {
	meta, err := s.NodeMeta()
	if err != nil {
		return nil, err
	}
	return s.connectNode(s.ctx, meta)
}

// BootstrapAddNode adds a node by using an address to do an insecure connection
// to a node, fetch node metadata and then reconnect via an encrypted
// connection.
func (s *Server) BootstrapAddNode(ctx context.Context, addr string) error {
	if ctx == nil {
		ctx = s.ctx
	}

	if err := validateAddr(addr); err != nil {
		return err
	}

	creds := credentials.NewTLS(&tls.Config{
		Rand:               rand.Reader,
		InsecureSkipVerify: true,
	})
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
	return s.AddNode(*meta, true)
}

type peer struct {
	meta         serverpb.NodeMeta
	ctx          context.Context
	cancel       context.CancelFunc
	client       serverpb.NodeClient
	conn         *grpc.ClientConn
	routingTable *serverpb.RoutingTable

	s *Server
}

func (p *peer) Close() {
	p.s.mu.Lock()
	defer p.s.mu.Unlock()

	p.closeLocked()
}

func (p *peer) closeLocked() {
	delete(p.s.mu.peers, p.meta.Id)

	p.cancel()

	if err := p.conn.Close(); err != nil {
		p.s.log.Printf("failed to close connection: %s: %+v", color.RedString(p.meta.Id), err)
	}
}

func (p *peer) heartbeat() {
	ticker := time.NewTicker(heartBeatInterval)
	for {
		select {
		case <-ticker.C:
		case <-p.ctx.Done():
			return
		}

		ctx, _ := context.WithTimeout(p.ctx, dialTimeout)
		if _, err := p.client.HeartBeat(ctx, &serverpb.HeartBeatRequest{}); err != nil {
			p.s.log.Printf("heartbeat error: %s: %+v", color.RedString(p.meta.Id), err)
			p.Close()
			return
		}
	}
}
