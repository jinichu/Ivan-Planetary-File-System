package server

import (
    "fmt"
    "github.com/willf/bloom"
    "proj2_f5w9a_h6v9a_q7w9a_r8u8_w1c0b/serverpb"
    "context"
    "github.com/fatih/color"
    "log"
    "time"
    "io"
    "github.com/pkg/errors"
)

const (
    n = 10
    load = 20
    num_keys = 5
)

func (s *Server) createNewBloomFilter() *bloom.BloomFilter {
    return bloom.New(load * n, 5)
}

func (s *Server) addToRoutingTable(id string, file_hash string) error {
    if _, ok := s.mu.routingTables[id]; ok {
      bfs := s.mu.routingTables[id].Filters
     
      filter := s.createNewBloomFilter()
      err := filter.GobDecode(bfs[0].Data)
      if err != nil {
        return err
      }
      filter.AddString(file_hash)
      encoded_bf, err := filter.GobEncode()
      if err != nil {
        return err
      }
      bfs[0].Data = encoded_bf
    } else {
      filter := s.createNewBloomFilter()
      filter.AddString(file_hash)
      encoded_bf, err := filter.GobEncode()
      if err != nil {
        return err
      }
      bloom_filter := &serverpb.BloomFilter{Data: encoded_bf}
      filters := []*serverpb.BloomFilter{bloom_filter}
      routing_table := serverpb.RoutingTable{Filters: filters}
      s.mu.routingTables[id] = routing_table
    }

    return nil
}

// send change
func (s *Server) SendCurrentRoutingTable(previousRT *serverpb.RoutingTable, stream serverpb.Node_SendCurrentRoutingTableServer) error {
  routing_table, err := s.getLocalRT()
  if err != nil {
    return err
  }
  if err := stream.Send(routing_table); err != nil {
    log.Fatalf("Failed to send a note: %v", err)
  }
  return nil
}

func (s *Server) ReceiveNewRoutingTable() error {
  go func() {
    for {
      for metaId, conn  := range s.mu.peerConns {
        currentRoutingTableForPeer := s.mu.routingTables[metaId]

        client := serverpb.NewNodeClient(conn)

        ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)

        previousRt := &serverpb.RoutingTable{Filters: currentRoutingTableForPeer.Filters}
        stream, err := client.SendCurrentRoutingTable(ctx, previousRt)

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
}

// helpers
// add to filters
func (s *Server) getLocalId() (string, error) {
  meta, err := s.NodeMeta()
  if err != nil {
    return "", err
  }
  return meta.Id, nil
}

func (s *Server) getLocalRT() (*serverpb.RoutingTable, error) {
  meta, err := s.NodeMeta()
  if err != nil {
    return nil, err
  }
  return &serverpb.RoutingTable{Filters: s.mu.routingTables[meta.Id].Filters}, nil
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
      filter := s.createNewBloomFilter()
      err := filter.GobDecode(bf.Data)
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

  return -1, errors.New("missing Document")
}

func (s *Server) receiveTableOfPeer(stream serverpb.Node_SendCurrentRoutingTableClient) {
  routing_table, err := s.getLocalRT()
  if err != nil {
    return
  }
  local_id, err := s.getLocalId()
  if err != nil {
    return
  }
  for {
    newRoutingTableOfPeer, err := stream.Recv()
    if err == io.EOF {
      break
    } 
    if err != nil {
      break
    }
    s.mu.Lock()
    merged, err := s.mergeReceived(routing_table, newRoutingTableOfPeer, local_id)
    if err != nil {
      break
    }
    if len(merged.Filters) > 0 {
      s.mu.routingTables[local_id] = *merged
    }

    s.mu.Unlock()
    time.Sleep(heartBeatInterval)
  }
}

func (s *Server) mergeReceived(rt0 *serverpb.RoutingTable, rt1 *serverpb.RoutingTable, local_id string) (*serverpb.RoutingTable, error) {
  i := 1
  var size int
  if (len(rt0.Filters) > len(rt1.Filters)) {
    size = len(rt0.Filters) + 1
  } else {
    size = len(rt1.Filters) + 1
  }

  with_dupes := make([]*serverpb.BloomFilter, size)

  for k := 0; k < size; k++ {
    with_dupes[k] = &serverpb.BloomFilter{}
  }

  if len(rt0.Filters) > 0 {
    with_dupes[0] = rt0.Filters[0]
  }


  for i < len(rt0.Filters) && i < len(rt1.Filters) {
    merged, err := s.mergeFilters(rt0.Filters[i], rt1.Filters[i - 1])
    if err != nil {
      return nil, err
    }
    with_dupes[i] = merged
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

  return &serverpb.RoutingTable{Filters: deduped}, nil
}

func (s *Server) deleteDuplicates(filters []*serverpb.BloomFilter) (deduped []*serverpb.BloomFilter) {
  seen := make(map[string]bool)
  tail_count := 0
  for _, filter := range filters {
    key := string(filter.Data)
    if seen[key] != true {
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

func (s *Server) mergeFilters(bf0 *serverpb.BloomFilter, bf1 *serverpb.BloomFilter) (*serverpb.BloomFilter, error) {
   if len(bf0.Data) <= 1 {
    return bf1, nil
   }

   if len(bf1.Data) <= 1 {
    return bf0, nil
   }

   filter := s.createNewBloomFilter()
   err := filter.GobDecode(bf0.Data)
   if err != nil {
    return nil, err
   }

   received_filter := s.createNewBloomFilter()
   err = received_filter.GobDecode(bf1.Data)
   if err != nil {
    return nil, err
   }

   filter.Merge(received_filter)

   merged_filter, err := filter.GobEncode()
   if err != nil {
    return nil, err
   }

   return &serverpb.BloomFilter{Data: merged_filter}, nil
}
