package raknet

import (
	"context"
	"net"
	"testing"
	"time"

	raknetlib "github.com/sandertv/go-raknet"
)

func TestListenRespondsToUnconnectedPing(t *testing.T) {
	server, err := Listen(Options{
		Address: "127.0.0.1:0",
		PongInfo: PongInfo{
			MOTD:             "Ping Test",
			ProtocolVersion:  944,
			MinecraftVersion: "1.26.10",
			MaxPlayers:       10,
			ServerName:       "BetterAltay-Go",
			GameMode:         "Survival",
		},
	})
	if err != nil {
		t.Fatalf("Listen() returned error: %v", err)
	}
	defer server.Close()

	_, port, err := net.SplitHostPort(server.Addr().String())
	if err != nil {
		t.Fatalf("SplitHostPort() returned error: %v", err)
	}

	data, err := raknetlib.PingTimeout(net.JoinHostPort("127.0.0.1", port), time.Second)
	if err != nil {
		t.Fatalf("PingTimeout() returned error: %v", err)
	}

	want := "MCPE;Ping Test;944;1.26.10;0;10;"
	if got := string(data); len(got) < len(want) || got[:len(want)] != want {
		t.Fatalf("ping data = %q, want prefix %q", got, want)
	}
}

func TestListenRunsSessionHandlerUntilServerClose(t *testing.T) {
	started := make(chan struct{})
	released := make(chan struct{})

	server, err := Listen(Options{
		Address: "127.0.0.1:0",
		PongInfo: PongInfo{
			MOTD:             "Session Test",
			ProtocolVersion:  944,
			MinecraftVersion: "1.26.10",
			MaxPlayers:       10,
			ServerName:       "BetterAltay-Go",
			GameMode:         "Survival",
		},
		SessionHandler: func(ctx context.Context, conn net.Conn) error {
			close(started)
			<-ctx.Done()
			close(released)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Listen() returned error: %v", err)
	}

	conn, err := raknetlib.DialTimeout(server.Addr().String(), 2*time.Second)
	if err != nil {
		t.Fatalf("DialTimeout() returned error: %v", err)
	}
	defer conn.Close()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatalf("session handler was not started")
	}

	if err := server.Close(); err != nil {
		t.Fatalf("Close() returned error: %v", err)
	}

	select {
	case <-released:
	default:
		t.Fatalf("session handler was not released before Close returned")
	}
}
