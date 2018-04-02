package server

import (
	"context"
	"proj2_f5w9a_h6v9a_q7w9a_r8u8_w1c0b/serverpb"
	"sort"
	"time"

	"github.com/fatih/color"
	"github.com/pkg/errors"
	"github.com/willf/bloom"
)

const (
	NumberOfKeys             = 100000
	FalsePositiveProbability = 0.01
)

func createNewBloomFilter() *bloom.BloomFilter {
	return bloom.NewWithEstimates(NumberOfKeys, FalsePositiveProbability)
}

type Route struct {
	ID      string
	Client  serverpb.NodeClient
	NumHops int32
}

func (s *Server) TestRTs() map[string]serverpb.RoutingTable {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.mu.routingTables
}

func (s *Server) peersWithFile(documentID string) []Route {
	s.mu.Lock()
	defer s.mu.Unlock()

	var routes []Route
	for id, client := range s.mu.peers {
		table, ok := s.mu.routingTables[id]
		if !ok {
			continue
		}

		for hops, bf := range table.Filters {
			if len(bf.Data) == 0 {
				continue
			}

			var filter bloom.BloomFilter
			if err := filter.GobDecode(bf.Data); err != nil {
				s.log.Printf("failed to decode filter for node %+v: %+v", id, err)
				continue
			}

			if filter.TestString(documentID) {
				routes = append(routes, Route{
					ID:      id,
					Client:  client,
					NumHops: int32(hops + 1),
				})
			}
		}
	}

	sort.Slice(routes, func(i, j int) bool {
		return routes[i].NumHops < routes[j].NumHops
	})

	return routes
}

func (s *Server) addToRoutingTable(id string, documentID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	table := s.mu.routingTables[id]

	filter := createNewBloomFilter()
	if len(table.Filters) > 0 {
		if err := filter.GobDecode(table.Filters[0].Data); err != nil {
			return err
		}
	}

	filter.AddString(documentID)
	data, err := filter.GobEncode()
	if err != nil {
		return err
	}
	entry := &serverpb.BloomFilter{
		Data: data,
	}
	if len(table.Filters) > 0 {
		table.Filters[0] = entry
	} else {
		table.Filters = append(table.Filters, entry)
	}
	s.mu.routingTables[id] = table

	return nil
}

// GetRoutingTable returns the local nodes routing table.
func (s *Server) GetRoutingTable(ctx context.Context, previousRT *serverpb.RoutingTable) (*serverpb.RoutingTable, error) {
	routingTable, err := s.getLocalRT()
	if err != nil {
		return nil, err
	}
	return routingTable, nil
}

func (s *Server) ReceiveNewRoutingTable() {
	ticker := time.NewTicker(heartBeatInterval)
	for {
		select {
		case <-ticker.C:
		case <-s.stopper.ShouldStop():
			return
		}

		clients := map[string]serverpb.NodeClient{}

		s.mu.Lock()
		s.log.Printf("fetching routing tables... have %d", len(s.mu.routingTables))
		for id, client := range s.mu.peers {
			clients[id] = client
		}
		s.mu.Unlock()

		for id, client := range clients {
			ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)

			if err := s.receiveTableOfPeer(ctx, id, client); err != nil {
				s.log.Printf("get routing table error: %s: %+v", color.RedString(id), err)

				s.mu.Lock()
				delete(s.mu.routingTables, id)
				s.mu.Unlock()

				continue
			}
		}
	}
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
	s.mu.Lock()
	defer s.mu.Unlock()

	rt := s.mu.routingTables[meta.Id]
	return &rt, nil
}

func (s *Server) checkNumHopsToGetToFile(documentID string) (int, error) {
	localMeta, err := s.NodeMeta()
	if err != nil {
		return 0, err
	}
	rt := s.mu.routingTables[localMeta.Id]
	for i, bf := range rt.Filters {
		if len(bf.Data) == 0 {
			continue
		}
		var filter bloom.BloomFilter
		if err := filter.GobDecode(bf.Data); err != nil {
			return 0, err
		}

		isInThisHop := filter.TestString(documentID)
		if isInThisHop == true {
			return i, nil
		}
	}

	return 0, errors.Errorf("missing Document: %+v", documentID)
}

func (s *Server) receiveTableOfPeer(ctx context.Context, remoteID string, client serverpb.NodeClient) error {
	remoteTable, err := client.GetRoutingTable(ctx, &serverpb.RoutingTable{})
	if err != nil {
		return err
	}

	localTable, err := s.getLocalRT()
	if err != nil {
		return err
	}
	localID, err := s.getLocalId()
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.mu.routingTables[remoteID] = *remoteTable
	merged, err := mergeReceived(localTable, remoteTable)
	if err != nil {
		return err
	}
	s.mu.routingTables[localID] = *merged

	return nil
}

func mergeReceived(rt0 *serverpb.RoutingTable, rt1 *serverpb.RoutingTable) (*serverpb.RoutingTable, error) {
	var size int
	if len(rt0.Filters) > len(rt1.Filters) {
		size = len(rt0.Filters) + 1
	} else {
		size = len(rt1.Filters) + 1
	}

	withDupes := make([]*serverpb.BloomFilter, size)

	for k := 0; k < size; k++ {
		withDupes[k] = &serverpb.BloomFilter{}
	}

	if len(rt0.Filters) > 0 {
		withDupes[0] = rt0.Filters[0]
	}

	i := 1
	for ; i < len(rt0.Filters) && i <= len(rt1.Filters); i++ {
		merged, err := mergeFilters(rt0.Filters[i], rt1.Filters[i-1])
		if err != nil {
			return nil, err
		}
		withDupes[i] = merged
	}

	for ; i <= len(rt1.Filters) && i >= 1; i++ {
		withDupes[i] = rt1.Filters[i-1]
	}

	for ; i < len(rt0.Filters); i++ {
		withDupes[i] = rt0.Filters[i]
	}

	deduped := deleteDuplicates(withDupes)

	return &serverpb.RoutingTable{Filters: deduped}, nil
}

func deleteDuplicates(filters []*serverpb.BloomFilter) []*serverpb.BloomFilter {
	seen := make(map[string]bool)
	deduped := make([]*serverpb.BloomFilter, len(filters))
	firstDuplicate := -1
	for i, filter := range filters {
		key := string(filter.Data)
		if seen[key] {
			if firstDuplicate <= 0 {
				firstDuplicate = i
			}
			continue
		}
		seen[key] = true
		deduped[i] = filter
	}

	if firstDuplicate <= 0 {
		firstDuplicate = len(deduped)
	}

	return deduped[:firstDuplicate]
}

func mergeFilters(bf0 *serverpb.BloomFilter, bf1 *serverpb.BloomFilter) (*serverpb.BloomFilter, error) {
	if len(bf0.Data) == 0 {
		return bf1, nil
	}

	if len(bf1.Data) == 0 {
		return bf0, nil
	}

	var filter bloom.BloomFilter
	if err := filter.GobDecode(bf0.Data); err != nil {
		return nil, err
	}

	var receivedFilter bloom.BloomFilter
	if err := receivedFilter.GobDecode(bf1.Data); err != nil {
		return nil, err
	}

	if err := filter.Merge(&receivedFilter); err != nil {
		return nil, err
	}

	mergedFilter, err := filter.GobEncode()
	if err != nil {
		return nil, err
	}

	return &serverpb.BloomFilter{Data: mergedFilter}, nil
}
