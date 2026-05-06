package server

import (
	"context"
	"crypto/ecdsa"
	"encoding/base64"
	"io"
	"log/slog"
	"testing"

	"github.com/go-gl/mathgl/mgl32"
	gtprotocol "github.com/sandertv/gophertunnel/minecraft/protocol"
	gtlogin "github.com/sandertv/gophertunnel/minecraft/protocol/login"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"

	appworld "gucooing/bds/internal/world"
)

func TestMCPEPlayerSyncSpawnsPeersAndBroadcastsMovement(t *testing.T) {
	world, err := appworld.NewFlatGenerator()
	if err != nil {
		t.Fatalf("NewFlatGenerator() returned error: %v", err)
	}
	handler, err := NewMCPEHandler(MCPEOptions{
		Logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		ServerName:   "MCPE Player Sync Test",
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
	alicePacketCount := len(aliceConn.packets)
	bob, bobConn, _ := spawnTestClient(t, ctx, handler, "Bob", "fd4c35b1-98c2-4208-9e0e-7d4c30e6dff1")

	if pk := packetAt[*packet.PlayerList](t, aliceConn.packets, alicePacketCount); pk.ActionType != packet.PlayerListActionAdd || len(pk.Entries) != 1 || pk.Entries[0].Username != "Bob" {
		t.Fatalf("Alice PlayerList update = %+v, want Bob add entry", pk)
	}
	if pk := packetAt[*packet.AddPlayer](t, aliceConn.packets, alicePacketCount+1); pk.Username != "Bob" || pk.EntityRuntimeID != bob.runtimeID {
		t.Fatalf("Alice AddPlayer = %+v, want Bob runtime ID %d", pk, bob.runtimeID)
	}
	if pk := packetAt[*packet.SetActorData](t, aliceConn.packets, alicePacketCount+2); pk.EntityRuntimeID != bob.runtimeID {
		t.Fatalf("Alice SetActorData runtime ID = %d, want Bob %d", pk.EntityRuntimeID, bob.runtimeID)
	}
	if pk := packetAt[*packet.SetActorMotion](t, aliceConn.packets, alicePacketCount+3); pk.EntityRuntimeID != bob.runtimeID {
		t.Fatalf("Alice SetActorMotion runtime ID = %d, want Bob %d", pk.EntityRuntimeID, bob.runtimeID)
	}

	bobPeerSpawnIndex := len(bobConn.packets) - 3
	if pk := packetAt[*packet.AddPlayer](t, bobConn.packets, bobPeerSpawnIndex); pk.Username != "Alice" || pk.EntityRuntimeID != alice.runtimeID {
		t.Fatalf("Bob AddPlayer = %+v, want Alice runtime ID %d", pk, alice.runtimeID)
	}
	if pk := packetAt[*packet.SetActorData](t, bobConn.packets, bobPeerSpawnIndex+1); pk.EntityRuntimeID != alice.runtimeID {
		t.Fatalf("Bob SetActorData runtime ID = %d, want Alice %d", pk.EntityRuntimeID, alice.runtimeID)
	}
	if pk := packetAt[*packet.SetActorMotion](t, bobConn.packets, bobPeerSpawnIndex+2); pk.EntityRuntimeID != alice.runtimeID {
		t.Fatalf("Bob SetActorMotion runtime ID = %d, want Alice %d", pk.EntityRuntimeID, alice.runtimeID)
	}

	beforeMovement := len(aliceConn.packets)
	input := gtprotocol.NewBitset(packet.PlayerAuthInputBitsetSize)
	input.Set(packet.InputFlagStartSprinting)
	if err := bob.HandlePacket(ctx, &packet.PlayerAuthInput{
		Position:  mgl32.Vec3{2, 64, 3},
		Pitch:     370,
		Yaw:       -30,
		HeadYaw:   -30,
		InputData: input,
		Delta:     mgl32.Vec3{0.2, 0, 0.4},
		Tick:      42,
	}); err != nil {
		t.Fatalf("HandlePacket(PlayerAuthInput) returned error: %v", err)
	}
	if pk := packetAt[*packet.SetActorData](t, aliceConn.packets, beforeMovement); !metadataFlag(pk.EntityMetadata, gtprotocol.EntityDataFlagSprinting) {
		t.Fatalf("SetActorData metadata = %+v, want sprinting flag", pk.EntityMetadata)
	}
	if pk := packetAt[*packet.MovePlayer](t, aliceConn.packets, beforeMovement+1); pk.EntityRuntimeID != bob.runtimeID || pk.Position != (mgl32.Vec3{2, 64, 3}) || pk.Yaw != 330 || pk.Pitch != 10 {
		t.Fatalf("MovePlayer from PlayerAuthInput = %+v, want Bob moved to 2,64,3 with normalised rotation", pk)
	}
	if pk := packetAt[*packet.SetActorMotion](t, aliceConn.packets, beforeMovement+2); pk.EntityRuntimeID != bob.runtimeID || pk.Velocity != (mgl32.Vec3{0.2, 0, 0.4}) || pk.Tick != 42 {
		t.Fatalf("SetActorMotion from PlayerAuthInput = %+v, want Bob delta motion", pk)
	}

	beforeLegacyMove := len(aliceConn.packets)
	if err := bob.HandlePacket(ctx, &packet.MovePlayer{
		EntityRuntimeID: bob.runtimeID,
		Position:        mgl32.Vec3{4, 64, 5},
		Pitch:           5,
		Yaw:             45,
		HeadYaw:         45,
		Mode:            packet.MoveModeNormal,
		OnGround:        true,
		Tick:            43,
	}); err != nil {
		t.Fatalf("HandlePacket(MovePlayer) returned error: %v", err)
	}
	if pk := packetAt[*packet.MovePlayer](t, aliceConn.packets, beforeLegacyMove); pk.EntityRuntimeID != bob.runtimeID || pk.Position != (mgl32.Vec3{4, 64, 5}) || pk.Tick != 43 {
		t.Fatalf("legacy MovePlayer broadcast = %+v, want Bob moved to 4,64,5", pk)
	}
}

func spawnTestClient(t *testing.T, ctx context.Context, handler *MCPEHandler, name, identity string) (*MCPEClient, *recordingMCPEConn, *packet.StartGame) {
	t.Helper()

	conn := &recordingMCPEConn{}
	client := NewMCPEClient(handler, conn)
	if err := client.HandlePacket(ctx, &packet.RequestNetworkSettings{ClientProtocol: int32(gtprotocol.CurrentProtocol)}); err != nil {
		t.Fatalf("%s RequestNetworkSettings returned error: %v", name, err)
	}
	if err := client.HandlePacket(ctx, offlineLoginPacketFor(t, newClientKey(t), name, identity)); err != nil {
		t.Fatalf("%s Login returned error: %v", name, err)
	}
	if err := client.HandlePacket(ctx, &packet.ClientToServerHandshake{}); err != nil {
		t.Fatalf("%s ClientToServerHandshake returned error: %v", name, err)
	}
	if err := client.HandlePacket(ctx, &packet.ResourcePackClientResponse{Response: packet.PackResponseCompleted}); err != nil {
		t.Fatalf("%s ResourcePackClientResponse returned error: %v", name, err)
	}
	start := packetAt[*packet.StartGame](t, conn.packets, 5)
	if err := client.HandlePacket(ctx, &packet.RequestChunkRadius{ChunkRadius: 1}); err != nil {
		t.Fatalf("%s RequestChunkRadius returned error: %v", name, err)
	}
	if err := client.HandlePacket(ctx, &packet.SetLocalPlayerAsInitialised{EntityRuntimeID: start.EntityRuntimeID}); err != nil {
		t.Fatalf("%s SetLocalPlayerAsInitialised returned error: %v", name, err)
	}
	return client, conn, start
}

func offlineLoginPacketFor(t *testing.T, key *ecdsa.PrivateKey, name, identity string) *packet.Login {
	t.Helper()
	return &packet.Login{
		ClientProtocol: int32(gtprotocol.CurrentProtocol),
		ConnectionRequest: gtlogin.EncodeOffline(gtlogin.IdentityData{
			Identity:    identity,
			DisplayName: name,
		}, gtlogin.ClientData{
			DeviceOS:          gtprotocol.DeviceWin10,
			GameVersion:       gtprotocol.CurrentVersion,
			LanguageCode:      "en_US",
			SelfSignedID:      "01f4ce7b-26a1-4a8b-8bbf-c067b49d0d4e",
			ServerAddress:     "127.0.0.1:19132",
			SkinResourcePatch: base64.StdEncoding.EncodeToString([]byte(`{"geometry":{"default":"geometry.humanoid.custom"}}`)),
			SkinID:            name + "-skin",
			UIProfile:         1,
		}, key, true),
	}
}

func metadataFlag(metadata map[uint32]any, flag int) bool {
	key := uint32(gtprotocol.EntityDataKeyFlags)
	index := flag
	if flag >= 64 {
		key = gtprotocol.EntityDataKeyFlagsTwo
		index = flag - 64
	}
	flags, _ := metadata[key].(int64)
	return flags&(int64(1)<<uint(index)) != 0
}
