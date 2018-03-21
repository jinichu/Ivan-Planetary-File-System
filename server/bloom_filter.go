package server

import (
"github.com/spaolacci/murmur3"
)

type BloomFilter struct {
  bit_array []bool
  k           uint
  m           uint
}

func NewBF(size, num_hash_funcs uint) *BloomFilter {
  bloomFilter := BloomFilter{
    bit_array: make([]bool, size),
    k: num_hash_funcs,
    m: size,
  }

  return &bloomFilter
}

func (bloomFilter *BloomFilter) Add(item []byte) {
  for i := 0; i < int(bloomFilter.k); i++ {
    hash := bloomFilter.getHashValue(item, i)
    pos := uint(hash) % bloomFilter.m
    bloomFilter.bit_array[uint(pos)] = true
  }
}

func (bloomFilter *BloomFilter) Check(item []byte) (exists bool) {
  for i:=0; i < int(bloomFilter.k); i++ {
    hash := bloomFilter.getHashValue(item, i)
    pos := uint(hash) % bloomFilter.m
    if !bloomFilter.bit_array[uint(pos)] {
      return false
    }
  }
  return true
}

func (bloomFilter *BloomFilter) getHashValue(item []byte, i int) uint64  {
  hash_func := murmur3.New64WithSeed(uint32(i))
  hash_func.Write(item)
  res := hash_func.Sum64()

  return res
}