package server

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

	handshake, serverEncryptionKey, err := newServerHandshake(&clientKey.PublicKey, serverKey, salt)
	if err != nil {
		t.Fatalf("newServerHandshake() returned error: %v", err)
	}
	clientEncryptionKey, err := clientEncryptionKey(handshake.JWT, clientKey)
	if err != nil {
		t.Fatalf("clientEncryptionKey() returned error: %v", err)
	}
	if clientEncryptionKey != serverEncryptionKey {
		t.Fatalf("client key = %x, want server key %x", clientEncryptionKey, serverEncryptionKey)
	}
}

func TestSharedEncryptionKeyRejectsBadInputs(t *testing.T) {
	serverKey := testP384Key(t)
	clientKey := testP384Key(t)
	if _, err := sharedEncryptionKey(nil, &clientKey.PublicKey, []byte("1234567890abcdef")); err == nil {
		t.Fatalf("sharedEncryptionKey(nil private) error = nil, want error")
	}
	if _, err := sharedEncryptionKey(serverKey, nil, []byte("1234567890abcdef")); err == nil {
		t.Fatalf("sharedEncryptionKey(nil public) error = nil, want error")
	}
	if _, err := sharedEncryptionKey(serverKey, &clientKey.PublicKey, []byte("short")); err == nil {
		t.Fatalf("sharedEncryptionKey(short salt) error = nil, want error")
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
