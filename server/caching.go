package server

import (
	"bytes"
	"fmt"
	"time"

	"proj2_f5w9a_h6v9a_q7w9a_r8u8_w1c0b/serverpb"

	"github.com/dgraph-io/badger"
)

type CacheKV struct {
	key   []byte
	value serverpb.CacheMeta
}

//Method that is called to cache an item
func (s *Server) LRUCache(remoteFile *serverpb.GetRemoteFileResponse, docID string) error {

	// Store sizes -- delete items until cache under size, then GC ONCE.
	lsm_size, vallog_size := s.db.Size()
	db_size := lsm_size + vallog_size
	for db_size > s.config.CacheSize {
		// Check if cache empty before evicting.
		numKeys, err := s.checkCacheNumKeys()
		if err != nil {
			return err
		}
		if numKeys == 0 {
			return nil
		}
		//perform cache eviction
		savings, err := s.cacheEvict()
		if err != nil {
			return err
		}
		db_size = db_size - savings
	}

	s.db.RunValueLogGC(0.5)

	//call code to add to cache
	err := s.AddToCache(remoteFile, docID)

	if err != nil {
		return err
	}

	return nil
}

func (s *Server) AddToCache(remoteFile *serverpb.GetRemoteFileResponse, docID string) error {
	sizeOfItem := remoteFile.Size()
	currTime := time.Now().UnixNano()

	cacheItem := serverpb.CacheMeta{Sizeofdoc: int64(sizeOfItem), LastAccessed: currTime}
	cacheValue, err := cacheItem.Marshal()
	if err != nil {
		return err
	}

	if err := s.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(fmt.Sprintf("/client/%s", docID)), cacheValue)
	}); err != nil {
		return err
	}

	if err := s.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(fmt.Sprintf("/document/%s", docID)), remoteFile.Body)
	}); err != nil {
		return err
	}

	return nil
}

// When the DB is too big, start deleting things from /cache/ . Key points to /documents
func (s *Server) cacheEvict() (int64, error) {

	candidates := []CacheKV{}
	// Iterate over cache key prefix
	if err := s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		randKey, err := GenerateAESKey()
		if err != nil {
			return err
		}
		// call iterator once, then call next 10 times
		prefix := []byte("/cache/")
		start := []byte(fmt.Sprintf("%s%s", prefix, randKey))

		it.Seek(start)
		for i := 0; i < int(s.config.CacheSample); i++ {
			it.Next()
			if !it.ValidForPrefix(prefix) {
				break
			}
			item := it.Item()
			k := item.Key()
			v, err := item.Value()
			if err != nil {
				return err
			}
			var cacheItem serverpb.CacheMeta
			cacheItem.Unmarshal(v)
			candidates = append(candidates, CacheKV{k, cacheItem})
		}
		return nil
	}); err != nil {
		return 0, err
	}

	oldestItem := CacheKV{}
	oldestTime := time.Now().UnixNano()
	// iterate through elements, find the oldest one
	for _, cacheItem := range candidates {
		if cacheItem.value.LastAccessed < oldestTime {
			oldestTime = cacheItem.value.LastAccessed
			oldestItem = cacheItem
		}
	}

	if err := s.db.View(func(txn *badger.Txn) error {
		err := txn.Delete(oldestItem.key)
		if err != nil {
			return err
		}
		return nil
	}); err != nil {
		return 0, err
	}

	docId := bytes.TrimPrefix(oldestItem.key, []byte("/cache/"))
	prefix := []byte("/document/")

	docKey := []byte(fmt.Sprintf("%s%s", prefix, docId))
	if err := s.db.View(func(txn *badger.Txn) error {
		err := txn.Delete(docKey)
		if err != nil {
			return err
		}
		return nil
	}); err != nil {
		return 0, err
	}

	return oldestItem.value.Sizeofdoc, nil
}

func (s *Server) checkCacheNumKeys() (int, error) {
	numKeys := 0
	if err := s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		prefix := []byte("/cache/")
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			numKeys += 1
		}
		return nil
	}); err != nil {
		return 0, err
	}

	return numKeys, nil
}
