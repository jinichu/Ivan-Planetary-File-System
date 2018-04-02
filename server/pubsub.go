package server

import (
	"context"
	"encoding/asn1"
	"encoding/base64"
	"fmt"
	"proj2_f5w9a_h6v9a_q7w9a_r8u8_w1c0b/serverpb"
	"time"

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

func (s *Server) Publish(ctx context.Context, req *serverpb.PublishRequest) (*serverpb.PublishResponse, error) {
	privKey, err := LoadPrivate(req.GetPrivKey())
	if err != nil {
		fmt.Println(err)
		return nil, err
	}
	pubKey, err := MarshalPublic(&privKey.PublicKey)
	if err != nil {
		fmt.Println(err)
		return nil, err
	}
	// Create reference
	msg := &serverpb.Message{
		Message:   req.GetMessage(),
		PublicKey: pubKey,
		Timestamp: time.Now().Unix(),
	}
	bytes, err := msg.Marshal()
	if err != nil {
		fmt.Println(err)
		return nil, err
	}
	r, s1, err := Sign(bytes, *privKey)
	if err != nil {
		fmt.Println(err)
		return nil, err
	}
	sig, err := asn1.Marshal(EcdsaSignature{R: r, S: s1})
	if err != nil {
		fmt.Println(err)
		return nil, err
	}
	msg.Signature = base64.StdEncoding.EncodeToString(sig)
	referenceId, err := Hash(msg.PublicKey)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	ch, ok := s.mu.channels[referenceId]
	if !ok {
		return &serverpb.PublishResponse{
			Listeners: 0,
		}, nil
	}

	listeners := int32(0)

	for _, c := range ch.listeners {
		// attempt to write messages to all listeners, but drop message if blocked
		select {
		case c <- msg:
			listeners++
		default:
		}
	}

	return &serverpb.PublishResponse{
		Listeners: listeners,
	}, nil
}

func (s *Server) SubscribeClient(req *serverpb.SubscribeRequest, stream serverpb.Client_SubscribeClientServer) error {
	return s.Subscribe(req, stream)
}
