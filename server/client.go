package server

import (
	"context"
	"crypto/sha1"
	"encoding/asn1"
	"encoding/base64"
	"fmt"
	"proj2_f5w9a_h6v9a_q7w9a_r8u8_w1c0b/serverpb"
	"time"

	"github.com/dgraph-io/badger"
	"github.com/pkg/errors"
)

func (s *Server) Get(ctx context.Context, in *serverpb.GetRequest) (*serverpb.GetResponse, error) {
	var f serverpb.Document
	if err := s.db.View(func(txn *badger.Txn) error {
		key := fmt.Sprintf("/document/%s", in.DocumentId)
		item, err := txn.Get([]byte(key))
		if err != nil {
			return err
		}
		body, err := item.Value()
		if err != nil {
			return err
		}
		if err := f.Unmarshal(body); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return nil, err
	}

	resp := &serverpb.GetResponse{
		Document: &f,
	}

	return resp, nil
}

func (s *Server) Add(ctx context.Context, in *serverpb.AddRequest) (*serverpb.AddResponse, error) {
	doc := in.GetDocument()
	if doc == nil {
		return nil, errors.New("missing Document")
	}
	b, err := doc.Marshal()
	if err != nil {
		return nil, err
	}

	data := sha1.Sum(b)
	hash := base64.StdEncoding.EncodeToString(data[:])

	if err := s.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(fmt.Sprintf("/document/%s", hash)), b)
	}); err != nil {
		return nil, err
	}

	resp := &serverpb.AddResponse{
		DocumentId: hash,
	}

	return resp, nil
}

func (s *Server) AddDirectory(ctx context.Context, in *serverpb.AddDirectoryRequest) (*serverpb.AddDirectoryResponse, error) {
	resp := &serverpb.AddDirectoryResponse{}
	return resp, nil
}

func (s *Server) GetPeers(ctx context.Context, in *serverpb.GetPeersRequest) (*serverpb.GetPeersResponse, error) {
	var peers []*serverpb.NodeMeta
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, v := range s.mu.peerMeta {
		peers = append(peers, &v)
	}

	resp := &serverpb.GetPeersResponse{
		Peers: peers,
	}

	return resp, nil
}

func (s *Server) AddPeer(ctx context.Context, in *serverpb.AddPeerRequest) (*serverpb.AddPeerResponse, error) {
	err := s.BootstrapAddNode(in.GetAddr())
	if err != nil {
		return nil, err
	}
	resp := &serverpb.AddPeerResponse{}
	return resp, nil
}

func (s *Server) GetReference(ctx context.Context, in *serverpb.GetReferenceRequest) (*serverpb.GetReferenceResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if reference, ok := s.mu.references[in.GetReferenceId()]; ok {
		resp := &serverpb.GetReferenceResponse{
			Reference: &reference,
		}
		return resp, nil
	}
	// TODO: Do a network lookup for this reference
	resp := &serverpb.GetReferenceResponse{}
	return resp, nil
}

func (s *Server) AddReference(ctx context.Context, in *serverpb.AddReferenceRequest) (*serverpb.AddReferenceResponse, error) {
	privKey, err := LoadPrivate(in.GetPrivKey())
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
	reference := &serverpb.Reference{
		Value:     in.GetRecord(),
		PublicKey: pubKey,
		Timestamp: time.Now().Unix(),
	}
	bytes, err := reference.Marshal()
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
	reference.Signature = base64.StdEncoding.EncodeToString(sig)

	// Add this reference locally
	s.mu.Lock()
	defer s.mu.Unlock()

	referenceId, err := Hash(reference.PublicKey)
	if err != nil {
		return nil, err
	}
	s.mu.references[referenceId] = *reference
	// TODO: Diseminate this reference to the rest of the network
	resp := &serverpb.AddReferenceResponse{
		ReferenceId: referenceId,
	}
	return resp, nil
}
