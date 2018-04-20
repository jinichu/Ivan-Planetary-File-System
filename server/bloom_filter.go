package server

import (
	"context"
	"path"
	"proj2_f5w9a_h6v9a_q7w9a_r8u8_w1c0b/serverpb"
	"sort"
	"time"

	"github.com/dgraph-io/badger"
	"github.com/fatih/color"
	"github.com/pkg/errors"
	"github.com/willf/bloom"
)

var (
	RoutingTableInterval = 2 * time.Second
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

func (s *Server) peersWithFile(documentID string) []Route {
	s.mu.Lock()
	defer s.mu.Unlock()

	var routes []Route
	for id, peer := range s.mu.peers {
		if peer.routingTable == nil {
			continue
		}

		for hops, bf := range peer.routingTable.Filters {
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
					Client:  peer.client,
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

func (s *Server) addToRoutingTable(documentID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	table := s.mu.routingTable

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
	s.mu.routingTable = table

	return nil
}

// GetRoutingTable returns the local nodes routing table.
func (s *Server) GetRoutingTable(ctx context.Context, previousRT *serverpb.RoutingTable) (*serverpb.RoutingTable, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rt := s.mu.routingTable

	for _, peer := range s.mu.peers {
		if peer.routingTable == nil {
			continue
		}

		merged, err := s.mergeReceived(&rt, peer.routingTable)
		if err != nil {
			return nil, err
		}
		rt = *merged
	}

	return &rt, nil
}

func (s *Server) ReceiveNewRoutingTable() {
	ticker := time.NewTicker(RoutingTableInterval)
	for {
		select {
		case <-ticker.C:
		case <-s.ctx.Done():
			return
		}

		peers := map[string]*peer{}

		tableCount := 0
		s.mu.Lock()
		maxDepth := 1

		for id, peer := range s.mu.peers {
			peers[id] = peer
			if peer.routingTable != nil {
				tableCount += 1
			}

			depth := len(peer.routingTable.GetFilters())
			if depth > maxDepth {
				maxDepth = depth + 1
			}
		}
		s.mu.Unlock()
		s.log.Printf("fetching routing tables... have %d, depth %d", tableCount, maxDepth)

		for id, peer := range peers {
			ctx, _ := context.WithTimeout(s.ctx, 10*time.Second)

			if err := s.receiveTableOfPeer(ctx, id, peer); err != nil {
				s.log.Printf("get routing table error: %s: %+v", color.RedString(id), err)

				s.mu.Lock()
				peer.routingTable = nil
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

func (s *Server) CheckNumHopsToGetToFile(documentID string) (int, error) {
	rt, err := s.GetRoutingTable(context.Background(), nil)
	if err != nil {
		return 0, err
	}

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

func (s *Server) receiveTableOfPeer(ctx context.Context, remoteID string, peer *peer) error {
	remoteTable, err := peer.client.GetRoutingTable(ctx, &serverpb.RoutingTable{})
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	peer.routingTable = remoteTable

	return nil
}

func (s *Server) mergeReceived(rt0 *serverpb.RoutingTable, rt1 *serverpb.RoutingTable) (*serverpb.RoutingTable, error) {
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

	deduped, err := deleteDuplicates(withDupes, int(s.config.MaxWidth))
	if err != nil {
		return nil, err
	}

	return &serverpb.RoutingTable{Filters: deduped}, nil
}

func deleteDuplicates(filters []*serverpb.BloomFilter, maxWidth int) ([]*serverpb.BloomFilter, error) {
	seen := make(map[string]bool)
	deduped := make([]*serverpb.BloomFilter, len(filters))
	firstDuplicate := -1
	lastNonEmpty := 0

	emptyFilter := createNewBloomFilter()

	for i, filter := range filters {
		var f bloom.BloomFilter
		if len(filter.Data) > 0 {
			if err := f.GobDecode(filter.Data); err != nil {
				return nil, err
			}
		}
		empty := len(filter.Data) == 0 || f.Equal(emptyFilter)
		if !empty {
			lastNonEmpty = i
		}

		key := string(filter.Data)
		if !empty && seen[key] {
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

	if lastNonEmpty < firstDuplicate {
		firstDuplicate = lastNonEmpty + 1
	}

	if maxWidth > 0 && firstDuplicate > maxWidth {
		firstDuplicate = maxWidth
	}

	return deduped[:firstDuplicate], nil
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

func (s *Server) loadRoutingTable() error {
	if err := s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		{
			prefix := []byte("/document/")
			for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
				item := it.Item()
				id := path.Base(string(item.Key()))
				if err := s.addToRoutingTable(id); err != nil {
					return err
				}
			}
		}

		{
			prefix := []byte("/reference/")
			for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
				item := it.Item()
				id := path.Base(string(item.Key()))
				if err := s.addToRoutingTable(id); err != nil {
					return err
				}
			}
		}
		return nil
	}); err != nil {
		return err
	}

	return nil
}
