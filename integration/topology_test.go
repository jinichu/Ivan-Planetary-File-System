package integration

import (
	"path"
	"proj2_f5w9a_h6v9a_q7w9a_r8u8_w1c0b/util"
	"reflect"
	"runtime"
	"testing"

	"github.com/pkg/errors"
)

var DefaultTopologies = []Topology{TopologyStar, TopologyFullyConnected, TopologyLine}

type Topology func(c *cluster)

func TopologyStar(c *cluster) {
	meta, err := c.Nodes[0].NodeMeta()
	if err != nil {
		c.t.Fatal(err)
	}
	for _, node := range c.Nodes[1:] {
		if err := node.AddNode(meta, true); err != nil {
			c.t.Fatalf("%+v", err)
		}
	}

	util.SucceedsSoon(c.t, func() error {
		got := c.Nodes[0].NumConnections()
		want := len(c.Nodes) - 1
		if want != got {
			return errors.Errorf("expected %d connections; got %d", want, got)
		}
		return nil
	})

	for _, node := range c.Nodes[1:] {
		util.SucceedsSoon(c.t, func() error {
			got := node.NumConnections()
			want := 1
			if want != got {
				return errors.Errorf("expected %d connections; got %d", want, got)
			}
			return nil
		})
	}
}

// TopologyLooselyConnected is a star topology without any strict checks or
// forced connections. In most cases this will bootstrap into a fully connected
// topology.
func TopologyLooselyConnected(c *cluster) {
	meta, err := c.Nodes[0].NodeMeta()
	if err != nil {
		c.t.Fatal(err)
	}
	for _, node := range c.Nodes[1:] {
		if err := node.AddNode(meta, false); err != nil {
			c.t.Fatalf("%+v", err)
		}
	}
}

func TopologyFullyConnected(c *cluster) {
	for i, node := range c.Nodes {
		meta, err := node.NodeMeta()
		if err != nil {
			c.t.Fatal(err)
		}
		for _, node := range c.Nodes[i+1:] {
			if err := node.AddNode(meta, true); err != nil {
				c.t.Fatalf("%+v", err)
			}
		}
	}

	for _, node := range c.Nodes {
		util.SucceedsSoon(c.t, func() error {
			got := node.NumConnections()
			want := len(c.Nodes) - 1
			if want != got {
				return errors.Errorf("expected %d connections; got %d", want, got)
			}
			return nil
		})
	}
}

func TopologyLine(c *cluster) {
	for i, node := range c.Nodes[1:] {
		meta, err := c.Nodes[i].NodeMeta()
		if err != nil {
			c.t.Fatal(err)
		}
		if err := node.AddNode(meta, true); err != nil {
			c.t.Fatalf("%+v", err)
		}
	}

	for i, node := range c.Nodes {
		util.SucceedsSoon(c.t, func() error {
			got := node.NumConnections()
			want := 2
			if i == 0 || i == len(c.Nodes)-1 {
				want = 1
			}
			if want != got {
				return errors.Errorf("expected %d connections; got %d", want, got)
			}
			return nil
		})
	}
}

func MultiTopologyTest(
	t *testing.T, topos []Topology, n int, f func(t *testing.T, ts *cluster), opts ...func(*cluster),
) {
	if len(topos) == 0 {
		t.Fatal("no topologies specified")
	}
	for _, topo := range topos {
		name := path.Base(runtime.FuncForPC(reflect.ValueOf(topo).Pointer()).Name())
		t.Run(name, func(t *testing.T) {
			topts := append([]func(*cluster){
				func(c *cluster) {
					c.Topology = topo
				},
				func(c *cluster) {
					c.NodeConfig.MaxPeers = 0
				},
			}, opts...)
			ts := NewTestCluster(t, n, topts...)
			defer ts.Close()

			f(t, ts)
		})
	}
}
