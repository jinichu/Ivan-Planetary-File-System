package integration

import (
	"io/ioutil"
	"log"
	"os"
	"testing"
	"time"

	"proj2_f5w9a_h6v9a_q7w9a_r8u8_w1c0b/server"
	"proj2_f5w9a_h6v9a_q7w9a_r8u8_w1c0b/serverpb"
	"proj2_f5w9a_h6v9a_q7w9a_r8u8_w1c0b/util"

	"github.com/fatih/color"
	"github.com/pkg/errors"
)

func init() {
	log.SetFlags(log.Flags() | log.Lshortfile)
	color.NoColor = false
}

type cluster struct {
	t     *testing.T
	Nodes []*server.Server
	Dirs  []string
}

func NewTestCluster(t *testing.T, n int, opts ...func(*serverpb.NodeConfig)) *cluster {
	server.RoutingTableInterval = 200 * time.Millisecond

	c := cluster{
		t: t,
	}
	for i := 0; i < n; i++ {
		c.AddNode(opts...)
	}

	var meta serverpb.NodeMeta
	var err error
	util.SucceedsSoon(t, func() error {
		meta, err = c.Nodes[0].NodeMeta()
		if err != nil {
			return err
		}
		if len(meta.Addrs) == 0 {
			return errors.Errorf("no address")
		}
		return nil
	})

	for _, node := range c.Nodes[1:] {
		util.SucceedsSoon(t, func() error {
			meta, err := node.NodeMeta()
			if err != nil {
				return err
			}
			if len(meta.Addrs) == 0 {
				return errors.Errorf("no address")
			}
			return nil
		})
		if err := node.AddNode(meta); err != nil {
			t.Fatalf("%+v", err)
		}
	}

	return &c
}

func (c *cluster) AddNode(opts ...func(*serverpb.NodeConfig)) *server.Server {
	dir, err := ioutil.TempDir("", "ipfs-cluster-test")
	if err != nil {
		c.t.Fatalf("%+v", err)
	}
	config := serverpb.NodeConfig{
		Path:     dir,
		MaxPeers: 10,
	}
	for _, f := range opts {
		f(&config)
	}
	s, err := server.New(config)
	if err != nil {
		c.t.Fatalf("%+v", err)
	}
	c.Nodes = append(c.Nodes, s)
	c.Dirs = append(c.Dirs, dir)

	go func() {
		if err := s.Listen(":0"); err != nil {
			c.t.Errorf("%+v", err)
		}
	}()

	return s
}

func (c *cluster) Close() {
	for _, s := range c.Nodes {
		if err := s.Close(); err != nil {
			c.t.Errorf("%+v", err)
		}
	}

	for _, dir := range c.Dirs {
		if err := os.RemoveAll(dir); err != nil {
			c.t.Errorf("%+v", err)
		}
	}
}
