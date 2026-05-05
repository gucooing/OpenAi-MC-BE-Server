package mcpe

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
	"time"

	dfchunk "github.com/df-mc/dragonfly/server/world/chunk"
	raknetlib "github.com/sandertv/go-raknet"
	gtprotocol "github.com/sandertv/gophertunnel/minecraft/protocol"
	gtlogin "github.com/sandertv/gophertunnel/minecraft/protocol/login"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"

	appprotocol "gucooing/bds/internal/protocol"
	appworld "gucooing/bds/internal/world"
)

func TestListenUsesOwnRakNetTransportForPing(t *testing.T) {
	server, err := Listen(Options{
		Address:      "127.0.0.1:0",
		ServerName:   "MCPE Ping Test",
		ServerBrand:  "BetterAltay-Go",
		GameMode:     "Survival",
		MaxPlayers:   5,
		ViewDistance: 2,
		OnlineMode:   false,
		Logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("Listen() returned error: %v", err)
	}
	defer server.Close()

	data, err := raknetlib.PingTimeout(server.Addr().String(), time.Second)
	if err != nil {
		t.Fatalf("PingTimeout() returned error: %v", err)
	}
	want := "MCPE;MCPE Ping Test;944;1.26.10;0;5;"
	if got := string(data); len(got) < len(want) || got[:len(want)] != want {
		t.Fatalf("ping data = %q, want prefix %q", got, want)
	}
}

func TestSessionHandlesNetworkSettingsLoginAndResourcePacks(t *testing.T) {
	world, err := appworld.NewFlatGenerator()
	if err != nil {
		t.Fatalf("NewFlatGenerator() returned error: %v", err)
	}
	server := &Server{
		logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		serverName:   "MCPE Session Test",
		serverBrand:  "BetterAltay-Go",
		gameMode:     "Survival",
		maxPlayers:   5,
		viewDistance: 2,
		world:        world,
		chunks:       newChunkPublisher(world, 1, nil),
	}

	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- newSession(server, serverConn).Serve(ctx)
	}()

	clientCodec := appprotocol.NewCodec()
	plainCodec := clientCodec
	plainCodec.Compression = nil
	writeClientPacket(t, clientConn, plainCodec, &packet.RequestNetworkSettings{ClientProtocol: int32(appprotocol.CurrentProtocol)})

	networkSettings := readClientPacket(t, clientConn, plainCodec, appprotocol.FromServer)
	if _, ok := networkSettings.(*packet.NetworkSettings); !ok {
		t.Fatalf("first response = %T, want *packet.NetworkSettings", networkSettings)
	}

	clientKey := newClientKey(t)
	writeClientPacket(t, clientConn, clientCodec, offlineLoginPacket(t, clientKey))
	handshakePacket := readClientPacket(t, clientConn, clientCodec, appprotocol.FromServer)
	handshake, ok := handshakePacket.(*packet.ServerToClientHandshake)
	if !ok {
		t.Fatalf("login response = %T, want *packet.ServerToClientHandshake", handshakePacket)
	}
	keyBytes, err := appprotocol.ClientEncryptionKey(handshake.JWT, clientKey)
	if err != nil {
		t.Fatalf("ClientEncryptionKey() returned error: %v", err)
	}
	clientCodec.EnableEncryption(keyBytes)
	writeClientPacket(t, clientConn, clientCodec, &packet.ClientToServerHandshake{})

	playStatus := readClientPacket(t, clientConn, clientCodec, appprotocol.FromServer)
	if pk, ok := playStatus.(*packet.PlayStatus); !ok || pk.Status != packet.PlayStatusLoginSuccess {
		t.Fatalf("login response = %T %+v, want PlayStatusLoginSuccess", playStatus, playStatus)
	}
	if _, ok := readClientPacket(t, clientConn, clientCodec, appprotocol.FromServer).(*packet.ResourcePacksInfo); !ok {
		t.Fatalf("second login response is not ResourcePacksInfo")
	}

	writeClientPacket(t, clientConn, clientCodec, &packet.ResourcePackClientResponse{Response: packet.PackResponseAllPacksDownloaded})
	if _, ok := readClientPacket(t, clientConn, clientCodec, appprotocol.FromServer).(*packet.ResourcePackStack); !ok {
		t.Fatalf("resource pack stack was not sent")
	}

	writeClientPacket(t, clientConn, clientCodec, &packet.ResourcePackClientResponse{Response: packet.PackResponseCompleted})
	startGame := readClientPacket(t, clientConn, clientCodec, appprotocol.FromServer)
	start, ok := startGame.(*packet.StartGame)
	if !ok {
		t.Fatalf("spawn response = %T, want *packet.StartGame", startGame)
	}
	if start.WorldName != "MCPE Session Test" || start.EntityRuntimeID == 0 {
		t.Fatalf("StartGame = %+v, want world name and runtime ID", start)
	}
	if _, ok := readClientPacket(t, clientConn, clientCodec, appprotocol.FromServer).(*packet.ItemRegistry); !ok {
		t.Fatalf("ItemRegistry was not sent after StartGame")
	}

	cancel()
	_ = clientConn.Close()
	if err := <-done; err != nil {
		t.Fatalf("Serve() returned error: %v", err)
	}
}

