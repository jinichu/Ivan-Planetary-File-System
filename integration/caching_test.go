package integration

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"proj2_f5w9a_h6v9a_q7w9a_r8u8_w1c0b/server"
	"proj2_f5w9a_h6v9a_q7w9a_r8u8_w1c0b/serverpb"
	"proj2_f5w9a_h6v9a_q7w9a_r8u8_w1c0b/util"

	"github.com/dgraph-io/badger"
	"github.com/pkg/errors"
)

type NodeDocRef struct {
	AccessID string
	Doc      serverpb.Document
}

//Test cache, run the caching and check that the values are present.
func TestServer_LRUCache(t *testing.T) {
	const nodes = 5

	linearTopo := []Topology{TopologyLine}

	MultiTopologyTest(t, linearTopo, nodes, func(t *testing.T, ts *cluster) {
		ctx := context.Background()
		nodeDocs := map[int]NodeDocRef{}

		for i, node := range ts.Nodes {
			doc := serverpb.Document{
				Data:        []byte(fmt.Sprintf("Document from node %d", i)),
				ContentType: "text/plain",
			}
			resp, err := node.Add(ctx, &serverpb.AddRequest{
				Document: &doc,
			})
			if err != nil {
				t.Fatal(err)
			}

			nodeDocs[i] = NodeDocRef{AccessID: resp.AccessId, Doc: doc}

			// Make sure local node has the file.
			{
				resp, err := node.Get(ctx, &serverpb.GetRequest{
					AccessId: resp.AccessId,
				})
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(resp.Document, &doc) {
					t.Fatalf("%d. got %+v; wanted %+v", i, resp.Document, &doc)
				}
			}
		}

		// Linear topology - 1-2-3-4-5
		// regular cache add/eviction
		// Get node 1 to fetch from node 3 -- 2 should have it locally
		util.SucceedsSoon(t, func() error {
			resp, err := ts.Nodes[0].Get(ctx, &serverpb.GetRequest{
				AccessId: nodeDocs[2].AccessID,
			})
			if err != nil {
				return errors.Wrapf(err, "fetching document %q, from node %d: %s", nodeDocs[2].AccessID, 2, nodeDocs[2].Doc.Data)
			}
			doc := nodeDocs[2].Doc
			if !reflect.DeepEqual(resp.Document, &doc) {
				return errors.Errorf("%d. got %+v; wanted %+v", 2, resp.Document, &doc)
			}

			// Check that 2 has it locally.
			err = ts.Nodes[1].GetDB().View(func(txn *badger.Txn) error {
				docID, _, _ := server.SplitAccessID(nodeDocs[2].AccessID)
				key := fmt.Sprintf("/document/%s", docID)
				_, err := txn.Get([]byte(key))
				if err != nil {
					return errors.Wrapf(err, "Fetching Document %q, from self %d: %s", key, 1, nodeDocs[1].Doc.Data)
				}

				return nil
			})

			if err != nil {
				t.Fatal(err)
			}
			return nil
		})

		// documents is full and cache empty

		// Regular cache eviction

		// Run file gets here.

	}, func(c *cluster) {
		c.NodeConfig.CacheSize = 1000000
	})

}
