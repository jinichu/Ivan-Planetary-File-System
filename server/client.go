package server

import (
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/asn1"
	"encoding/base64"
	"fmt"
	"proj2_f5w9a_h6v9a_q7w9a_r8u8_w1c0b/serverpb"
	"strings"
	"time"

	"github.com/dgraph-io/badger"
	"github.com/pkg/errors"
)

func SplitAccessID(id string) (string, []byte, error) {
	parts := strings.Split(id, ":")
	if len(parts) != 2 {
		return "", nil, errors.Errorf("AccessId should have a :")
	}
	documentID := parts[0]
	accessKey, err := base64.URLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", nil, err
	}
	return documentID, accessKey, nil
}

func (s *Server) Get(ctx context.Context, in *serverpb.GetRequest) (*serverpb.GetResponse, error) {
	documentId, accessKey, err := SplitAccessID(in.GetAccessId())
	if err != nil {
		return nil, err
	}
	respRemote, err := s.GetRemoteFile(ctx, &serverpb.GetRemoteFileRequest{
		DocumentId: documentId,
		NumHops:    -1, // -1 tells GetRemoteFile to infer it.
	})
	if err != nil {
		return nil, err
	}
	f, err := s.DecryptDocument(respRemote.Body, accessKey)
	if err != nil {
		s.log.Println("cannot decrypt document", err)
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
	encryptedDocument, key, err := s.EncryptDocument(*doc)
	if err != nil {
		return nil, err
	}

	hash := HashBytes(encryptedDocument)

	if err := s.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(fmt.Sprintf("/document/%s", hash)), encryptedDocument)
	}); err != nil {
		return nil, err
	}

	accessKey := base64.URLEncoding.EncodeToString(key)
	accessId := hash + ":" + accessKey
	resp := &serverpb.AddResponse{
		AccessId: accessId,
	}

	if err := s.addToRoutingTable(hash); err != nil {
		return nil, err
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
	err := s.BootstrapAddNode(ctx, in.GetAddr())
	if err != nil {
		return nil, err
	}
	resp := &serverpb.AddPeerResponse{}
	return resp, nil
}

func (s *Server) GetReference(ctx context.Context, in *serverpb.GetReferenceRequest) (*serverpb.GetReferenceResponse, error) {
	referenceID, accessKey, err := SplitAccessID(in.GetReferenceId())
	if err != nil {
		return nil, err
	}

	resp, err := s.GetRemoteReference(ctx, &serverpb.GetRemoteReferenceRequest{
		ReferenceId: referenceID,
		NumHops:     -1,
	})
	if err != nil {
		return nil, err
	}

	reference := resp.GetReference()
	value, err := DecryptBytes(accessKey, []byte(reference.GetValue()))
	if err != nil {
		return nil, err
	}
	reference.Value = string(value)

	return &serverpb.GetReferenceResponse{
		Reference: reference,
	}, nil
}

func (s *Server) AddReference(ctx context.Context, in *serverpb.AddReferenceRequest) (*serverpb.AddReferenceResponse, error) {
	privKey, err := LoadPrivate(in.GetPrivKey())
	if err != nil {
		return nil, err
	}
	pubKey, err := MarshalPublic(&privKey.PublicKey)
	if err != nil {
		return nil, err
	}

	key, err := GenerateAESKeyFromECDSA(privKey)
	if err != nil {
		return nil, err
	}
	encryptedValue, err := EncryptBytes(key, []byte(in.GetRecord()))
	if err != nil {
		return nil, err
	}

	// Create reference
	reference := &serverpb.Reference{
		Value:     string(encryptedValue),
		PublicKey: pubKey,
		Timestamp: time.Now().Unix(),
	}
	bytes, err := reference.Marshal()
	if err != nil {
		return nil, err
	}
	refHash := sha1.Sum(bytes)
	r, s1, err := Sign(refHash[:], *privKey)
	if err != nil {
		return nil, err
	}
	sig, err := asn1.Marshal(EcdsaSignature{R: r, S: s1})
	if err != nil {
		return nil, err
	}
	reference.Signature = base64.URLEncoding.EncodeToString(sig)

	// Add this reference locally
	referenceId, err := Hash(reference.PublicKey)
	if err != nil {
		return nil, err
	}
	b, err := reference.Marshal()
	if err != nil {
		return nil, err
	}

	if err := s.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(fmt.Sprintf("/reference/%s", referenceId)), b)
	}); err != nil {
		return nil, err
	}

	if err := s.addToRoutingTable(referenceId); err != nil {
		return nil, err
	}

	resp := &serverpb.AddReferenceResponse{
		ReferenceId: referenceId + ":" + base64.URLEncoding.EncodeToString(key),
	}
	return resp, nil
}

func (s *Server) EncryptDocument(doc serverpb.Document) (encryptedData []byte, key []byte, err error) {
	// Create a new SHA256 handler
	shaHandler := sha256.New()

	marshalledData, err := doc.Marshal()
	if err != nil {
		return nil, nil, err
	}

	// Attempt to write the data to the SHA1 handler (is this right?)
	if _, err := shaHandler.Write(marshalledData); err != nil {
		// Well, error I guess
		return nil, nil, err
	}

	// Grab the SHA1 key
	docKey := shaHandler.Sum(nil)

	ciphertext, err := EncryptBytes(docKey, marshalledData)
	if err != nil {
		return nil, nil, err
	}

	return ciphertext, docKey, nil
}

func (s *Server) DecryptDocument(documentData []byte, key []byte) (decryptedDocument serverpb.Document, err error) {

	plainText, err := DecryptBytes(key, documentData)
	if err != nil {
		return serverpb.Document{}, err
	}
	if err := decryptedDocument.Unmarshal(plainText); err != nil {
		return serverpb.Document{}, err
	}

	return decryptedDocument, nil
}
