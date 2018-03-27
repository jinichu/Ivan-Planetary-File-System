package server

import (
  "fmt"
  "github.com/willf/bloom"
  "proj2_f5w9a_h6v9a_q7w9a_r8u8_w1c0b/serverpb"
   "google.golang.org/grpc"
   "google.golang.org/grpc/credentials"
   "context"
  "github.com/fatih/color"
  "crypto/rand"
   "crypto/tls"
   "log"
  "time"
   "io"
)

const (
  n = 10
)

// add to routing table
// finished.
func (s *Server) addToRoutingTable(id string, file_hash string) error {
  //if does not have key initialize
    if _, ok := s.mu.routingTables[id]; ok {
      bfs := s.mu.routingTables[id].Filters
      //fmt.Println("Exists in the filter", bfs)
      
      filter := bloom.New(20*n, 5)
      filter.GobDecode(bfs[0].Data)
      filter.AddString(file_hash)
      encoded_bf, _ := filter.GobEncode()
      bfs[0].Data = encoded_bf
    } else {
      filter := bloom.New(20*n, 5)
      filter.AddString(file_hash)
      encoded_bf, _ := filter.GobEncode()
      local_id := s.getLocalId()
      map_init := map[string]bool{local_id: true}
      bloom_filter := &serverpb.BloomFilter{Data: encoded_bf, Ids: map_init}
      filters := []*serverpb.BloomFilter{bloom_filter}
      routing_table := &serverpb.RoutingTable{Filters: filters}
      s.mu.routingTables[id] = *routing_table
    }

    //fmt.Println("\nadded to the routing table!", s.mu.routingTables)

    return nil
}

// send change
func (s *Server) SendCurrentRoutingTable(previousRT *serverpb.RoutingTable, stream serverpb.PublishBloomFilters_SendCurrentRoutingTableServer) error {
  routing_table := s.getLocalRT()
  if err := stream.Send(routing_table); err != nil {
    log.Fatalf("Failed to send a note: %v", err)
  }
  return nil
}

// receive change
func (s *Server) ReceiveNewRoutingTable() error {
    go func() {
      for {
          for metaId, peerMeta  := range s.mu.peerMeta {
            addr := peerMeta.Addrs[0]
            creds := credentials.NewTLS(&tls.Config{
              Rand:               rand.Reader,
              InsecureSkipVerify: true,
            })
            ctx := context.TODO()
            ctxDial, _ := context.WithTimeout(ctx, dialTimeout)
            conn, err := grpc.DialContext(ctxDial, addr, grpc.WithTransportCredentials(creds), grpc.WithBlock())
            if err != nil {
              fmt.Println(err)
              return
            }
            defer conn.Close()

            currentRoutingTableForPeer := s.mu.routingTables[metaId]

            client := serverpb.NewPublishBloomFiltersClient(conn)

            ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
            defer cancel()

            previousRt := &serverpb.RoutingTable{Filters: currentRoutingTableForPeer.Filters}
            stream, err := client.SendCurrentRoutingTable(ctx, previousRt)

            // handle error for stream
            if err != nil {
              s.log.Printf("send current routing table error: %s: %+v", color.RedString(metaId), err)
              s.mu.Lock()
              delete(s.mu.routingTables, metaId)
              s.mu.Unlock()
              if err := conn.Close(); err != nil {
                s.log.Printf("failed to close connection: %s: %+v", color.RedString(metaId), err)
              }
              return
            }
            s.receiveTableOfPeer(stream)
          }
          time.Sleep(heartBeatInterval)
        }
      }()
    return nil
  return nil
}

// helpers
// add to filters
func (s *Server) getLocalId() string {
  meta, _ := s.NodeMeta()
  return meta.Id
}

func (s *Server) addToRTFilter(filters []serverpb.BloomFilter, filter serverpb.BloomFilter) []serverpb.BloomFilter {
  filter_new := append(filters, filter)
  return filter_new
}

func (s *Server) getLocalRT() *serverpb.RoutingTable {
  localMeta, _ := s.NodeMeta()
  return &serverpb.RoutingTable{Filters: s.mu.routingTables[localMeta.Id].Filters}
}

func (s *Server) hasKey(id string) bool {
    if _, ok := s.mu.routingTables[id]; ok {
      return true
    } else {
      return false
    }
}

