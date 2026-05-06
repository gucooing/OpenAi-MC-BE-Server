package server

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	cryptorand "crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"

	appcommand "gucooing/bds/internal/command"
	appworld "gucooing/bds/internal/world"
)

type MCPEOptions struct {
	ServerName   string
	ServerBrand  string
	GameMode     string
	MaxPlayers   int
	ViewDistance int
	OnlineMode   bool
	Logger       *slog.Logger
	World        appworld.ChunkProvider
	Shutdown     func()
}

type MCPEHandler struct {
	logger       *slog.Logger
	serverName   string
	serverBrand  string
	gameMode     string
	maxPlayers   int
	viewDistance int
	onlineMode   bool
	world        appworld.ChunkProvider
	chunks       chunkPublisher
	keyMu        sync.Mutex
	privateKey   *ecdsa.PrivateKey
	playersMu    sync.RWMutex
	players      map[uint64]*MCPEClient
	nextID       atomic.Uint64
	commands     *appcommand.Registry
	permissions  *permissionManager
	shutdown     func()
}

type MCPEConn interface {
	WritePacket(packet.Packet) error
	WritePacketUncompressed(packet.Packet) error
	EnableCompression()
	EnableEncryption([32]byte)
	CompressionThreshold() int
	CompressionAlgorithm() uint16
	Flush() error
	RemoteAddr() net.Addr
}

type mcpeSessionState uint8

const (
	stateAwaitNetworkSettings mcpeSessionState = iota
	stateAwaitLogin
	stateAwaitClientHandshake
	stateAwaitResourcePackResponse
	stateAwaitChunkRadius
	stateAwaitInitialised
	stateSpawned
)

type MCPEClient struct {
	handler            *MCPEHandler
	conn               MCPEConn
	state              mcpeSessionState
	runtimeID          uint64
	login              loginData
	player             mcpePlayerState
	inventory          *mcpeInventory
	packets            packetRouter
	chunksSent         bool
	clientCacheEnabled bool
	loadingScreenOpen  bool
	loadingScreenID    uint32
	loadingScreenIDOK  bool
	inventoryOpen      bool
}

func NewMCPEHandler(options MCPEOptions) (*MCPEHandler, error) {
	if options.ServerName == "" {
		return nil, fmt.Errorf("mcpe server name cannot be empty")
	}
	if options.ServerBrand == "" {
		options.ServerBrand = "BetterAltay-Go"
	}
	if options.MaxPlayers < 1 {
		options.MaxPlayers = 1
	}
	if options.GameMode == "" {
		options.GameMode = "Survival"
	}
	if options.World == nil {
		world, err := appworld.NewFlatGenerator()
		if err != nil {
			return nil, err
		}
		options.World = world
	}
	handler := &MCPEHandler{
		logger:       options.Logger,
		serverName:   options.ServerName,
		serverBrand:  options.ServerBrand,
		gameMode:     options.GameMode,
		maxPlayers:   options.MaxPlayers,
		viewDistance: options.ViewDistance,
		onlineMode:   options.OnlineMode,
		world:        options.World,
		chunks:       newChunkPublisher(options.World, int32(options.ViewDistance), options.Logger),
		players:      make(map[uint64]*MCPEClient),
		permissions:  newPermissionManager(),
		shutdown:     options.Shutdown,
	}
	handler.commands = newDefaultCommands(handler)
	if _, err := handler.encryptionPrivateKey(); err != nil {
		return nil, err
	}
	return handler, nil
}

func NewMCPEClient(handler *MCPEHandler, conn MCPEConn) *MCPEClient {
	client := &MCPEClient{
		handler:   handler,
		conn:      conn,
		state:     stateAwaitNetworkSettings,
		inventory: newMCPEInventory(),
	}
	client.packets = newMCPEClientRouter(client)
	return client
}

func (client *MCPEClient) HandlePacket(ctx context.Context, pk packet.Packet) error {
	err := client.packets.dispatch(ctx, pk)
	if errors.Is(err, errUnhandledPacket) {
		if client.handler.logger != nil {
			client.handler.logger.Error("未处理的数据包", "remote", client.conn.RemoteAddr(), "packet_id", pk.ID(), "packet", fmt.Sprintf("%T", pk), "state", client.state)
		}
		return nil
	}
	return err
}

func (client *MCPEClient) State() int {
	return int(client.state)
}

func (handler *MCPEHandler) nextRuntimeID() uint64 {
	return handler.nextID.Add(1)
}

func (handler *MCPEHandler) encryptionPrivateKey() (*ecdsa.PrivateKey, error) {
	handler.keyMu.Lock()
	defer handler.keyMu.Unlock()
	if handler.privateKey != nil {
		return handler.privateKey, nil
	}
	key, err := ecdsa.GenerateKey(elliptic.P384(), cryptorand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generating ECDSA key: %w", err)
	}
	handler.privateKey = key
	return key, nil
}

func gameModeID(value string) int32 {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "creative", "1":
		return packet.GameTypeCreative
	case "adventure", "2":
		return packet.GameTypeAdventure
	case "spectator", "3", "6":
		return packet.GameTypeSpectator
	default:
		return packet.GameTypeSurvival
	}
}
