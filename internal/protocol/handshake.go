package protocol

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	gtlogin "github.com/sandertv/gophertunnel/minecraft/protocol/login"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

const HandshakeSaltBytes = 16

type handshakeSaltClaims struct {
	Salt string `json:"salt"`
}

func NewServerHandshake(clientPublicKey *ecdsa.PublicKey, serverPrivateKey *ecdsa.PrivateKey, salt []byte) (*packet.ServerToClientHandshake, [32]byte, error) {
	keyBytes, err := SharedEncryptionKey(serverPrivateKey, clientPublicKey, salt)
	if err != nil {
		return nil, [32]byte{}, err
	}
	signer, err := jose.NewSigner(jose.SigningKey{Key: serverPrivateKey, Algorithm: jose.ES384}, &jose.SignerOptions{
		ExtraHeaders: map[jose.HeaderKey]any{"x5u": gtlogin.MarshalPublicKey(&serverPrivateKey.PublicKey)},
	})
	if err != nil {
		return nil, [32]byte{}, fmt.Errorf("create server handshake signer: %w", err)
	}
	serverJWT, err := jwt.Signed(signer).Claims(handshakeSaltClaims{
		Salt: base64.RawStdEncoding.EncodeToString(salt),
	}).Serialize()
	if err != nil {
		return nil, [32]byte{}, fmt.Errorf("compact serialise server JWT: %w", err)
	}
	return &packet.ServerToClientHandshake{JWT: []byte(serverJWT)}, keyBytes, nil
}

func ClientEncryptionKey(serverJWT []byte, clientPrivateKey *ecdsa.PrivateKey) ([32]byte, error) {
	if clientPrivateKey == nil {
		return [32]byte{}, fmt.Errorf("client private key cannot be nil")
	}
	token, err := jwt.ParseSigned(string(serverJWT), []jose.SignatureAlgorithm{jose.ES384})
	if err != nil {
		return [32]byte{}, fmt.Errorf("parse server token: %w", err)
	}
	raw, _ := token.Headers[0].ExtraHeaders["x5u"]
	keyString, _ := raw.(string)

	serverPublicKey := new(ecdsa.PublicKey)
	if err := gtlogin.ParsePublicKey(keyString, serverPublicKey); err != nil {
		return [32]byte{}, fmt.Errorf("parse server public key: %w", err)
	}

	var claims handshakeSaltClaims
	if err := token.Claims(serverPublicKey, &claims); err != nil {
		return [32]byte{}, fmt.Errorf("verify server handshake claims: %w", err)
	}
	salt, err := base64.RawStdEncoding.DecodeString(strings.TrimRight(claims.Salt, "="))
	if err != nil {
		return [32]byte{}, fmt.Errorf("decode server handshake salt: %w", err)
	}
	return SharedEncryptionKey(clientPrivateKey, serverPublicKey, salt)
}

func SharedEncryptionKey(privateKey *ecdsa.PrivateKey, publicKey *ecdsa.PublicKey, salt []byte) ([32]byte, error) {
	if privateKey == nil {
		return [32]byte{}, fmt.Errorf("private key cannot be nil")
	}
	if privateKey.Curve == nil || privateKey.D == nil {
		return [32]byte{}, fmt.Errorf("private key is incomplete")
	}
	if publicKey == nil {
		return [32]byte{}, fmt.Errorf("public key cannot be nil")
	}
	if len(salt) != HandshakeSaltBytes {
		return [32]byte{}, fmt.Errorf("handshake salt must be %d bytes, got %d", HandshakeSaltBytes, len(salt))
	}
	if publicKey.Curve == nil || publicKey.X == nil || publicKey.Y == nil {
		return [32]byte{}, fmt.Errorf("public key is incomplete")
	}
	if privateKey.Curve.Params().Name != publicKey.Curve.Params().Name {
		return [32]byte{}, fmt.Errorf("key curve mismatch: private %s, public %s", privateKey.Curve.Params().Name, publicKey.Curve.Params().Name)
	}

	x, _ := publicKey.Curve.ScalarMult(publicKey.X, publicKey.Y, privateKey.D.Bytes())
	if x == nil {
		return [32]byte{}, fmt.Errorf("derive shared secret: scalar multiplication failed")
	}
	secret := x.Bytes()
	if len(secret) > 48 {
		return [32]byte{}, fmt.Errorf("shared secret length %d exceeds P-384 size", len(secret))
	}
	sharedSecret := append(bytes.Repeat([]byte{0}, 48-len(secret)), secret...)
	return sha256.Sum256(append(append([]byte(nil), salt...), sharedSecret...)), nil
}
