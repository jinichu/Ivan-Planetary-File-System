package integration

import (
	"context"
	"fmt"
	"proj2_f5w9a_h6v9a_q7w9a_r8u8_w1c0b/serverpb"
	"proj2_f5w9a_h6v9a_q7w9a_r8u8_w1c0b/util"
	"reflect"
	"testing"

	"github.com/pkg/errors"
)

func TestSimpleCluster(t *testing.T) {
	ts := NewTestCluster(t, 1)
	defer ts.Close()
}

func TestCluster(t *testing.T) {
	const nodes = 5
	ts := NewTestCluster(t, nodes)
	defer ts.Close()

	for i, node := range ts.Nodes {
		util.SucceedsSoon(t, func() error {
			got := node.NumConnections()
			want := nodes - 1
			if got != want {
				return errors.Errorf("%d. expected %d connections; got %d", i, want, got)
			}
			return nil
		})
	}
}

func TestClusterMaxPeers(t *testing.T) {
	const nodes = 5
	ts := NewTestCluster(t, nodes, func(c *serverpb.NodeConfig) {
		c.MaxPeers = 3
	})
	defer ts.Close()

	for i, node := range ts.Nodes {
		util.SucceedsSoon(t, func() error {
			got := node.NumConnections()
			want := 3
			if got != want {
				return errors.Errorf("%d. expected %d connections; got %d", i, want, got)
			}
			return nil
		})
	}
}

func TestBootstrapAddNode(t *testing.T) {
	ts := NewTestCluster(t, 1)
	defer ts.Close()

	s := ts.AddNode()
	meta, err := ts.Nodes[0].NodeMeta()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.BootstrapAddNode(meta.Addrs[0]); err != nil {
		t.Fatal(err)
	}

	for i, node := range ts.Nodes {
		util.SucceedsSoon(t, func() error {
			got := node.NumConnections()
			want := 1
			if got != want {
				return errors.Errorf("%d. expected %d connections; got %d", i, want, got)
			}
			return nil
		})
	}
}

func TestClusterFetchDocument(t *testing.T) {
	const nodes = 5
	ts := NewTestCluster(t, nodes)
	defer ts.Close()

	ctx := context.Background()

	files := map[string]serverpb.Document{}

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
		files[resp.AccessId] = doc

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

	// Check to make sure all nodes can access other nodes files.
	for i, node := range ts.Nodes {
		for accessID, doc := range files {
			util.SucceedsSoon(t, func() error {
				resp, err := node.Get(ctx, &serverpb.GetRequest{
					AccessId: accessID,
				})
				if err != nil {
					return errors.Wrapf(err, "fetching document %q, from node %d: %s", accessID, i, doc.Data)
				}
				if !reflect.DeepEqual(resp.Document, &doc) {
					return errors.Errorf("%d. got %+v; wanted %+v", i, resp.Document, &doc)
				}
				return nil
			})
		}
	}
}
