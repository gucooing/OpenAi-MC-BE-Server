package server

import (
	"context"
	cryptorand "crypto/rand"
	"fmt"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/google/uuid"
	gtprotocol "github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

func (client *MCPEClient) handleRequestNetworkSettings(_ context.Context, pk *packet.RequestNetworkSettings) error {
	if pk.ClientProtocol != int32(gtprotocol.CurrentProtocol) {
		status := packet.PlayStatusLoginFailedClient
		if pk.ClientProtocol > int32(gtprotocol.CurrentProtocol) {
			status = packet.PlayStatusLoginFailedServer
		}
		_ = client.conn.WritePacketUncompressed(&packet.PlayStatus{Status: status})
		return fmt.Errorf("incompatible protocol version: expected %d, got %d", gtprotocol.CurrentProtocol, pk.ClientProtocol)
	}

	if err := client.conn.WritePacketUncompressed(&packet.NetworkSettings{
		CompressionThreshold: uint16(client.conn.CompressionThreshold()),
		CompressionAlgorithm: client.conn.CompressionAlgorithm(),
	}); err != nil {
		return fmt.Errorf("send NetworkSettings: %w", err)
	}
	client.conn.EnableCompression()
	client.state = stateAwaitLogin
	return nil
}

func (client *MCPEClient) handleLogin(_ context.Context, pk *packet.Login) error {
	if pk.ClientProtocol != int32(gtprotocol.CurrentProtocol) {
		_ = client.conn.WritePacket(&packet.PlayStatus{Status: packet.PlayStatusLoginFailedClient})
		return fmt.Errorf("incompatible login protocol version: expected %d, got %d", gtprotocol.CurrentProtocol, pk.ClientProtocol)
	}
	loginData, err := parseLoginPacket(pk)
	if err != nil {
		return err
	}
	if client.handler.onlineMode && !loginData.Auth.XBOXLiveAuthenticated {
		_ = client.conn.WritePacket(&packet.Disconnect{Message: "Xbox Live authentication is required."})
		return fmt.Errorf("client was not authenticated to Xbox Live")
	}
	client.login = loginData
	handshake, keyBytes, err := client.serverHandshake(loginData)
	if err != nil {
		return err
	}
	if err := client.conn.WritePacket(handshake); err != nil {
		return fmt.Errorf("send ServerToClientHandshake: %w", err)
	}
	client.conn.EnableEncryption(keyBytes)
	client.state = stateAwaitClientHandshake
	return nil
}

func (client *MCPEClient) handleClientToServerHandshake(_ context.Context, _ *packet.ClientToServerHandshake) error {
	client.state = stateAwaitResourcePackResponse
	if client.handler.logger != nil {
		client.handler.logger.Info(
			"mcpe player login accepted",
			"remote", client.conn.RemoteAddr(),
			"display_name", client.login.Identity.DisplayName,
			"identity", client.login.Identity.Identity,
			"xuid", client.login.Identity.XUID,
			"xbox_live_authenticated", client.login.Auth.XBOXLiveAuthenticated,
		)
	}
	if err := client.conn.WritePacket(&packet.PlayStatus{Status: packet.PlayStatusLoginSuccess}); err != nil {
		return fmt.Errorf("send PlayStatus login success: %w", err)
	}
	if err := client.conn.WritePacket(&packet.ResourcePacksInfo{}); err != nil {
		return fmt.Errorf("send ResourcePacksInfo: %w", err)
	}
	return nil
}

func (client *MCPEClient) serverHandshake(loginData loginData) (*packet.ServerToClientHandshake, [32]byte, error) {
	if loginData.Auth.PublicKey == nil {
		return nil, [32]byte{}, fmt.Errorf("login request did not include a client public key")
	}
	privateKey, err := client.handler.encryptionPrivateKey()
	if err != nil {
		return nil, [32]byte{}, err
	}
	salt := make([]byte, handshakeSaltBytes)
	if _, err := cryptorand.Read(salt); err != nil {
		return nil, [32]byte{}, fmt.Errorf("generate handshake salt: %w", err)
	}
	return newServerHandshake(loginData.Auth.PublicKey, privateKey, salt)
}

func (client *MCPEClient) handleResourcePackClientResponse(ctx context.Context, pk *packet.ResourcePackClientResponse) error {
	switch pk.Response {
	case packet.PackResponseSendPacks:
		if len(pk.PacksToDownload) != 0 {
			return fmt.Errorf("resource pack downloads are not implemented")
		}
		return client.sendResourcePackStack()
	case packet.PackResponseAllPacksDownloaded:
		return client.sendResourcePackStack()
	case packet.PackResponseCompleted:
		return client.startGame(ctx)
	case packet.PackResponseRefused:
		return fmt.Errorf("client refused resource packs")
	default:
		return fmt.Errorf("unknown resource pack response %d", pk.Response)
	}
}

func (client *MCPEClient) handleClientCacheStatus(_ context.Context, pk *packet.ClientCacheStatus) error {
	client.clientCacheEnabled = pk.Enabled
	return nil
}

func (client *MCPEClient) sendResourcePackStack() error {
	return client.conn.WritePacket(&packet.ResourcePackStack{
		BaseGameVersion: gtprotocol.CurrentVersion,
	})
}

func (client *MCPEClient) startGame(_ context.Context) error {
	client.runtimeID = client.handler.nextRuntimeID()
	if err := client.initPlayerState(); err != nil {
		return err
	}
	existingPlayers := client.handler.addPlayer(client)
	if err := client.conn.WritePacket(client.startGamePacket()); err != nil {
		return fmt.Errorf("send StartGame: %w", err)
	}
	if err := client.conn.WritePacket(&packet.ItemRegistry{}); err != nil {
		return fmt.Errorf("send ItemRegistry: %w", err)
	}
	if err := client.sendInitialPlayerSync(existingPlayers); err != nil {
		return err
	}
	client.state = stateAwaitChunkRadius
	if client.handler.logger != nil {
		client.handler.logger.Info("mcpe start game sent", "remote", client.conn.RemoteAddr(), "display_name", client.login.Identity.DisplayName)
	}
	return nil
}

func (client *MCPEClient) startGamePacket() *packet.StartGame {
	handler := client.handler
	gameMode := gameModeID(handler.gameMode)
	spawn := handler.world.Spawn()
	spawnBlock := handler.world.SpawnBlock()
	return &packet.StartGame{
		EntityUniqueID:               int64(client.runtimeID),
		EntityRuntimeID:              client.runtimeID,
		PlayerGameMode:               gameMode,
		PlayerPosition:               mgl32.Vec3{spawn.X, spawn.Y, spawn.Z},
		WorldSeed:                    1,
		SpawnBiomeType:               packet.SpawnBiomeTypeDefault,
		Dimension:                    handler.world.Dimension().ID(),
		Generator:                    1,
		WorldGameMode:                gameMode,
		Difficulty:                   1,
		WorldSpawn:                   gtprotocol.BlockPos{spawnBlock.X, spawnBlock.Y, spawnBlock.Z},
		AchievementsDisabled:         true,
		MultiPlayerGame:              true,
		LANBroadcastEnabled:          true,
		CommandsEnabled:              true,
		PlayerPermissions:            1,
		ServerChunkTickRadius:        int32(handler.viewDistance),
		BaseGameVersion:              gtprotocol.CurrentVersion,
		LevelID:                      handler.serverName,
		WorldName:                    handler.serverName,
		PlayerMovementSettings:       gtprotocol.PlayerMovementSettings{},
		MultiPlayerCorrelationID:     uuid.NewString(),
		ServerAuthoritativeInventory: true,
		GameVersion:                  gtprotocol.CurrentVersion,
		PropertyData:                 map[string]any{},
	}
}
