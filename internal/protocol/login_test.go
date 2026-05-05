package protocol

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	cryptorand "crypto/rand"
	"encoding/base64"
	"testing"

	gtprotocol "github.com/sandertv/gophertunnel/minecraft/protocol"
	gtlogin "github.com/sandertv/gophertunnel/minecraft/protocol/login"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

func TestParseLoginPacketUsesGophertunnelLoginParser(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P384(), cryptorand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey() returned error: %v", err)
	}

	identity := IdentityData{
		Identity:    "7b2d9639-5a8c-4f2f-9d8d-4d9f1e6e1f7a",
		DisplayName: "TestPlayer",
	}
	client := ClientData{
		DeviceOS:          gtprotocol.DeviceWin10,
		GameVersion:       CurrentVersion,
		LanguageCode:      "en_US",
		SelfSignedID:      "01f4ce7b-26a1-4a8b-8bbf-c067b49d0d4e",
		ServerAddress:     "127.0.0.1:19132",
		SkinData:          "",
		CapeData:          "",
		SkinResourcePatch: base64.StdEncoding.EncodeToString([]byte(`{"geometry":{"default":"geometry.humanoid.custom"}}`)),
		SkinID:            "test-skin",
		UIProfile:         1,
	}

	pk := &packet.Login{
		ClientProtocol:    CurrentProtocol,
		ConnectionRequest: gtlogin.EncodeOffline(identity, client, key, true),
	}
	data, err := ParseLoginPacket(pk)
	if err != nil {
		t.Fatalf("ParseLoginPacket() returned error: %v", err)
	}
	if data.Identity.Identity != identity.Identity || data.Identity.DisplayName != identity.DisplayName {
		t.Fatalf("Identity = %+v, want %+v", data.Identity, identity)
	}
	if data.Client.DeviceOS != client.DeviceOS || data.Client.SkinID != client.SkinID || data.Client.SelfSignedID != client.SelfSignedID {
		t.Fatalf("Client = %+v, want key fields from %+v", data.Client, client)
	}
	if data.Auth.PublicKey == nil {
		t.Fatalf("Auth.PublicKey = nil, want parsed public key")
	}
	if data.Auth.XBOXLiveAuthenticated {
		t.Fatalf("Auth.XBOXLiveAuthenticated = true, want false for offline login")
	}
}

func TestParseLoginPacketRejectsInvalidRequest(t *testing.T) {
	_, err := ParseLoginPacket(&packet.Login{ConnectionRequest: []byte{0x01, 0x02}})
	if err == nil {
		t.Fatalf("ParseLoginPacket() error = nil, want invalid request error")
	}
}

func TestParseLoginPacketRejectsNilPacket(t *testing.T) {
	_, err := ParseLoginPacket(nil)
	if err == nil {
		t.Fatalf("ParseLoginPacket(nil) error = nil, want error")
	}
}
