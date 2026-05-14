package mcpe

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"

	gtprotocol "github.com/sandertv/gophertunnel/minecraft/protocol"
	networkraknet "gucooing/bds/internal/network/raknet"
)

type Options struct {
	Address     string
	ServerName  string
	ServerBrand string
	GameMode    string
	MaxPlayers  int
	Logger      *slog.Logger
	NewClient   ClientFactory
	OnlineCount func() int
}

type Server struct {
	transport *networkraknet.Server
	logger    *slog.Logger
	newClient ClientFactory
	pongMu    sync.Mutex
	pongInfo  networkraknet.PongInfo
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
	if options.NewClient == nil {
		return nil, fmt.Errorf("mcpe client factory cannot be nil")
	}
	server := &Server{logger: options.Logger, newClient: options.NewClient}
	server.pongInfo = networkraknet.PongInfo{
		MOTD:             options.ServerName,
		ProtocolVersion:  gtprotocol.CurrentProtocol,
		MinecraftVersion: gtprotocol.CurrentVersion,
		MaxPlayers:       options.MaxPlayers,
		ServerName:       options.ServerBrand,
		GameMode:         options.GameMode,
	}
	if options.OnlineCount != nil {
		server.pongInfo.OnlinePlayers = options.OnlineCount()
	}
	transport, err := networkraknet.Listen(networkraknet.Options{
		Address:        options.Address,
		Logger:         options.Logger,
		PongInfo:       server.pongInfo,
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

func (server *Server) UpdateOnlinePlayers(online int) {
	if online < 0 {
		online = 0
	}
	server.pongMu.Lock()
	defer server.pongMu.Unlock()

	info := server.pongInfo
	info.OnlinePlayers = online
	server.pongInfo = info
	server.transport.SetPongInfo(info)
}

func (server *Server) serveSession(ctx context.Context, conn net.Conn) error {
	session := newSession(server.newClient, server.logger, conn)
	if session.client == nil {
		return fmt.Errorf("mcpe client factory returned nil")
	}
	return session.Serve(ctx)
}
