package server

import (
	"context"
	"encoding/base64"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"path/filepath"
	"proj2_f5w9a_h6v9a_q7w9a_r8u8_w1c0b/serverpb"
	"sort"
	"strings"

	"github.com/dgraph-io/badger"
	"github.com/pkg/errors"
)

func (s *Server) setupHTTP() {
	s.mux.HandleFunc("/badger/", httpErr(s.httpBadger))
	s.mux.HandleFunc("/document/", httpErr(s.httpDocument))
	s.mux.HandleFunc("/", httpErr(s.httpIndex))
}

func httpErr(f func(w http.ResponseWriter, r *http.Request) error) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := f(w, r); err != nil {
			http.Error(w, fmt.Sprintf("%+v", err), 500)
		}
	})
}

func (s *Server) httpIndex(w http.ResponseWriter, r *http.Request) error {
	fmt.Fprintf(w, "<h1>Welcome to Ivan Planetary File System!</h1>")

	fmt.Fprintf(w, "<h2>Badger keys:</h2>")
	if err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchSize = 10
		it := txn.NewIterator(opts)
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			k := item.Key()
			key := base64.URLEncoding.EncodeToString(k)
			fmt.Fprintf(w, `<li><a href="/badger/%s/%s">%s</a></li>`, k, key, html.EscapeString(string(k)))
		}
		return nil
	}); err != nil {
		return err
	}

	return nil
}

func (s *Server) httpBadger(w http.ResponseWriter, r *http.Request) error {
	key, err := base64.URLEncoding.DecodeString(filepath.Base(r.URL.Path))
	if err != nil {
		return err
	}
	if err := s.db.View(func(txn *badger.Txn) error {
		kv, err := txn.Get(key)
		if err != nil {
			return err
		}
		val, err := kv.Value()
		if err != nil {
			return err
		}
		if _, err := w.Write(val); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}
	return nil
}

func (s *Server) httpDocument(w http.ResponseWriter, r *http.Request) error {
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 3 {
		return errors.Errorf("must have 2 slashes")
	}
	s.log.Printf("GET %s", r.URL.Path)
	doc, err := s.resolveDoc(r.Context(), parts[2], parts[3:])
	if err != nil {
		return err
	}
	if doc.ContentType == "directory" {
		fmt.Fprintf(w, "<h1>%s</h1>", r.URL.Path)
		var keys []string
		for k := range doc.Children {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, key := range keys {
			fmt.Fprintf(w, `<li><a href="%s">%s</a></li>`, url.QueryEscape(key), html.EscapeString(key))
		}
	} else {
		w.Header().Set("Content-Type", doc.GetContentType())
		w.Write(doc.GetData())
	}
	return nil
}

func (s *Server) resolveDoc(ctx context.Context, id string, path []string) (*serverpb.Document, error) {
	resp, err := s.Get(ctx, &serverpb.GetRequest{
		AccessId: id,
	})
	if err != nil {
		return nil, err
	}
	doc := resp.GetDocument()
	if len(path) == 0 || len(path[0]) == 0 {
		if doc.ContentType == "directory" {
			index, ok := doc.Children["index.html"]
			if ok {
				return s.resolveDoc(ctx, index, nil)
			}
		}
		return doc, nil
	}
	key := path[0]
	child, ok := doc.Children[key]
	if !ok {
		return nil, errors.Errorf("doc %q missing child %q", doc, key)
	}
	return s.resolveDoc(ctx, child, path[1:])
}
