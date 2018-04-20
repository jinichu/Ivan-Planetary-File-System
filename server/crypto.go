package server

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"math/big"
	"os"
	"time"

	"github.com/dgraph-io/badger"
	"github.com/pkg/errors"
)

const (
	keyKey        = "/config/key"
	privateKeyKey = "/config/privateKey"
	certKey       = "/config/cert"

	validFor = 10 * 365 * 24 * time.Hour
)

type EcdsaSignature struct {
	R, S *big.Int
}

func publicKey(priv interface{}) interface{} {
	switch k := priv.(type) {
	case *rsa.PrivateKey:
		return &k.PublicKey
	case *ecdsa.PrivateKey:
		return &k.PublicKey
	default:
		return nil
	}
}

func PemBlockForKey(priv interface{}) *pem.Block {
	switch k := priv.(type) {
	case *rsa.PrivateKey:
		return &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(k)}
	case *ecdsa.PrivateKey:
		b, err := x509.MarshalECPrivateKey(k)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Unable to marshal ECDSA private key: %v", err)
			os.Exit(2)
		}
		return &pem.Block{Type: "EC PRIVATE KEY", Bytes: b}
	default:
		return nil
	}
}

func (s *Server) loadCert() error {
	if err := s.db.View(func(txn *badger.Txn) error {
		keyItem, err := txn.Get([]byte(keyKey))
		if err != nil {
			return err
		}
		keyValue, err := keyItem.Value()
		if err != nil {
			return err
		}
		certItem, err := txn.Get([]byte(certKey))
		if err != nil {
			return err
		}
		certValue, err := certItem.Value()
		if err != nil {
			return err
		}

		s.certPublic = string(certValue)

		cert, err := tls.X509KeyPair(certValue, keyValue)
		if err != nil {
			return err
		}
		s.cert = &cert

		privateKeyItem, err := txn.Get([]byte(privateKeyKey))
		if err != nil {
			return err
		}
		privKey, err := privateKeyItem.Value()
		if err != nil {
			return err
		}
		s.key, err = x509.ParseECPrivateKey(privKey)
		if err != nil {
			return err
		}

		return nil
	}); err != nil {
		return err
	}

	return ErrUnimplemented
}

func (s *Server) generateCert() error {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		log.Fatalf("failed to generate private key: %s", err)
	}
	s.key = priv
	privKey, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return err
	}

	notBefore := time.Now()
	notAfter := notBefore.Add(validFor)

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		log.Fatalf("failed to generate serial number: %s", err)
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"UBC 416"},
		},
		NotBefore: notBefore,
		NotAfter:  notAfter,

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	template.IPAddresses = append(template.IPAddresses, getOutboundIP())

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, publicKey(priv), priv)
	if err != nil {
		return err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	keyPEM := pem.EncodeToMemory(PemBlockForKey(priv))
	s.certPublic = string(certPEM)

	if err := s.db.Update(func(txn *badger.Txn) error {
		if err := txn.Set([]byte(privateKeyKey), privKey); err != nil {
			return err
		}
		if err := txn.Set([]byte(certKey), certPEM); err != nil {
			return err
		}
		if err := txn.Set([]byte(keyKey), keyPEM); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return err
	}
	s.cert = &cert

	return nil
}

func (s *Server) loadOrGenerateCert() error {
	if err := s.loadCert(); err == badger.ErrKeyNotFound {
		if err := s.generateCert(); err != nil {
			return err
		}
	}
	return nil
}

func LoadPrivate(privateBody []byte) (*ecdsa.PrivateKey, error) {
	privateKey, err := UnmarshalPrivate(string(privateBody))
	if err != nil {
		return nil, err
	}

	return privateKey, nil
}

// UnmarshalPrivate unmarshals a x509/PEM encoded ECDSA private key.
func UnmarshalPrivate(key string) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(key))
	if block == nil {
		return nil, errors.New("no PEM block found in private key")
	}
	privKey, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	return privKey, nil
}

