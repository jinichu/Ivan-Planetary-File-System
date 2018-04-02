package server

import (
	"fmt"
	"proj2_f5w9a_h6v9a_q7w9a_r8u8_w1c0b/serverpb"

	"github.com/dgraph-io/badger"
	"github.com/pkg/errors"
)

type channel struct {
	listeners map[int]chan *serverpb.Message
}

func (s *Server) listen(ref string) (<-chan *serverpb.Message, func()) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := s.mu.nextListenerID
	s.mu.nextListenerID++
	ch, ok := s.mu.channels[ref]
	if !ok {
		ch = &channel{
			listeners: map[int]chan *serverpb.Message{},
		}
		s.mu.channels[ref] = ch
	}

	c := make(chan *serverpb.Message, 10)
	ch.listeners[id] = c

	return c, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		delete(ch.listeners, id)
	}
}

func (s *Server) Subscribe(req *serverpb.SubscribeRequest, stream serverpb.Node_SubscribeServer) error {
	referenceID := req.GetChannelId()
	// Check to make sure corresponding reference exists.
	if err := s.db.View(func(txn *badger.Txn) error {
		key := fmt.Sprintf("/reference/%s", referenceID)
		_, err := txn.Get([]byte(key))
		return err
	}); err == badger.ErrKeyNotFound {
		if req.GetNumHops() == 0 {
			return errors.Wrapf(ErrNumHops, "referenceID: %s", referenceID)
		}

		// Look reference up via the network.
		routes := s.peersWithFile(referenceID)
		if len(routes) == 0 {
			return errors.Errorf("no routes to reference: %s", referenceID)
		}
		for _, route := range routes {
			numHops := req.GetNumHops()
			if numHops == -1 {
				numHops = route.NumHops
			}
			clientStream, err := route.Client.Subscribe(stream.Context(), &serverpb.SubscribeRequest{
				ChannelId: referenceID,
				Starting:  req.Starting,
				NumHops:   numHops,
			})
			if err != nil {
				s.log.Printf("failed to find file: %+v", err)
				continue
			}

			for {
				msg, err := clientStream.Recv()
				if err != nil {
					return err
				}
				if err := stream.Send(msg); err != nil {
					return err
				}
			}
		}
		return errors.Errorf("failed to find reference: %s", referenceID)
	} else if err != nil {
		// Error wasn't an error relating to the reference not being found locally. Return.
		return err
	}

	channel, cleanup := s.listen(referenceID)
	defer cleanup()

	for msg := range channel {
		if err := stream.Send(msg); err != nil {
			return err
		}
	}
	return nil
}