func TestLevelChunkPacketEncodesFlatSpawnChunk(t *testing.T) {
	world, err := appworld.NewFlatGenerator()
	if err != nil {
		t.Fatalf("NewFlatGenerator() returned error: %v", err)
	}
	chunk, err := world.Chunk(context.Background(), appworld.ChunkPos{X: 0, Z: 0})
	if err != nil {
		t.Fatalf("Chunk() returned error: %v", err)
	}
	pk, err := levelChunkPacket(chunk)
	if err != nil {
		t.Fatalf("levelChunkPacket() returned error: %v", err)
	}

	air, err := appworld.DefaultRuntimeRegistry().RuntimeID(appworld.AirBlock)
	if err != nil {
		t.Fatalf("RuntimeID(air) returned error: %v", err)
	}
	decoded, err := dfchunk.NetworkDecode(air, pk.RawPayload, int(pk.SubChunkCount), world.Dimension().Range())
	if err != nil {
		t.Fatalf("NetworkDecode(spawn chunk) returned error: %v", err)
	}
	grass, err := appworld.DefaultRuntimeRegistry().RuntimeID(appworld.GrassBlock)
	if err != nil {
		t.Fatalf("RuntimeID(grass) returned error: %v", err)
	}
	if got := decoded.Block(0, 63, 0, 0); got != grass {
		t.Fatalf("decoded spawn surface runtime ID = %d, want %d", got, grass)
	}
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
		ClientProtocol: int32(appprotocol.CurrentProtocol),
		ConnectionRequest: gtlogin.EncodeOffline(appprotocol.IdentityData{
			Identity:    "7b2d9639-5a8c-4f2f-9d8d-4d9f1e6e1f7a",
			DisplayName: "HandshakeBot",
		}, appprotocol.ClientData{
			DeviceOS:          gtprotocol.DeviceWin10,
			GameVersion:       appprotocol.CurrentVersion,
			LanguageCode:      "en_US",
			SelfSignedID:      "01f4ce7b-26a1-4a8b-8bbf-c067b49d0d4e",
			ServerAddress:     "127.0.0.1:19132",
			SkinResourcePatch: base64.StdEncoding.EncodeToString([]byte(`{"geometry":{"default":"geometry.humanoid.custom"}}`)),
			SkinID:            "test-skin",
			UIProfile:         1,
		}, key, true),
	}
}

func writeClientPacket(t *testing.T, conn net.Conn, codec appprotocol.Codec, pk appprotocol.Packet) {
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

func readClientPacket(t *testing.T, conn net.Conn, codec appprotocol.Codec, direction appprotocol.Direction) appprotocol.Packet {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("SetReadDeadline() returned error: %v", err)
	}
	buffer := make([]byte, 1<<20)
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