// UnmarshalPublic unmarshals a x509/PEM encoded ECDSA public key.
func UnmarshalPublic(key string) (*ecdsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(key))
	if block == nil {
		return nil, errors.New("no PEM block found in public key")
	}
	pubKey, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	ecdsaPubKey, ok := pubKey.(*ecdsa.PublicKey)
	if !ok {
		return nil, errors.New("invalid public key")
	}
	return ecdsaPubKey, nil
}

// MarshalPublic marshals a x509/PEM encoded ECDSA public key.
func MarshalPublic(key *ecdsa.PublicKey) (string, error) {
	if key == nil || key.Curve == nil || key.X == nil || key.Y == nil {
		return "", fmt.Errorf("key or part of key is nil: %+v", key)
	}

	key.Curve = fixCurve(key.Curve)

	rawPriv, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		return "", err
	}

	keyBlock := &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: rawPriv,
	}

	return string(pem.EncodeToMemory(keyBlock)), nil
}

var curves = []elliptic.Curve{elliptic.P224(), elliptic.P256(), elliptic.P384(), elliptic.P521()}

func fixCurve(curve elliptic.Curve) elliptic.Curve {
	if curve == nil {
		return curve
	}

	for _, c := range curves {
		if c.Params().Name == curve.Params().Name {
			return c
		}
	}
	return curve
}

// Provides a sig for an operation
func Sign(operation []byte, privKey ecdsa.PrivateKey) (signedR, signedS *big.Int, err error) {
	r, s, err := ecdsa.Sign(rand.Reader, &privKey, operation)
	if err != nil {
		return big.NewInt(0), big.NewInt(0), err
	}

	signedR = r
	signedS = s
	return
}

// Compute the Hash of any string
func Hash(a interface{}) (string, error) {
	h := sha1.New()
	if err := json.NewEncoder(h).Encode(a); err != nil {
		return "", nil
	}
	return base64.URLEncoding.EncodeToString(h.Sum(nil)), nil
}

func HashBytes(a []byte) string {
	hash := sha1.Sum(a)
	return base64.URLEncoding.EncodeToString(hash[:])
}

func EncryptBytes(key, body []byte) ([]byte, error) {
	// Create a new AESBlockCipher
	aesBlock, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	ciphertext := make([]byte, aes.BlockSize+len(body))
	iv := ciphertext[:aes.BlockSize]
	if _, err := io.ReadFull(DevZero(0), iv); err != nil {
		return nil, err
	}

	stream := cipher.NewCFBEncrypter(aesBlock, iv)
	stream.XORKeyStream(ciphertext[aes.BlockSize:], body)

	return ciphertext, nil
}

func GenerateAESKey() ([]byte, error) {
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, err
	}
	return key, nil
}

type DevZero int

func (z DevZero) Read(b []byte) (n int, err error) {
	for i := range b {
		b[i] = 0
	}

	return len(b), nil
}

func GenerateAESKeyFromECDSA(key *ecdsa.PrivateKey) ([]byte, error) {
	body := []byte(`yes this is some body yes wow very body`)
	r, s, err := ecdsa.Sign(DevZero(0), key, body)
	if err != nil {
		return nil, err
	}
	sig, err := asn1.Marshal(EcdsaSignature{R: r, S: s})
	if err != nil {
		return nil, err
	}
	h := sha256.New()
	if _, err := h.Write(body); err != nil {
		return nil, err
	}
	if _, err := h.Write(sig); err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}

func DecryptBytes(key, body []byte) ([]byte, error) {
	aesBlock, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	if len(body) < aes.BlockSize {
		return nil, errors.Errorf("ciphertext too short")
	}

	iv := body[:aes.BlockSize]
	body = body[aes.BlockSize:]

	stream := cipher.NewCFBDecrypter(aesBlock, iv)
	plainText := make([]byte, len(body))
	stream.XORKeyStream(plainText, body)

	return plainText, nil
}
