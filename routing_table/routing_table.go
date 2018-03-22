package routing_table

import (
  "google.golang.org/grpc"
  "sync"
  "net"
  "context"

  "os"
  "proj2_f5w9a_h6v9a_q7w9a_r8u8_w1c0b/routing_tablepb"
  "log"
)

type RoutingTable struct {
  log    *log.Logger
  config routing_tablepb.NodeConfig

  mu struct {
    sync.Mutex
    l          net.Listener
    grpcServer *grpc.Server
    unionizedEntries  map[int]BloomFilter        // Union of bloom filters for every i
  }
}

func (rt *RoutingTable) ListenRT(addr string) error {
  l, err := net.Listen("tcp", addr)
  if err != nil {
    return err
  }

  grpcServer := grpc.NewServer()

  rt.mu.Lock()
  rt.mu.l = l
  rt.mu.grpcServer = grpcServer
  rt.mu.Unlock()

  rt.log.Printf("Listening to %s", l.Addr().String())
  if err := grpcServer.Serve(l); err != nil && err != grpc.ErrServerStopped {
    return err
  }
  return nil
}

// New returns a new server.
func NewRT(c routing_tablepb.NodeConfig) (*RoutingTable, error) {
  rt := &RoutingTable{
    log:    log.New(os.Stderr, "", log.Flags()|log.Lshortfile),
    config: c,
  }

  //retrieve by id, and union of all filters int hops away
  rt.mu.unionizedEntries = map[int]BloomFilter{}
  rt.mu.unionizedEntries[0] = BloomFilter.New(64, 3)

  return rt, nil
}

// Update entry when we receive a change from one of the peers
func (rt *RoutingTable) UpdateEntry(ctx context.Context, in *routing_tablepb.UpdateEntryRequest) (*routing_tablepb.UpdateEntryResponse, error)  {
  // Updae an entry to the routing table.
  num_hops := int(in.NumHops)
  bf := *in.Bf
  bfs := rt.mu.unionizedEntries[num_hops]
  union_of_bfs := rt.UnionBloomFilters(bfs, bf)
  rt.mu.unionizedEntries[num_hops] = union_of_bfs
  return nil, nil
}


 // Get the union of all bloom filters num_hops away
func (rt * RoutingTable) UnionBloomFilters(bfs BloomFilter, bf BloomFilter) BloomFilter {
  return BloomFilter{}
}

func (rt * RoutingTable) AddFile(ctx context.Context, in *routing_tablepb.AddNewFileRequest) (*routing_tablepb.AddNewFileResponse, error) {
  bf := rt.mu.unionizedEntries[0]
  file_hash := in.FileHash
  bf.Add([]byte(file_hash))


  //TODO publish change to all the other nodes
  return nil, nil
}

// Check every single set of peers to see if it has file. If it does return that.
func (rt * RoutingTable) GetPotentialPeersWithFile(ctx context.Context, in *routing_tablepb.GetPeersWithFileRequest) (*routing_tablepb.GetPeersWithFileResponse, error) {
  return nil, nil
}

