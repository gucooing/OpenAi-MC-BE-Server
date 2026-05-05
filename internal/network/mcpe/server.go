package mcpe

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	cryptorand "crypto/rand"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"

	networkraknet "gucooing/bds/internal/network/raknet"
	appprotocol "gucooing/bds/internal/protocol"
	appworld "gucooing/bds/internal/world"
)

type Options struct {
	Address      string
	ServerName   string
	ServerBrand  string
	GameMode     string
	MaxPlayers   int
	ViewDistance int
	OnlineMode   bool
	Logger       *slog.Logger
	World        appworld.ChunkProvider
}

type Server struct {
	transport    *networkraknet.Server
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
	nextID       atomic.Uint64
}

func Listen(options Options) (*Server, error) {
	if options.Address == "" {
		return nil, fmt.Errorf("mcpe listen address cannot be empty")
	}
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

	server := &Server{
		logger:       options.Logger,
		serverName:   options.ServerName,
		serverBrand:  options.ServerBrand,
		gameMode:     options.GameMode,
		maxPlayers:   options.MaxPlayers,
		viewDistance: options.ViewDistance,
		onlineMode:   options.OnlineMode,
		world:        options.World,
		chunks:       newChunkPublisher(options.World, int32(options.ViewDistance), options.Logger),
	}
	if _, err := server.encryptionPrivateKey(); err != nil {
		return nil, err
	}
	transport, err := networkraknet.Listen(networkraknet.Options{
		Address: options.Address,
		Logger:  options.Logger,
		PongInfo: networkraknet.PongInfo{
			MOTD:             options.ServerName,
			ProtocolVersion:  appprotocol.CurrentProtocol,
			MinecraftVersion: appprotocol.CurrentVersion,
			MaxPlayers:       options.MaxPlayers,
			ServerName:       options.ServerBrand,
			GameMode:         options.GameMode,
		},
		SessionHandler: server.serveSession,
	})
	if err != nil {
		return nil, err
	}
	server.transport = transport
	return server, nil
}

func (server *Server) Addr() net.Addr {
	return server.transport.Addr()
}

func (server *Server) Close() error {
	return server.transport.Close()
}

func (server *Server) serveSession(ctx context.Context, conn net.Conn) error {
	return newSession(server, conn).Serve(ctx)
}

func (server *Server) nextRuntimeID() uint64 {
	return server.nextID.Add(1)
}

func (server *Server) encryptionPrivateKey() (*ecdsa.PrivateKey, error) {
	server.keyMu.Lock()
	defer server.keyMu.Unlock()
	if server.privateKey != nil {
		return server.privateKey, nil
	}
	key, err := ecdsa.GenerateKey(elliptic.P384(), cryptorand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generating ECDSA key: %w", err)
	}
	server.privateKey = key
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
