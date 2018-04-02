package server

import (
	"crypto/ecdsa"
	"crypto/tls"
	"log"
	"net"
	"os"
	"path/filepath"
	"proj2_f5w9a_h6v9a_q7w9a_r8u8_w1c0b/serverpb"
	"sync"

	"github.com/dgraph-io/badger"
	"github.com/fatih/color"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

var ErrUnimplemented = errors.New("unimplemented")

// Server is the main server struct.
type Server struct {
	log    *log.Logger
	config serverpb.NodeConfig
	db     *badger.DB

	key        *ecdsa.PrivateKey
	cert       *tls.Certificate
	certPublic string

	mu struct {
		sync.Mutex

		l             net.Listener
		grpcServer    *grpc.Server
		peerMeta      map[string]serverpb.NodeMeta
		peers         map[string]serverpb.NodeClient
		peerConns     map[string]*grpc.ClientConn
		routingTables map[string]serverpb.RoutingTable
	}
}

// New returns a new server.
func New(c serverpb.NodeConfig) (*Server, error) {
	s := &Server{
		log:    log.New(os.Stderr, "", log.Flags()|log.Lshortfile),
		config: c,
	}
	s.mu.peerMeta = map[string]serverpb.NodeMeta{}
	s.mu.peers = map[string]serverpb.NodeClient{}
	s.mu.peerConns = map[string]*grpc.ClientConn{}
	s.mu.routingTables = map[string]serverpb.RoutingTable{}

	if len(c.Path) == 0 {
		return nil, errors.Errorf("config: path must not be empty")
	}
	if err := os.MkdirAll(c.Path, 0700); err != nil {
		return nil, err
	}

	badgerDir := filepath.Join(c.Path, "badger")
	opts := badger.DefaultOptions
	opts.Dir = badgerDir
	opts.ValueDir = badgerDir
	db, err := badger.Open(opts)
	if err != nil {
		return nil, err
	}

	s.db = db

	if err := s.loadOrGenerateCert(); err != nil {
		return nil, err
	}

	return s, nil
}

func (s *Server) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	err := errors.New("shutting down...")
	s.log.Printf("%v", err)

	if s.mu.grpcServer != nil {
		s.mu.grpcServer.Stop()
	}

	if err := s.db.Close(); err != nil {
		return errors.Wrapf(err, "db close")
	}
	return nil
}

// TestGRPCServer returns the internal *grpc.Server. Testing purposes only!
func (s *Server) TestGRPCServer() *grpc.Server {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.mu.grpcServer
}

// Listen causes the server to listen on the specified IP and port.
func (s *Server) Listen(addr string) error {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	creds := credentials.NewServerTLSFromCert(s.cert)
	grpcServer := grpc.NewServer(grpc.Creds(creds))
	serverpb.RegisterNodeServer(grpcServer, s)
	serverpb.RegisterClientServer(grpcServer, s)
	go s.ReceiveNewRoutingTable()

	s.mu.Lock()
	s.mu.l = l
	s.mu.grpcServer = grpcServer
	s.mu.Unlock()

	meta, err := s.NodeMeta()
	if err != nil {
		return err
	}

	s.log.SetPrefix(color.RedString(meta.Id) + " " + color.GreenString(l.Addr().String()) + " ")

	s.log.Printf("Listening to %s", l.Addr().String())
	if err := grpcServer.Serve(l); err != nil && err != grpc.ErrServerStopped {
		return err
	}
	return nil
}
