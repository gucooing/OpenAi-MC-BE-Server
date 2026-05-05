package protocol

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	cryptorand "crypto/rand"
	"encoding/base64"
	"log/slog"
	"net"
	"strings"
	"testing"
	"time"

	gtprotocol "github.com/sandertv/gophertunnel/minecraft/protocol"
	gtlogin "github.com/sandertv/gophertunnel/minecraft/protocol/login"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

func TestDebugSessionLogsParsedNetworkTraffic(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- ServeDebugSession(ctx, serverConn, logger)
	}()

	codec := NewCodec()
	plainCodec := codec
	plainCodec.Compression = nil

	writePacket(t, clientConn, plainCodec, &packet.RequestNetworkSettings{ClientProtocol: int32(CurrentProtocol)})
	response := readPacket(t, clientConn, plainCodec, FromServer)
	if _, ok := response.(*packet.NetworkSettings); !ok {
		t.Fatalf("response = %T, want *packet.NetworkSettings", response)
	}

	key, err := ecdsa.GenerateKey(elliptic.P384(), cryptorand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey() returned error: %v", err)
	}
	loginPacket := &packet.Login{
		ClientProtocol: int32(CurrentProtocol),
		ConnectionRequest: gtlogin.EncodeOffline(IdentityData{
			Identity:    "7b2d9639-5a8c-4f2f-9d8d-4d9f1e6e1f7a",
			DisplayName: "TestPlayer",
		}, ClientData{
			DeviceOS:          gtprotocol.DeviceWin10,
			GameVersion:       CurrentVersion,
			LanguageCode:      "en_US",
			SelfSignedID:      "01f4ce7b-26a1-4a8b-8bbf-c067b49d0d4e",
			ServerAddress:     "127.0.0.1:19132",
			SkinResourcePatch: base64.StdEncoding.EncodeToString([]byte(`{"geometry":{"default":"geometry.humanoid.custom"}}`)),
			SkinID:            "test-skin",
			UIProfile:         1,
		}, key, true),
	}
	writePacket(t, clientConn, codec, loginPacket)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(logs.String(), "mcpe login parsed") && strings.Contains(logs.String(), "TestPlayer") {
			cancel()
			_ = clientConn.Close()
			<-done
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("debug logs did not contain parsed login record:\n%s", logs.String())
}

func writePacket(t *testing.T, conn net.Conn, codec Codec, pk Packet) {
	t.Helper()
	payload, err := codec.EncodePacket(pk)
	if err != nil {
		t.Fatalf("EncodePacket(%T) returned error: %v", pk, err)
	}
	batch, err := codec.EncodeBatch([][]byte{payload})
	if err != nil {
		t.Fatalf("EncodeBatch() returned error: %v", err)
	}
	if _, err := conn.Write(batch); err != nil {
		t.Fatalf("Write() returned error: %v", err)
	}
}

func readPacket(t *testing.T, conn net.Conn, codec Codec, direction Direction) Packet {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("SetReadDeadline() returned error: %v", err)
	}
	buffer := make([]byte, 2048)
	n, err := conn.Read(buffer)
	if err != nil {
		t.Fatalf("Read() returned error: %v", err)
	}
	packets, err := codec.DecodeBatch(buffer[:n])
	if err != nil {
		t.Fatalf("DecodeBatch() returned error: %v", err)
	}
	if len(packets) != 1 {
		t.Fatalf("DecodeBatch() returned %d packets, want 1", len(packets))
	}
	pk, err := codec.DecodePacket(packets[0], direction)
	if err != nil {
		t.Fatalf("DecodePacket() returned error: %v", err)
	}
	return pk
}
