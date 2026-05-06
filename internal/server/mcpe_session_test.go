package server

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	cryptorand "crypto/rand"
	"encoding/base64"
	"io"
	"log/slog"
	"net"
	"testing"

	gtprotocol "github.com/sandertv/gophertunnel/minecraft/protocol"
	gtlogin "github.com/sandertv/gophertunnel/minecraft/protocol/login"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"

	appworld "gucooing/bds/internal/world"
)

func TestMCPEClientHandlesLoginResourcePacksAndWorldSync(t *testing.T) {
	world, err := appworld.NewFlatGenerator()
	if err != nil {
		t.Fatalf("NewFlatGenerator() returned error: %v", err)
	}
	handler, err := NewMCPEHandler(MCPEOptions{
		Logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		ServerName:   "MCPE Session Test",
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
	conn := &recordingMCPEConn{}
	client := NewMCPEClient(handler, conn)

	if err := client.HandlePacket(ctx, &packet.RequestNetworkSettings{ClientProtocol: int32(gtprotocol.CurrentProtocol)}); err != nil {
		t.Fatalf("HandlePacket(RequestNetworkSettings) returned error: %v", err)
	}
	if _ = packetAt[*packet.NetworkSettings](t, conn.uncompressed, 0); !conn.compressionEnabled {
		t.Fatalf("compression was not enabled after NetworkSettings")
	}

	if err := client.HandlePacket(ctx, offlineLoginPacket(t, newClientKey(t))); err != nil {
		t.Fatalf("HandlePacket(Login) returned error: %v", err)
	}
	packetAt[*packet.ServerToClientHandshake](t, conn.packets, 0)
	if !conn.encryptionEnabled {
		t.Fatalf("encryption was not enabled after Login")
	}

	if err := client.HandlePacket(ctx, &packet.ClientToServerHandshake{}); err != nil {
		t.Fatalf("HandlePacket(ClientToServerHandshake) returned error: %v", err)
	}
	if pk := packetAt[*packet.PlayStatus](t, conn.packets, 1); pk.Status != packet.PlayStatusLoginSuccess {
		t.Fatalf("login status = %d, want PlayStatusLoginSuccess", pk.Status)
	}
	packetAt[*packet.ResourcePacksInfo](t, conn.packets, 2)

	if err := client.HandlePacket(ctx, &packet.ClientCacheStatus{Enabled: true}); err != nil {
		t.Fatalf("HandlePacket(ClientCacheStatus) returned error: %v", err)
	}
	if !client.clientCacheEnabled {
		t.Fatalf("client cache status was not recorded")
	}

	if err := client.HandlePacket(ctx, &packet.ResourcePackClientResponse{Response: packet.PackResponseAllPacksDownloaded}); err != nil {
		t.Fatalf("HandlePacket(ResourcePackClientResponse AllPacksDownloaded) returned error: %v", err)
	}
	packetAt[*packet.ResourcePackStack](t, conn.packets, 3)

	if err := client.HandlePacket(ctx, &packet.ResourcePackClientResponse{Response: packet.PackResponseCompleted}); err != nil {
		t.Fatalf("HandlePacket(ResourcePackClientResponse Completed) returned error: %v", err)
	}
	packetAt[*packet.JigsawStructureData](t, conn.packets, 4)
	packetAt[*packet.VoxelShapes](t, conn.packets, 5)
	start := packetAt[*packet.StartGame](t, conn.packets, 6)
	if start.WorldName != "MCPE Session Test" || start.EntityRuntimeID == 0 {
		t.Fatalf("StartGame = %+v, want world name and runtime ID", start)
	}
	if !start.UseBlockNetworkIDHashes {
		t.Fatalf("StartGame.UseBlockNetworkIDHashes = false, want true")
	}
	packetAt[*packet.ItemRegistry](t, conn.packets, 7)
	if pk := packetAt[*packet.AvailableCommands](t, conn.packets, 8); len(pk.Commands) != 2 {
		t.Fatalf("AvailableCommands count = %d, want default player commands", len(pk.Commands))
	}
	if pk := packetAt[*packet.PlayerList](t, conn.packets, 9); pk.ActionType != packet.PlayerListActionAdd || len(pk.Entries) != 1 || pk.Entries[0].Username != "HandshakeBot" {
		t.Fatalf("PlayerList = %+v, want single HandshakeBot entry", pk)
	}
	if pk := packetAt[*packet.SetActorData](t, conn.packets, 10); pk.EntityRuntimeID != start.EntityRuntimeID {
		t.Fatalf("SetActorData runtime ID = %d, want %d", pk.EntityRuntimeID, start.EntityRuntimeID)
	}
	if pk := packetAt[*packet.SetActorMotion](t, conn.packets, 11); pk.EntityRuntimeID != start.EntityRuntimeID {
		t.Fatalf("SetActorMotion runtime ID = %d, want %d", pk.EntityRuntimeID, start.EntityRuntimeID)
	}

	if err := client.HandlePacket(ctx, &packet.RequestChunkRadius{ChunkRadius: 1}); err != nil {
		t.Fatalf("HandlePacket(RequestChunkRadius) returned error: %v", err)
	}
	if pk := packetAt[*packet.ChunkRadiusUpdated](t, conn.packets, 12); pk.ChunkRadius != 1 {
		t.Fatalf("ChunkRadiusUpdated radius = %d, want 1", pk.ChunkRadius)
	}
	packetAt[*packet.BiomeDefinitionList](t, conn.packets, 13)
	packetAt[*packet.CreativeContent](t, conn.packets, 14)
	packetAt[*packet.NetworkChunkPublisherUpdate](t, conn.packets, 15)
	if pk := packetAt[*packet.SetTime](t, conn.packets, 16); pk.Time != 0 {
		t.Fatalf("SetTime time = %d, want 0", pk.Time)
	}
	if pk := packetAt[*packet.SetSpawnPosition](t, conn.packets, 17); pk.SpawnPosition != (gtprotocol.BlockPos{0, 64, 0}) {
		t.Fatalf("SetSpawnPosition = %+v, want spawn 0,64,0", pk)
	}
	for i := 0; i < 9; i++ {
		if pk := packetAt[*packet.LevelChunk](t, conn.packets, 18+i); pk.SubChunkCount != gtprotocol.SubChunkRequestModeLimited {
			t.Fatalf("LevelChunk %d sub chunk count = %d, want limited request mode", i, pk.SubChunkCount)
		}
	}
	if pk := packetAt[*packet.PlayStatus](t, conn.packets, 27); pk.Status != packet.PlayStatusPlayerSpawn {
		t.Fatalf("spawn status = %d, want PlayStatusPlayerSpawn", pk.Status)
	}

	if err := client.HandlePacket(ctx, &packet.SetLocalPlayerAsInitialised{EntityRuntimeID: start.EntityRuntimeID}); err != nil {
		t.Fatalf("HandlePacket(SetLocalPlayerAsInitialised) returned error: %v", err)
	}
	if client.state != stateSpawned {
		t.Fatalf("client state = %d, want stateSpawned", client.state)
	}
}

type recordingMCPEConn struct {
	packets            []packet.Packet
	uncompressed       []packet.Packet
	compressionEnabled bool
	encryptionEnabled  bool
}

func (conn *recordingMCPEConn) WritePacket(pk packet.Packet) error {
	conn.packets = append(conn.packets, pk)
	return nil
}

func (conn *recordingMCPEConn) WritePacketUncompressed(pk packet.Packet) error {
	conn.uncompressed = append(conn.uncompressed, pk)
	return nil
}

func (conn *recordingMCPEConn) EnableCompression() {
	conn.compressionEnabled = true
}

func (conn *recordingMCPEConn) EnableEncryption([32]byte) {
	conn.encryptionEnabled = true
}

func (conn *recordingMCPEConn) CompressionThreshold() int {
	return 256
}

func (conn *recordingMCPEConn) CompressionAlgorithm() uint16 {
	return packet.DefaultCompression.EncodeCompression()
}

func (conn *recordingMCPEConn) Flush() error {
	return nil
}

func (conn *recordingMCPEConn) RemoteAddr() net.Addr {
	return nil
}

func packetAt[T packet.Packet](t *testing.T, packets []packet.Packet, index int) T {
	t.Helper()
	if index >= len(packets) {
		t.Fatalf("packet index %d out of range; wrote %d packets", index, len(packets))
	}
	pk, ok := packets[index].(T)
	if !ok {
		var want T
		t.Fatalf("packet %d = %T, want %T", index, packets[index], want)
	}
	return pk
}

func newClientKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P384(), cryptorand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey() returned error: %v", err)
	}
	return key
}

func offlineLoginPacket(t *testing.T, key *ecdsa.PrivateKey) *packet.Login {
	t.Helper()
	return &packet.Login{
		ClientProtocol: int32(gtprotocol.CurrentProtocol),
		ConnectionRequest: gtlogin.EncodeOffline(gtlogin.IdentityData{
			Identity:    "7b2d9639-5a8c-4f2f-9d8d-4d9f1e6e1f7a",
			DisplayName: "HandshakeBot",
		}, gtlogin.ClientData{
			DeviceOS:          gtprotocol.DeviceWin10,
			GameVersion:       gtprotocol.CurrentVersion,
			LanguageCode:      "en_US",
			SelfSignedID:      "01f4ce7b-26a1-4a8b-8bbf-c067b49d0d4e",
			ServerAddress:     "127.0.0.1:19132",
			SkinResourcePatch: base64.StdEncoding.EncodeToString([]byte(`{"geometry":{"default":"geometry.humanoid.custom"}}`)),
			SkinID:            "test-skin",
			UIProfile:         1,
		}, key, true),
	}
}