func (s *Server) checkNumHopsToGetToFile(accessId string) (int, error) {
  localMeta, err := s.NodeMeta()
  if err != nil {
    return 0, err
  }
  my_rt := s.mu.routingTables[localMeta.Id]
  my_bloom_filters := my_rt.Filters

  for i, bf := range my_bloom_filters {
    if (len(bf.Data) > 1) {
      filter := bloom.New(20*n, 5)
      err := filter.GobDecode(bf.Data)
      //fmt.Println("\ntrying to decode this: ", bf.Data)
      if err != nil {
        fmt.Println("Couldn't decode.", err)
        return -1, err
      }

      isInThisHop := filter.TestString(accessId)
      if isInThisHop == true {
        return i, nil
      }
    }
  }

  return -1, nil
}

func (s *Server) receiveTableOfPeer(stream serverpb.PublishBloomFilters_SendCurrentRoutingTableClient) {
  routing_table := s.getLocalRT()
  local_id := s.getLocalId()
  for {
    // receive table from peer
    newRoutingTableOfPeer, err := stream.Recv()
    if err == io.EOF {
      break
    } 
    if err != nil {
      fmt.Println(err)
    }
    s.mu.Lock()
    merged := *s.mergeReceived(routing_table, newRoutingTableOfPeer, local_id)
    if len(merged.Filters) > 0 {
      s.mu.routingTables[local_id] = merged
    }

    s.mu.Unlock()
    time.Sleep(heartBeatInterval)
  }
}

func (s *Server) mergeReceived(rt0 *serverpb.RoutingTable, rt1 *serverpb.RoutingTable, local_id string) *serverpb.RoutingTable {
  i := 1
  var size int
  if (len(rt0.Filters) > len(rt1.Filters)) {
    size = len(rt0.Filters) + 1
  } else {
    size = len(rt1.Filters) + 1
  }


  with_dupes := make([]*serverpb.BloomFilter, size)

  for k := 0; k < size; k++ {
    data := make([]byte, 1)
    ids := make(map[string]bool)
    with_dupes[k] = &serverpb.BloomFilter{Data: data, Ids: ids}
  }

  if len(rt0.Filters) > 0 {
    with_dupes[0] = rt0.Filters[0]
  }


  for i < len(rt0.Filters) && i < len(rt1.Filters) {
    with_dupes[i] = s.mergeFilters(rt0.Filters[i], rt1.Filters[i - 1])
    i += 1
  }

  for i <= len(rt1.Filters) && i >= 1 {
    with_dupes[i] = rt1.Filters[i - 1]
    i += 1
  }

  for i < len(rt0.Filters) {
    with_dupes[i] = rt0.Filters[i]
    i += 1
  }

  deduped := s.deleteDuplicates(with_dupes)

  return &serverpb.RoutingTable{Filters: deduped}
}

func (s *Server) deleteDuplicates(filters []*serverpb.BloomFilter) (deduped []*serverpb.BloomFilter) {
  deduped = []*serverpb.BloomFilter{}
  seen := make(map[string]bool)
  tail_count := 0
  for _, filter := range filters {
    key := string(filter.Data)
    if seen[key] == true {
    } else {
      if (len(filter.Data) > 1) {
        seen[key] = true
        tail_count = 0
      } else {
        tail_count += 1
      }
      deduped = append(deduped, filter)
    }
  }
  if (len(deduped) > 0) {
    deduped = deduped[0:len(deduped) -  tail_count]
  }
  return deduped
}

func (s *Server) mergeFilters(bf0 *serverpb.BloomFilter, bf1 *serverpb.BloomFilter) *serverpb.BloomFilter {
   filter := bloom.New(20*n, 5)
   filter.GobDecode(bf0.Data)

   if len(bf0.Data) <= 1 {
    return bf1
   }

   if len(bf1.Data) <= 1 {
    return bf0
   }

   received_filter := bloom.New(20*n, 5)
   received_filter.GobDecode(bf1.Data)

   filter.Merge(received_filter)

   merged_filter, _ := filter.GobEncode()

   for k, v := range bf1.Ids {
       bf0.Ids[k] = v
   }

   return &serverpb.BloomFilter{Data: merged_filter, Ids: bf0.Ids}
}
