package server

import (
	"context"
	"crypto/ecdsa"
	"crypto/tls"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"proj2_f5w9a_h6v9a_q7w9a_r8u8_w1c0b/config"
	"proj2_f5w9a_h6v9a_q7w9a_r8u8_w1c0b/serverpb"
	"proj2_f5w9a_h6v9a_q7w9a_r8u8_w1c0b/stopper"
	"strings"
	"sync"

	"github.com/dgraph-io/badger"
	"github.com/fatih/color"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"github.com/pkg/errors"

	"golang.org/x/sync/errgroup"

	"google.golang.org/grpc"
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

	stopper *stopper.Stopper
	mux     *http.ServeMux

	mu struct {
		sync.Mutex

		l              net.Listener
		grpcServer     *grpc.Server
		httpServer     *http.Server
		peerMeta       map[string]serverpb.NodeMeta
		peers          map[string]serverpb.NodeClient
		peerConns      map[string]*grpc.ClientConn
		routingTables  map[string]serverpb.RoutingTable
		channels       map[string]*channel
		nextListenerID int
	}
}

// New returns a new server.
func New(c serverpb.NodeConfig) (*Server, error) {
	s := &Server{
		log:     log.New(os.Stderr, "", log.Flags()|log.Lshortfile),
		config:  c,
		stopper: stopper.New(),
		mux:     http.NewServeMux(),
	}
	s.mu.peerMeta = map[string]serverpb.NodeMeta{}
	s.mu.peers = map[string]serverpb.NodeClient{}
	s.mu.peerConns = map[string]*grpc.ClientConn{}
	s.mu.routingTables = map[string]serverpb.RoutingTable{}
	s.mu.channels = map[string]*channel{}

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

	s.setupHTTP()
	if err := s.loadRoutingTable(); err != nil {
		return nil, err
	}

	return s, nil
}

func (s *Server) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	err := errors.New("shutting down...")
	s.log.Printf("%v", err)
	s.stopper.Stop()

	if s.mu.grpcServer != nil {
		s.mu.grpcServer.Stop()
	}

	if s.mu.httpServer != nil {
		if err := s.mu.httpServer.Close(); err != nil {
			return err
		}
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

	//creds := credentials.NewServerTLSFromCert(s.cert)
	grpcServer := grpc.NewServer(
		grpc.MaxRecvMsgSize(int(config.GRPCMsgSize)),
		grpc.MaxSendMsgSize(int(config.GRPCMsgSize)),
	)
	serverpb.RegisterNodeServer(grpcServer, s)
	serverpb.RegisterClientServer(grpcServer, s)
	go s.ReceiveNewRoutingTable()

	httpServer := http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.ProtoMajor == 2 && strings.HasPrefix(
				r.Header.Get("Content-Type"), "application/grpc") {
				grpcServer.ServeHTTP(w, r)
			} else {
				s.mux.ServeHTTP(w, r)
			}
		}),
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{
				*s.cert,
			},
		},
	}

	s.mu.Lock()
	s.mu.l = l
	s.mu.grpcServer = grpcServer
	s.mu.httpServer = &httpServer
	s.mu.Unlock()

	meta, err := s.NodeMeta()
	if err != nil {
		return err
	}

	s.log.SetPrefix(color.RedString(meta.Id) + " " + color.GreenString(l.Addr().String()) + " ")

	s.log.Printf("Listening to %s", l.Addr().String())
	var g errgroup.Group
	g.Go(func() error {
		if err := httpServer.ServeTLS(l, "", ""); err != http.ErrServerClosed {
			return err
		}
		return nil
	})

	mux := runtime.NewServeMux()
	s.mux.Handle("/api/", http.StripPrefix("/api", mux))

	ctx := context.Background()
	conn, err := s.LocalConn()
	if err != nil {
		return err
	}
	if err := serverpb.RegisterClientHandler(ctx, mux, conn); err != nil {
		return err
	}

	return g.Wait()
}
