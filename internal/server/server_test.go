package server

import (
	"context"
	"io"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	raknetlib "github.com/sandertv/go-raknet"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"

	"gucooing/bds/internal/config"
)

func TestServerLifecycleRunsTicksAndMainThreadTasks(t *testing.T) {
	server := newTestServer(t)
	if err := server.Start(); err != nil {
		t.Fatalf("Start() returned error: %v", err)
	}
	defer stopServer(t, server)

	done := make(chan struct{})
	if err := server.Submit(func(context.Context) {
		close(done)
	}); err != nil {
		t.Fatalf("Submit() returned error: %v", err)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("submitted task was not executed")
	}

	waitFor(t, func() bool {
		return server.Stats().Tick > 0
	}, "server tick did not advance")
	stats := server.Stats()
	if stats.TPS <= 0 || stats.MaxPlayers != 5 || stats.Goroutines == 0 {
		t.Fatalf("Stats() = %+v, want populated lifecycle metrics", stats)
	}
}

func TestServerStopCommandStopsRuntime(t *testing.T) {
	server := newTestServer(t)
	if err := server.Start(); err != nil {
		t.Fatalf("Start() returned error: %v", err)
	}

	result := server.ExecuteConsoleCommand(context.Background(), "stop")
	if !result.Success {
		t.Fatalf("stop command failed: %+v", result)
	}
	select {
	case <-server.Done():
	case <-time.After(2 * time.Second):
		t.Fatalf("server did not stop after stop command")
	}
}

func TestMCPEPlayerDisconnectRemovesPlayerAndBroadcastsLeave(t *testing.T) {
	store := &recordingPlayerStore{saved: make(chan PlayerSnapshot, 1)}
	server := newTestServer(t, func(options *Options) {
		options.PlayerStore = store
	})
	if err := server.Start(); err != nil {
		t.Fatalf("Start() returned error: %v", err)
	}
	defer stopServer(t, server)

	ctx := context.Background()
	alice, aliceConn, _ := spawnTestClient(t, ctx, server.Handler(), "Alice", "7b2d9639-5a8c-4f2f-9d8d-4d9f1e6e1f7a")
	bob, _, _ := spawnTestClient(t, ctx, server.Handler(), "Bob", "fd4c35b1-98c2-4208-9e0e-7d4c30e6dff1")
	if got := server.OnlinePlayers(); got != 2 {
		t.Fatalf("OnlinePlayers() = %d, want 2", got)
	}

	before := len(aliceConn.packets)
	bob.OnDisconnect(ctx)
	if got := server.OnlinePlayers(); got != 1 {
		t.Fatalf("OnlinePlayers() after disconnect = %d, want 1", got)
	}
	packetAt[*packet.PlayerList](t, aliceConn.packets, before)
	packetAt[*packet.RemoveActor](t, aliceConn.packets, before+1)
	leave := packetAt[*packet.Text](t, aliceConn.packets, before+2)
	if leave.TextType != packet.TextTypeTranslation || leave.Message != "multiplayer.player.left" {
		t.Fatalf("leave text = %+v, want translated leave message", leave)
	}
	if err := bob.HandlePacket(ctx, &packet.MovePlayer{EntityRuntimeID: bob.runtimeID}); err != nil {
		t.Fatalf("disconnected client HandlePacket() returned error: %v", err)
	}
	if alice.state != stateSpawned {
		t.Fatalf("remaining player state = %d, want spawned", alice.state)
	}
	select {
	case snapshot := <-store.saved:
		if snapshot.Name != "Bob" || snapshot.UUID != "fd4c35b1-98c2-4208-9e0e-7d4c30e6dff1" || snapshot.RuntimeID != bob.runtimeID {
			t.Fatalf("saved player snapshot = %+v, want Bob data", snapshot)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("player snapshot was not saved on disconnect")
	}
}

func TestServerMOTDReflectsOnlinePlayers(t *testing.T) {
	server := newTestServer(t)
	if err := server.Start(); err != nil {
		t.Fatalf("Start() returned error: %v", err)
	}
	defer stopServer(t, server)

	_, _, _ = spawnTestClient(t, context.Background(), server.Handler(), "Alice", "7b2d9639-5a8c-4f2f-9d8d-4d9f1e6e1f7a")
	info := server.Info()
	if info.ServerName != "Lifecycle Test" || info.ProtocolVersion != 975 || info.MinecraftVersion != "1.26.20" || info.OnlinePlayers != 1 || info.MaxPlayers != 5 {
		t.Fatalf("Info() = %+v, want current server query data", info)
	}
	waitFor(t, func() bool {
		data, err := raknetlib.PingTimeout(server.Addr().String(), time.Second)
		return err == nil && strings.Contains(string(data), ";1;5;")
	}, "MOTD did not include one online player")
}

func newTestServer(t *testing.T, edits ...func(*Options)) *Server {
	t.Helper()
	cfg := config.DefaultConfig
	cfg.Address = "127.0.0.1"
	cfg.Port = freeUDPPort(t)
	cfg.ServerName = "Lifecycle Test"
	cfg.MaxPlayers = 5
	cfg.ViewDistance = 2
	cfg.LogFile = ""
	options := Options{
		Config: cfg,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Brand:  "BetterAltay-Go",
	}
	for _, edit := range edits {
		edit(&options)
	}
	server, err := New(options)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	return server
}

type recordingPlayerStore struct {
	saved chan PlayerSnapshot
}

func (store *recordingPlayerStore) SavePlayer(_ context.Context, snapshot PlayerSnapshot) error {
	store.saved <- snapshot
	return nil
}

func stopServer(t *testing.T, server *Server) {
	t.Helper()
	server.Stop()
	select {
	case <-server.Done():
	case <-time.After(2 * time.Second):
		t.Fatalf("server did not stop")
	}
}

func waitFor(t *testing.T, condition func() bool, message string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal(message)
}

func freeUDPPort(t *testing.T) int {
	t.Helper()

	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("ListenPacket() returned error: %v", err)
	}
	defer conn.Close()

	_, portString, err := net.SplitHostPort(conn.LocalAddr().String())
	if err != nil {
		t.Fatalf("SplitHostPort() returned error: %v", err)
	}
	port, err := strconv.Atoi(portString)
	if err != nil {
		t.Fatalf("Atoi() returned error: %v", err)
	}
	return port
}
