package server

import (
	"context"
	"crypto/aes"
	"crypto/sha1"
	"encoding/asn1"
	"encoding/base64"
	"fmt"
	"proj2_f5w9a_h6v9a_q7w9a_r8u8_w1c0b/serverpb"
	"time"

	"github.com/dgraph-io/badger"
)

func (s *Server) Get(ctx context.Context, in *serverpb.GetRequest) (*serverpb.GetResponse, error) {
	var f serverpb.Document
	// TODO: Change this to take an _accessID_ and string split to separate the key and the documentID

	// Local check, sees if the file is on the local node
	if err := s.db.View(func(txn *badger.Txn) error {
		key := fmt.Sprintf("/document/%s", in.AccessId)
		item, err := txn.Get([]byte(key))
		if err != nil {
			return err
		}
		body, err := item.Value()
		if err != nil {
			return err
		}

		// TODO: Decrypt the data so it's possible to unmarshal into a document
		if err := f.Unmarshal(body); err != nil {
			return err
		}
		return nil
	}); err != nil {
		// TODO: Check error, if file not found in the local one, network check
		// TODO: Network check, does this document exist on the network? If so, return it
		// Currently just returns no matter what the error was
		return nil, err
	}


	// f should be a document at this point
	resp := &serverpb.GetResponse{
		Document: &f,
	}


	return resp, nil
}

func (s *Server) Add(ctx context.Context, in *serverpb.AddRequest) (*serverpb.AddResponse, error) {

	// TODO: Take in a document (in.Document)
	// TODO: Encrypt the document
	//

	b, err := in.Document.Marshal()
	if err != nil {
		return nil, err
	}

	data := sha1.Sum(b)
	hash := base64.StdEncoding.EncodeToString(data[:])


	// TODO: Change this to add the encrypted document andd the document ID (hash of the encrypted document)
	if err := s.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(fmt.Sprintf("/document/%s", hash)), b)
	}); err != nil {
		return nil, err
	}

	// TODO: Return the access ID (Rename DocumentID -> AccessID?)
	resp := &serverpb.AddResponse{
		AccessId: hash,
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

// TODO: MAKE SOME End to End behaviour testing
func (s *Server) EncryptDocument(doc serverpb.Document) (encryptedData []byte, key []byte, err error) {
	// Create a new SHA1 handler
	shaHandler := sha1.New()

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

	// Create a new AESBlockCipher
	aesBlock, err := aes.NewCipher(docKey)
	if err != nil {
		return nil, nil, err
	}

	// Empty Byte Array
	encryptedData = []byte{}

	// Encrypt the block
	aesBlock.Encrypt(encryptedData, marshalledData)

	return encryptedData, docKey, nil
}

func (s *Server) DecryptDocument(documentData []byte, key []byte) (decryptedDocument serverpb.Document, err error){
	aesBlock, err := aes.NewCipher(key)
	if err != nil {
		return serverpb.Document{}, err
	}

	decryptedData := []byte{}
	aesBlock.Decrypt(decryptedData, documentData)

	err = decryptedDocument.Unmarshal(decryptedData)
	if err != nil {
		return serverpb.Document{}, err
	}

	return decryptedDocument, nil
}

// Given the (encrypted) document data and the key
func (s *Server) GetAccessID(documentData []byte, key []byte) (accessID string, err error){
	shaHandler := sha1.New()

	// Attempt to write the data to the SHA1 handler (is this right?)
	if _, err := shaHandler.Write(documentData); err != nil {
		// Well, error I guess
		return "", err
	}

	id := shaHandler.Sum(nil)

	accessID = string(id) + ":" + string(key)

	return accessID, nil
}
