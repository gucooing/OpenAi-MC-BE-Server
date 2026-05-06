package server

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/google/uuid"
	gtprotocol "github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"

	appworld "gucooing/bds/internal/world"
)

func TestMCPEChatAndCommands(t *testing.T) {
	world, err := appworld.NewFlatGenerator()
	if err != nil {
		t.Fatalf("NewFlatGenerator() returned error: %v", err)
	}
	handler, err := NewMCPEHandler(MCPEOptions{
		Logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		ServerName:   "MCPE Commands Test",
		ServerBrand:  "BetterAltay-Go",
		GameMode:     "Survival",
		MaxPlayers:   5,
		ViewDistance: 1,
		World:        world,
	})
	if err != nil {
		t.Fatalf("NewMCPEHandler() returned error: %v", err)
	}

	ctx := context.Background()
	alice, aliceConn, _ := spawnTestClient(t, ctx, handler, "Alice", "7b2d9639-5a8c-4f2f-9d8d-4d9f1e6e1f7a")
	_, bobConn, _ := spawnTestClient(t, ctx, handler, "Bob", "fd4c35b1-98c2-4208-9e0e-7d4c30e6dff1")

	beforeAliceChat := len(aliceConn.packets)
	beforeBobChat := len(bobConn.packets)
	if err := alice.HandlePacket(ctx, &packet.Text{TextType: packet.TextTypeChat, Message: "hello world"}); err != nil {
		t.Fatalf("HandlePacket(Text) returned error: %v", err)
	}
	if pk := packetAt[*packet.Text](t, aliceConn.packets, beforeAliceChat); pk.TextType != packet.TextTypeChat || pk.SourceName != "Alice" || pk.Message != "hello world" {
		t.Fatalf("Alice chat echo = %+v, want authored chat", pk)
	}
	if pk := packetAt[*packet.Text](t, bobConn.packets, beforeBobChat); pk.TextType != packet.TextTypeChat || pk.SourceName != "Alice" || pk.Message != "hello world" {
		t.Fatalf("Bob chat receive = %+v, want Alice chat", pk)
	}

	origin := gtprotocol.CommandOrigin{Origin: gtprotocol.CommandOriginPlayer, UUID: uuid.New(), RequestID: "list-1"}
	beforeList := len(aliceConn.packets)
	if err := alice.HandlePacket(ctx, &packet.CommandRequest{CommandLine: "/list", CommandOrigin: origin}); err != nil {
		t.Fatalf("HandlePacket(CommandRequest /list) returned error: %v", err)
	}
	if pk := packetAt[*packet.CommandOutput](t, aliceConn.packets, beforeList); pk.SuccessCount != 1 || !commandOutputContains(pk, "Alice") || !commandOutputContains(pk, "Bob") {
		t.Fatalf("/list output = %+v, want success with both names", pk)
	}

	beforeDenied := len(aliceConn.packets)
	if err := alice.HandlePacket(ctx, &packet.CommandRequest{CommandLine: "/say denied", CommandOrigin: gtprotocol.CommandOrigin{Origin: gtprotocol.CommandOriginPlayer, UUID: uuid.New(), RequestID: "say-denied"}}); err != nil {
		t.Fatalf("HandlePacket(CommandRequest denied /say) returned error: %v", err)
	}
	if pk := packetAt[*packet.CommandOutput](t, aliceConn.packets, beforeDenied); pk.SuccessCount != 0 || !commandOutputContains(pk, "permission") {
		t.Fatalf("denied /say output = %+v, want permission failure", pk)
	}

	if result := handler.ExecuteConsoleCommand(ctx, "op Alice"); !result.Success {
		t.Fatalf("console op result = %#v, want success", result)
	}
	opCommands := packetAt[*packet.AvailableCommands](t, aliceConn.packets, beforeDenied+1)
	if len(opCommands.Commands) <= 2 {
		t.Fatalf("op AvailableCommands count = %d, want operator commands included", len(opCommands.Commands))
	}

	beforeOpSayAlice := len(aliceConn.packets)
	beforeOpSayBob := len(bobConn.packets)
	if err := alice.HandlePacket(ctx, &packet.CommandRequest{CommandLine: "/say promoted", CommandOrigin: gtprotocol.CommandOrigin{Origin: gtprotocol.CommandOriginPlayer, UUID: uuid.New(), RequestID: "say-ok"}}); err != nil {
		t.Fatalf("HandlePacket(CommandRequest op /say) returned error: %v", err)
	}
	if pk := packetAt[*packet.Text](t, bobConn.packets, beforeOpSayBob); pk.TextType != packet.TextTypeAnnouncement || pk.SourceName != "Alice" || pk.Message != "promoted" {
		t.Fatalf("op /say broadcast = %+v, want announcement from Alice", pk)
	}
	if pk := packetAt[*packet.CommandOutput](t, aliceConn.packets, beforeOpSayAlice+1); pk.SuccessCount != 1 || !commandOutputContains(pk, "Message sent") {
		t.Fatalf("op /say output = %+v, want success", pk)
	}
}

func TestConsoleStopCommandRequestsShutdown(t *testing.T) {
	handler, err := NewMCPEHandler(MCPEOptions{
		ServerName: "Stop Test",
		Shutdown:   func() {},
	})
	if err != nil {
		t.Fatalf("NewMCPEHandler() returned error: %v", err)
	}
	stopped := false
	handler.shutdown = func() { stopped = true }

	result := handler.ExecuteConsoleCommand(context.Background(), "stop")
	if !result.Success || !stopped {
		t.Fatalf("stop result = %#v stopped=%t, want success and shutdown", result, stopped)
	}
}

func commandOutputContains(pk *packet.CommandOutput, needle string) bool {
	needle = strings.ToLower(needle)
	for _, message := range pk.OutputMessages {
		if strings.Contains(strings.ToLower(message.Message), needle) {
			return true
		}
		for _, parameter := range message.Parameters {
			if strings.Contains(strings.ToLower(parameter), needle) {
				return true
			}
		}
	}
	return false
}
