package integration

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	mrand "math/rand"
	"proj2_f5w9a_h6v9a_q7w9a_r8u8_w1c0b/server"
	"proj2_f5w9a_h6v9a_q7w9a_r8u8_w1c0b/serverpb"
	"proj2_f5w9a_h6v9a_q7w9a_r8u8_w1c0b/util"
	"reflect"
	"sync"
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
	if err := s.BootstrapAddNode(nil, meta.Addrs[0]); err != nil {
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

func generatePrivateKey(t *testing.T) []byte {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM := pem.EncodeToMemory(server.PemBlockForKey(priv))
	return keyPEM
}

func TestClusterFetchReference(t *testing.T) {
	const nodes = 5
	ts := NewTestCluster(t, nodes)
	defer ts.Close()

	ctx := context.Background()

	files := map[string]string{}

	for i, node := range ts.Nodes {
		key := generatePrivateKey(t)
		data := fmt.Sprintf("Reference from node %d", i)
		resp, err := node.AddReference(ctx, &serverpb.AddReferenceRequest{
			PrivKey: key,
			Record:  data,
		})
		if err != nil {
			t.Fatal(err)
		}
		files[resp.ReferenceId] = data

		// Make sure local node has the file.
		{
			resp, err := node.GetReference(ctx, &serverpb.GetReferenceRequest{
				ReferenceId: resp.ReferenceId,
			})
			if err != nil {
				t.Fatal(err)
			}
			if resp.Reference.Value != data {
				t.Fatalf("%d. got %+v; wanted %+v", i, resp.Reference.Value, data)
			}
		}
	}

	// Check to make sure all nodes can access other nodes references.
	for i, node := range ts.Nodes {
		for referenceID, doc := range files {
			util.SucceedsSoon(t, func() error {
				resp, err := node.GetReference(ctx, &serverpb.GetReferenceRequest{
					ReferenceId: referenceID,
				})
				if err != nil {
					return errors.Wrapf(err, "fetching reference %q, from node %d: %s", referenceID, i, doc)
				}
				if resp.Reference.Value != doc {
					return errors.Errorf("%d. got %+v; wanted %+v", i, resp, doc)
				}
				return nil
			})
		}
	}
}

func TestClusterPubSub(t *testing.T) {
	const nodes = 5
	ts := NewTestCluster(t, nodes)
	defer ts.Close()

	ctx := context.Background()

	i := mrand.Intn(len(ts.Nodes))
	t.Logf("node picked to publish: %d", i)
	node := ts.Nodes[i]

	key := generatePrivateKey(t)
	data := fmt.Sprintf("Reference on node %d", i)
	resp, err := node.AddReference(ctx, &serverpb.AddReferenceRequest{
		PrivKey: key,
		Record:  data,
	})
	if err != nil {
		t.Fatal(err)
	}

	referenceID := resp.ReferenceId

	var mu struct {
		sync.Mutex

		seen map[int]*serverpb.Message
	}
	mu.seen = map[int]*serverpb.Message{}

	// Check to make sure all nodes can access other nodes references.
	for i, node := range ts.Nodes {
		util.SucceedsSoon(t, func() error {
			_, err := node.GetReference(ctx, &serverpb.GetReferenceRequest{
				ReferenceId: referenceID,
			})
			return err
		})

		conn, err := node.LocalConn()
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()

		client := serverpb.NewClientClient(conn)
		stream, err := client.SubscribeClient(ctx, &serverpb.SubscribeRequest{
			ChannelId: referenceID,
		})
		if err != nil {
			t.Fatal(err)
		}

		i := i

		go func() {
			msg, err := stream.Recv()
			if err != nil {
				t.Fatal(err)
			}

			mu.Lock()
			defer mu.Unlock()

			mu.seen[i] = msg
		}()
	}

	util.SucceedsSoon(t, func() error {
		n := node.NumListeners(referenceID)
		if n != len(ts.Nodes) {
			return errors.Errorf("NumListeners() = %d; not %d", n, len(ts.Nodes))
		}
		return nil
	})

	msg := "some message woo"
	{
		resp, err := node.Publish(ctx, &serverpb.PublishRequest{
			PrivKey: key,
			Message: msg,
		})
		if err != nil {
			t.Fatal(err)
		}
		if resp.Listeners != int32(len(ts.Nodes)) {
			t.Fatalf("Publish sent to %d listeners; not %d", resp.Listeners, len(ts.Nodes))
		}
	}

	util.SucceedsSoon(t, func() error {
		mu.Lock()
		defer mu.Unlock()

		if len(mu.seen) != len(ts.Nodes) {
			return errors.Errorf("len seen != len Nodes; %#v", mu.seen)
		}

		for i, got := range mu.seen {
			if got.Message != msg {
				return errors.Errorf("%d. message = %+v; want %q", i, got, msg)
			}
		}
		return nil
	})
}
