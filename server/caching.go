package server

import (
	"fmt"

	"proj2_f5w9a_h6v9a_q7w9a_r8u8_w1c0b/serverpb"

	"github.com/dgraph-io/badger"
	"math/rand"
	"time"
)

const (
	MAX_BYTE_CAPACITY     = 100
	CACHE_CANDIDATES_SIZE = 10
)

type CacheKV struct {
	key   []byte
	value []byte //Is a marshalled serverpb.CacheMeta
}

//Method that is called to cache an item
func (s *Server) LRUCache(body *serverpb.CacheMeta) error {

	lsm_size, _ := s.db.Size()
	for lsm_size > MAX_BYTE_CAPACITY {
		//perform cache eviction
		err := s.cacheEvict()
		if err != nil {
			return err
		}
		lsm_size, _ = s.db.Size()
	}

	//call code to add to cache
	err := s.AddToCache(body)

	if err != nil {
		fmt.Printf("error in cache function")
	}
}

func (s *Server) AddToCache(body *serverpb.CacheMeta) error {
	//GetRemoteFile

	//TODO: item in cache has a timestamp
	currTime := time.Now().UnixNano()
	body.Marshal()
	if err := s.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte("/client/%s"), docId)
	}); err != nil {
		return err
	}

	// TODO: what do we do about the new item we want to 'cache' locally.
	// Something with the file body, locally storing it

	return nil
}

// When the DB is too big, start deleting things from /cache/ . Key points to /documents
func (s *Server) cacheEvict() error {
	// Iterate through keys, select 10 keys.
	// Key only iteration for now.
	// Check the number of keys in the local db.

	cache := []CacheKV{}
	var numKeys = 0
	// Iterate over cache key prefix
	s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		prefix := []byte("/cache/")
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()

			k := item.Key()
			v, err := item.Value()
			if err != nil {
				return err
			}
			cacheItem := CacheKV{k, v}

			cache = append(cache, cacheItem)
			numKeys += 1
		}
		return nil
	})

	// randomly selects 10 candidates from cache
	rand.NewSource(time.Now().UnixNano())
	var candidates []int
	for len(candidates) < CACHE_CANDIDATES_SIZE {
		candidates = append(candidates, rand.Intn(numKeys))
	}

	// iterate through elements, find the oldest one
	for _, index := range candidates {
		item := cache[index]
		item.key.Unmarshal()
		item.value.Unmarshal()
	}

	//TODO: Txn.Delete() to delete a key from the cache/documents

}
