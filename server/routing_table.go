package server

import (
  "google.golang.org/grpc"
)

type RoutingTable struct {
  unionizedEntries  []BloomFilter,         // Union of bloom filters for every i
  bloomFilterMap    map[string]BloomFilter // Bloom filter map with key as the node meta id
}

func NewRT() *RoutingTable {
  routingTable := RoutingTable{
    unionizedEntries: make([]BloomFilter)
    bloomFilterMap: make(map[string]BloomFilter)
  }
  return &routingTable
}

func (rt *RoutingTable) AddEntry(bf BloomFilter) {
  // Add a new entry to the routing table
  rt.unionizedEntries = append(rt.unionizedEntries, bf)
}

func (rt *RoutingTable) UpdateEntry(bf BloomFilter, i int) {
  // Updae an entry to the routing table.
  rt.unionizedEntries[i] = bf
}

func (rt * RoutingTable) UnionBloomFilters(num_hops uint64) {
  // Get the union of all bloom filters num_hops away
}