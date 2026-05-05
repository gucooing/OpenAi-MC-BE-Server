package protocol

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	cryptorand "crypto/rand"
	"testing"
)

func TestServerHandshakeProducesClientEncryptionKey(t *testing.T) {
	serverKey := testP384Key(t)
	clientKey := testP384Key(t)
	salt := []byte("1234567890abcdef")

	handshake, serverEncryptionKey, err := NewServerHandshake(&clientKey.PublicKey, serverKey, salt)
	if err != nil {
		t.Fatalf("NewServerHandshake() returned error: %v", err)
	}
	clientEncryptionKey, err := ClientEncryptionKey(handshake.JWT, clientKey)
	if err != nil {
		t.Fatalf("ClientEncryptionKey() returned error: %v", err)
	}
	if clientEncryptionKey != serverEncryptionKey {
		t.Fatalf("client key = %x, want server key %x", clientEncryptionKey, serverEncryptionKey)
	}
}

func TestSharedEncryptionKeyRejectsBadInputs(t *testing.T) {
	serverKey := testP384Key(t)
	clientKey := testP384Key(t)
	if _, err := SharedEncryptionKey(nil, &clientKey.PublicKey, []byte("1234567890abcdef")); err == nil {
		t.Fatalf("SharedEncryptionKey(nil private) error = nil, want error")
	}
	if _, err := SharedEncryptionKey(serverKey, nil, []byte("1234567890abcdef")); err == nil {
		t.Fatalf("SharedEncryptionKey(nil public) error = nil, want error")
	}
	if _, err := SharedEncryptionKey(serverKey, &clientKey.PublicKey, []byte("short")); err == nil {
		t.Fatalf("SharedEncryptionKey(short salt) error = nil, want error")
	}
}

func testP384Key(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P384(), cryptorand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey() returned error: %v", err)
	}
	return key
}
