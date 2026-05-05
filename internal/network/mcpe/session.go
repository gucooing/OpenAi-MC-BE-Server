package mcpe

import (
	"context"
	cryptorand "crypto/rand"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/google/uuid"
	gtprotocol "github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"

	appprotocol "gucooing/bds/internal/protocol"
)

type sessionState uint8

const (
	stateAwaitNetworkSettings sessionState = iota
	stateAwaitLogin
	stateAwaitClientHandshake
	stateAwaitResourcePackResponse
	stateAwaitChunkRadius
	stateAwaitInitialised
	stateSpawned
)

type session struct {
	server     *Server
	conn       net.Conn
	codec      appprotocol.Codec
	logger     *slog.Logger
	state      sessionState
	compressed bool
	runtimeID  uint64
	login      appprotocol.LoginData
	writeMu    sync.Mutex
	chunksSent bool
}

func newSession(server *Server, conn net.Conn) *session {
	return &session{
		server: server,
		conn:   conn,
		codec:  appprotocol.NewCodec(),
		logger: server.logger,
		state:  stateAwaitNetworkSettings,
	}
}

func (session *session) Serve(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = session.conn.Close()
		case <-done:
		}
	}()
	defer close(done)

	buffer := make([]byte, appprotocol.MaxDecompressedBatchBytes)
	for {
		n, err := session.conn.Read(buffer)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrClosedPipe) || errors.Is(err, net.ErrClosed) {
				return nil
			}
			return fmt.Errorf("read mcpe batch: %w", err)
		}
		if n == 0 {
			continue
		}
		if err := session.handleBatch(ctx, buffer[:n]); err != nil {
			return err
		}
	}
}

func (session *session) handleBatch(ctx context.Context, data []byte) error {
	packets, err := session.decodeBatch(data)
	if err != nil {
		return fmt.Errorf("decode mcpe batch: %w", err)
	}
	for _, payload := range packets {
		pk, err := session.codec.DecodePacket(payload, appprotocol.FromClient)
		if err != nil {
			return fmt.Errorf("decode mcpe packet: %w", err)
		}
		if session.logger != nil && pk.ID() != packet.IDPlayerAuthInput {
			session.logger.Debug("mcpe packet received", "remote", session.conn.RemoteAddr(), "packet_id", pk.ID(), "packet", fmt.Sprintf("%T", pk), "state", session.state)
		}
		if err := session.handlePacket(ctx, pk); err != nil {
			return err
		}
	}
	return nil
}

func (session *session) handlePacket(ctx context.Context, pk appprotocol.Packet) error {
	switch pk := pk.(type) {
	case *packet.RequestNetworkSettings:
		return session.handleRequestNetworkSettings(pk)
	case *packet.Login:
		return session.handleLogin(pk)
	case *packet.ClientToServerHandshake:
		return session.handleClientToServerHandshake()
	case *packet.ResourcePackClientResponse:
		return session.handleResourcePackClientResponse(ctx, pk)
	case *packet.RequestChunkRadius:
		return session.handleRequestChunkRadius(ctx, pk)
	case *packet.SetLocalPlayerAsInitialised:
		return session.handleSetLocalPlayerAsInitialised(pk)
	default:
		session.logger.Error("未处理的数据包", "remote", session.conn.RemoteAddr(), "packet_id", pk.ID(), "packet", fmt.Sprintf("%T", pk), "state", session.state)
		return nil
	}
}

func (session *session) handleRequestNetworkSettings(pk *packet.RequestNetworkSettings) error {
	if session.state != stateAwaitNetworkSettings {
		return fmt.Errorf("unexpected RequestNetworkSettings while in state %d", session.state)
	}
	if pk.ClientProtocol != int32(appprotocol.CurrentProtocol) {
		status := packet.PlayStatusLoginFailedClient
		if pk.ClientProtocol > int32(appprotocol.CurrentProtocol) {
			status = packet.PlayStatusLoginFailedServer
		}
		_ = session.writePacket(&packet.PlayStatus{Status: status}, false)
		return fmt.Errorf("incompatible protocol version: expected %d, got %d", appprotocol.CurrentProtocol, pk.ClientProtocol)
	}

	if err := session.writePacket(&packet.NetworkSettings{
		CompressionThreshold: uint16(session.codec.CompressionThreshold),
		CompressionAlgorithm: session.codec.Compression.EncodeCompression(),
	}, false); err != nil {
		return fmt.Errorf("send NetworkSettings: %w", err)
	}
	session.compressed = true
	session.state = stateAwaitLogin
	return nil
}

