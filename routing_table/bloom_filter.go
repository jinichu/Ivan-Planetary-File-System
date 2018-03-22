package routing_table

import (
"github.com/spaolacci/murmur3"
)

// BloomFilter probabilistic data structure definition
type BloomFilter struct {
  bit_array []bool
  k           uint
  m           uint
}

// Returns a new BloomFilter object,
func New(size, num_hash_funcs uint) *BloomFilter {
  return &BloomFilter{
    bit_array: make([]bool, size),
    k: num_hash_funcs,
    m: size,
  }
}

func (bloomFilter *BloomFilter) Add(item []byte) {
  for i := 0; i < int(bloomFilter.k); i++ {
    hash := bloomFilter.hashValues(item, i)
    pos := uint(hash) % bloomFilter.m
    bloomFilter.bit_array[uint(pos)] = true
  }
}

func (bloomFilter *BloomFilter) Check(item []byte) (exists bool) {
  for i:=0; i < int(bloomFilter.k); i++ {
    hash := bloomFilter.hashValues(item, i)
    pos := uint(hash) % bloomFilter.m
    if !bloomFilter.bit_array[uint(pos)] {
      return false
    }
  }
  return true
}

func (bloomFilter *BloomFilter) hashValues(item []byte, i int) uint64  {
  hashFunc := murmur3.New64WithSeed(uint32(i))
  hashFunc.Write(item)
  res := hashFunc.Sum64()
  hashFunc.Reset()

  return res
}