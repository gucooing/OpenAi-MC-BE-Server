package mcpe

import (
	"context"
	"fmt"
	"log/slog"
	"net"

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
}

type Server struct {
	transport *networkraknet.Server
	logger    *slog.Logger
	newClient ClientFactory
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
	transport, err := networkraknet.Listen(networkraknet.Options{
		Address: options.Address,
		Logger:  options.Logger,
		PongInfo: networkraknet.PongInfo{
			MOTD:             options.ServerName,
			ProtocolVersion:  gtprotocol.CurrentProtocol,
			MinecraftVersion: gtprotocol.CurrentVersion,
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
	session := newSession(server.newClient, server.logger, conn)
	if session.client == nil {
		return fmt.Errorf("mcpe client factory returned nil")
	}
	return session.Serve(ctx)
}