func (session *session) handleLogin(pk *packet.Login) error {
	if session.state != stateAwaitLogin {
		return fmt.Errorf("unexpected Login while in state %d", session.state)
	}
	if pk.ClientProtocol != int32(appprotocol.CurrentProtocol) {
		_ = session.WritePacket(&packet.PlayStatus{Status: packet.PlayStatusLoginFailedClient})
		return fmt.Errorf("incompatible login protocol version: expected %d, got %d", appprotocol.CurrentProtocol, pk.ClientProtocol)
	}
	loginData, err := appprotocol.ParseLoginPacket(pk)
	if err != nil {
		return err
	}
	if session.server.onlineMode && !loginData.Auth.XBOXLiveAuthenticated {
		_ = session.WritePacket(&packet.Disconnect{Message: "Xbox Live authentication is required."})
		return fmt.Errorf("client was not authenticated to Xbox Live")
	}
	session.login = loginData
	handshake, keyBytes, err := session.serverHandshake(loginData)
	if err != nil {
		return err
	}
	if err := session.WritePacket(handshake); err != nil {
		return fmt.Errorf("send ServerToClientHandshake: %w", err)
	}
	session.codec.EnableEncryption(keyBytes)
	session.state = stateAwaitClientHandshake
	return nil
}

func (session *session) handleClientToServerHandshake() error {
	if session.state != stateAwaitClientHandshake {
		return fmt.Errorf("unexpected ClientToServerHandshake while in state %d", session.state)
	}
	session.state = stateAwaitResourcePackResponse
	if session.logger != nil {
		session.logger.Info(
			"mcpe player login accepted",
			"remote", session.conn.RemoteAddr(),
			"display_name", session.login.Identity.DisplayName,
			"identity", session.login.Identity.Identity,
			"xuid", session.login.Identity.XUID,
			"xbox_live_authenticated", session.login.Auth.XBOXLiveAuthenticated,
		)
	}
	if err := session.WritePacket(&packet.PlayStatus{Status: packet.PlayStatusLoginSuccess}); err != nil {
		return fmt.Errorf("send PlayStatus login success: %w", err)
	}
	if err := session.WritePacket(&packet.ResourcePacksInfo{}); err != nil {
		return fmt.Errorf("send ResourcePacksInfo: %w", err)
	}
	return nil
}

func (session *session) serverHandshake(loginData appprotocol.LoginData) (*packet.ServerToClientHandshake, [32]byte, error) {
	if loginData.Auth.PublicKey == nil {
		return nil, [32]byte{}, fmt.Errorf("login request did not include a client public key")
	}
	privateKey, err := session.server.encryptionPrivateKey()
	if err != nil {
		return nil, [32]byte{}, err
	}
	salt := make([]byte, appprotocol.HandshakeSaltBytes)
	if _, err := cryptorand.Read(salt); err != nil {
		return nil, [32]byte{}, fmt.Errorf("generate handshake salt: %w", err)
	}
	return appprotocol.NewServerHandshake(loginData.Auth.PublicKey, privateKey, salt)
}

func (session *session) handleResourcePackClientResponse(ctx context.Context, pk *packet.ResourcePackClientResponse) error {
	if session.state != stateAwaitResourcePackResponse {
		return fmt.Errorf("unexpected ResourcePackClientResponse while in state %d", session.state)
	}
	switch pk.Response {
	case packet.PackResponseSendPacks:
		if len(pk.PacksToDownload) != 0 {
			return fmt.Errorf("resource pack downloads are not implemented")
		}
		return session.sendResourcePackStack()
	case packet.PackResponseAllPacksDownloaded:
		return session.sendResourcePackStack()
	case packet.PackResponseCompleted:
		return session.startGame(ctx)
	case packet.PackResponseRefused:
		return fmt.Errorf("client refused resource packs")
	default:
		return fmt.Errorf("unknown resource pack response %d", pk.Response)
	}
}

func (session *session) sendResourcePackStack() error {
	return session.WritePacket(&packet.ResourcePackStack{
		BaseGameVersion: appprotocol.CurrentVersion,
	})
}

func (session *session) startGame(ctx context.Context) error {
	session.runtimeID = session.server.nextRuntimeID()
	if err := session.WritePacket(session.startGamePacket()); err != nil {
		return fmt.Errorf("send StartGame: %w", err)
	}
	if err := session.WritePacket(&packet.ItemRegistry{}); err != nil {
		return fmt.Errorf("send ItemRegistry: %w", err)
	}
	session.state = stateAwaitChunkRadius
	if session.logger != nil {
		session.logger.Info("mcpe start game sent", "remote", session.conn.RemoteAddr(), "display_name", session.login.Identity.DisplayName)
	}
	return nil
}

