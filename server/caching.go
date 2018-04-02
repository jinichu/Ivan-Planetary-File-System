package server

import (
	"fmt"
	"github.com/dgraph-io/badger"
	"math/rand"
	"time"
)

const MAX_SIZE = 100
const CACHE_SIZE = 10

type CacheSettings struct {
	MAX_SIZE   int
	CACHE_SIZE int
}

func (s *Server) LRUCache() {
	// Iterate. Key only iteration for now.
	// Check the number of keys in the local db
	var numKeys = 0
	keys := [][]byte{}
	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			k := item.Key()
			keys = append(keys, k)
			numKeys += 1
			fmt.Printf("key=%s\n", k)
		}
		return nil
	})

	if err != nil {
		fmt.Printf("error in cache function")
	}

	// Do something about eviction.
	// If number of keys is the maximum size,
	// then an insertion of a document will overfill cache?
	if numKeys == MAX_SIZE {
		candidateKeys := [][]byte{}
		// seed every time so indices aren't deterministic.
		rand.Seed(time.Now().UnixNano())

		// Randomly select 10 keys from our db
		for len(candidateKeys) < CACHE_SIZE {
			randIndex := rand.Intn(100)
			candidateKeys = append(candidateKeys, keys[randIndex])
		}

		// Fetch items from the database
		s.db.View(func(txn *badger.Txn) error {
			//TODO: inspect this and the iterator.
			it := txn.NewIterator(badger.DefaultIteratorOptions)
			defer it.Close()

			for _, key := range candidateKeys {
				for it.Seek(key); it.ValidForPrefix(key); it.Next() {
					item := it.Item()
					k := item.Key()
					//TODO: fill an array with the key,value pairs and select one to evict.
					v, err := item.Value()
					if err != nil {
						return err
					}
					fmt.Printf("key=%s, value=%s\n", k, v)
				}
			}

			return nil
		})
	}

	// TODO: what do we do about the new item wwe want to 'cache' locally.
	// Something with the file body, locally storing it

}
