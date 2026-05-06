package mcpe

import (
	"context"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	raknetlib "github.com/sandertv/go-raknet"
	gtprotocol "github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

func TestListenUsesOwnRakNetTransportForPing(t *testing.T) {
	server, err := Listen(Options{
		Address:     "127.0.0.1:0",
		ServerName:  "MCPE Ping Test",
		ServerBrand: "BetterAltay-Go",
		GameMode:    "Survival",
		MaxPlayers:  5,
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		NewClient: func(PacketConn) PacketClient {
			return noopClient{}
		},
	})
	if err != nil {
		t.Fatalf("Listen() returned error: %v", err)
	}
	defer server.Close()

	data, err := raknetlib.PingTimeout(server.Addr().String(), time.Second)
	if err != nil {
		t.Fatalf("PingTimeout() returned error: %v", err)
	}
	want := "MCPE;MCPE Ping Test;975;1.26.20;0;5;"
	if got := string(data); len(got) < len(want) || got[:len(want)] != want {
		t.Fatalf("ping data = %q, want prefix %q", got, want)
	}
}

func TestSessionDecodesBatchAndForwardsPackets(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := &recordingClient{packets: make(chan packet.Packet, 1)}
	done := make(chan error, 1)
	go func() {
		done <- newSession(func(PacketConn) PacketClient {
			return client
		}, slog.New(slog.NewTextHandler(io.Discard, nil)), serverConn).Serve(ctx)
	}()

	clientCodec := newCodec()
	plainCodec := clientCodec
	plainCodec.Compression = nil
	want := &packet.RequestNetworkSettings{ClientProtocol: int32(gtprotocol.CurrentProtocol)}
	writeClientPacket(t, clientConn, plainCodec, want)
	select {
	case got := <-client.packets:
		if pk, ok := got.(*packet.RequestNetworkSettings); !ok || pk.ClientProtocol != want.ClientProtocol {
			t.Fatalf("forwarded packet = %T %+v, want RequestNetworkSettings", got, got)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("session did not forward decoded packet")
	}

	cancel()
	_ = clientConn.Close()
	if err := <-done; err != nil {
		t.Fatalf("Serve() returned error: %v", err)
	}
}

type recordingClient struct {
	packets chan packet.Packet
}

func (client *recordingClient) HandlePacket(_ context.Context, pk packet.Packet) error {
	client.packets <- pk
	return nil
}

func (client *recordingClient) State() int {
	return 0
}

type noopClient struct{}

func (noopClient) HandlePacket(context.Context, packet.Packet) error {
	return nil
}

func (noopClient) State() int {
	return 0
}

func writeClientPacket(t *testing.T, conn net.Conn, codec codec, pk packet.Packet) {
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