func (session *session) startGamePacket() *packet.StartGame {
	gameMode := gameModeID(session.server.gameMode)
	spawn := session.server.world.Spawn()
	spawnBlock := session.server.world.SpawnBlock()
	return &packet.StartGame{
		EntityUniqueID:               int64(session.runtimeID),
		EntityRuntimeID:              session.runtimeID,
		PlayerGameMode:               gameMode,
		PlayerPosition:               mgl32.Vec3{spawn.X, spawn.Y, spawn.Z},
		WorldSeed:                    1,
		SpawnBiomeType:               packet.SpawnBiomeTypeDefault,
		Dimension:                    session.server.world.Dimension().ID(),
		Generator:                    1,
		WorldGameMode:                gameMode,
		Difficulty:                   1,
		WorldSpawn:                   gtprotocol.BlockPos{spawnBlock.X, spawnBlock.Y, spawnBlock.Z},
		AchievementsDisabled:         true,
		MultiPlayerGame:              true,
		LANBroadcastEnabled:          true,
		CommandsEnabled:              true,
		PlayerPermissions:            1,
		ServerChunkTickRadius:        int32(session.server.viewDistance),
		BaseGameVersion:              appprotocol.CurrentVersion,
		LevelID:                      session.server.serverName,
		WorldName:                    session.server.serverName,
		PlayerMovementSettings:       gtprotocol.PlayerMovementSettings{},
		MultiPlayerCorrelationID:     uuid.NewString(),
		ServerAuthoritativeInventory: true,
		GameVersion:                  appprotocol.CurrentVersion,
		PropertyData:                 map[string]any{},
	}
}

func (session *session) handleRequestChunkRadius(ctx context.Context, pk *packet.RequestChunkRadius) error {
	if session.state != stateAwaitChunkRadius && session.state != stateAwaitInitialised && session.state != stateSpawned {
		return fmt.Errorf("unexpected RequestChunkRadius while in state %d", session.state)
	}
	if pk.ChunkRadius < 1 {
		return fmt.Errorf("requested chunk radius must be at least 1, got %d", pk.ChunkRadius)
	}
	radius := pk.ChunkRadius
	if session.server.viewDistance > 0 && int32(session.server.viewDistance) < radius {
		radius = int32(session.server.viewDistance)
	}
	if err := session.WritePacket(&packet.ChunkRadiusUpdated{ChunkRadius: radius}); err != nil {
		return fmt.Errorf("send ChunkRadiusUpdated: %w", err)
	}
	if err := session.WritePacket(&packet.BiomeDefinitionList{}); err != nil {
		return fmt.Errorf("send BiomeDefinitionList: %w", err)
	}
	if err := session.WritePacket(&packet.PlayStatus{Status: packet.PlayStatusPlayerSpawn}); err != nil {
		return fmt.Errorf("send PlayStatus player spawn: %w", err)
	}
	if err := session.WritePacket(&packet.CreativeContent{}); err != nil {
		return fmt.Errorf("send CreativeContent: %w", err)
	}
	if !session.chunksSent {
		if err := session.server.chunks.SendInitial(ctx, session); err != nil {
			return err
		}
		session.chunksSent = true
	}
	session.state = stateAwaitInitialised
	return nil
}

func (session *session) handleSetLocalPlayerAsInitialised(pk *packet.SetLocalPlayerAsInitialised) error {
	if session.state != stateAwaitInitialised && session.state != stateSpawned {
		return fmt.Errorf("unexpected SetLocalPlayerAsInitialised while in state %d", session.state)
	}
	if pk.EntityRuntimeID != session.runtimeID {
		return fmt.Errorf("entity runtime ID mismatch: expected %d, got %d", session.runtimeID, pk.EntityRuntimeID)
	}
	session.state = stateSpawned
	if session.logger != nil {
		session.logger.Info("mcpe player spawned", "remote", session.conn.RemoteAddr(), "display_name", session.login.Identity.DisplayName)
	}
	return nil
}

func (session *session) decodeBatch(data []byte) ([][]byte, error) {
	codec := session.codec
	if !session.compressed {
		codec.Compression = nil
	}
	return codec.DecodeBatch(data)
}

func (session *session) WritePacket(pk appprotocol.Packet) error {
	return session.writePacket(pk, session.compressed)
}

func (session *session) Flush() error {
	return nil
}

func (session *session) RemoteAddr() net.Addr {
	return session.conn.RemoteAddr()
}

func (session *session) writePacket(pk appprotocol.Packet, compressed bool) error {
	session.writeMu.Lock()
	defer session.writeMu.Unlock()

	payload, err := session.codec.EncodePacket(pk)
	if err != nil {
		return err
	}
	codec := session.codec
	if !compressed {
		codec.Compression = nil
	}
	batch, err := codec.EncodeBatch([][]byte{payload})
	if err != nil {
		return err
	}

	_, err = session.conn.Write(batch)
	return err
}
