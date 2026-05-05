package server

import (
	"context"
	"errors"
	"fmt"

	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

var errUnhandledPacket = errors.New("unhandled packet")

type packetHandler func(context.Context, packet.Packet) error

type packetRouter struct {
	routes map[uint32]packetHandler
}

func newMCPEClientRouter(client *MCPEClient) packetRouter {
	router := packetRouter{routes: make(map[uint32]packetHandler)}
	handlePacketRoute(&router, packet.IDRequestNetworkSettings, client.handleRequestNetworkSettings)
	handlePacketRoute(&router, packet.IDLogin, client.handleLogin)
	handlePacketRoute(&router, packet.IDClientToServerHandshake, client.handleClientToServerHandshake)
	handlePacketRoute(&router, packet.IDResourcePackClientResponse, client.handleResourcePackClientResponse)
	handlePacketRoute(&router, packet.IDClientCacheStatus, client.handleClientCacheStatus)
	handlePacketRoute(&router, packet.IDRequestChunkRadius, client.handleRequestChunkRadius)
	handlePacketRoute(&router, packet.IDSubChunkRequest, client.handleSubChunkRequest)
	handlePacketRoute(&router, packet.IDSetLocalPlayerAsInitialised, client.handleSetLocalPlayerAsInitialised)
	handlePacketRoute(&router, packet.IDPlayerAuthInput, client.handlePlayerAuthInput)
	handlePacketRoute(&router, packet.IDMovePlayer, client.handleMovePlayer)
	return router
}

func handlePacketRoute[T packet.Packet](router *packetRouter, id uint32, handler func(context.Context, T) error) {
	router.routes[id] = func(ctx context.Context, pk packet.Packet) error {
		typed, ok := pk.(T)
		if !ok {
			var want T
			return fmt.Errorf("packet id %d got %T want %T", id, pk, want)
		}
		return handler(ctx, typed)
	}
}

func (router packetRouter) dispatch(ctx context.Context, pk packet.Packet) error {
	handler := router.routes[pk.ID()]
	if handler == nil {
		return fmt.Errorf("%w: packet id %d (%T)", errUnhandledPacket, pk.ID(), pk)
	}
	return handler(ctx, pk)
}
