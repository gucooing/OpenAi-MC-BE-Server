package server

import (
	"context"
	"fmt"

	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

func (client *MCPEClient) handleRequestChunkRadius(ctx context.Context, pk *packet.RequestChunkRadius) error {
	if pk.ChunkRadius < 1 {
		return fmt.Errorf("requested chunk radius must be at least 1, got %d", pk.ChunkRadius)
	}
	radius := pk.ChunkRadius
	if client.handler.viewDistance > 0 && int32(client.handler.viewDistance) < radius {
		radius = int32(client.handler.viewDistance)
	}
	if err := client.conn.WritePacket(&packet.ChunkRadiusUpdated{ChunkRadius: radius}); err != nil {
		return fmt.Errorf("send ChunkRadiusUpdated: %w", err)
	}
	if err := client.conn.WritePacket(&packet.BiomeDefinitionList{}); err != nil {
		return fmt.Errorf("send BiomeDefinitionList: %w", err)
	}
	if err := client.conn.WritePacket(&packet.CreativeContent{}); err != nil {
		return fmt.Errorf("send CreativeContent: %w", err)
	}
	if !client.chunksSent {
		if err := client.handler.chunks.SendInitial(ctx, client.conn); err != nil {
			return err
		}
		client.chunksSent = true
	}
	if err := client.conn.WritePacket(&packet.PlayStatus{Status: packet.PlayStatusPlayerSpawn}); err != nil {
		return fmt.Errorf("send PlayStatus player spawn: %w", err)
	}
	client.state = stateAwaitInitialised
	return nil
}

func (client *MCPEClient) handleSubChunkRequest(ctx context.Context, pk *packet.SubChunkRequest) error {
	return client.handler.chunks.SendSubChunks(ctx, client.conn, pk)
}

func (client *MCPEClient) handleSetLocalPlayerAsInitialised(_ context.Context, pk *packet.SetLocalPlayerAsInitialised) error {
	if pk.EntityRuntimeID != client.runtimeID {
		return fmt.Errorf("entity runtime ID mismatch: expected %d, got %d", client.runtimeID, pk.EntityRuntimeID)
	}
	client.state = stateSpawned
	if err := client.spawnToInitialisedPlayers(); err != nil {
		return err
	}
	if client.handler.logger != nil {
		client.handler.logger.Info("mcpe player spawned", "remote", client.conn.RemoteAddr(), "display_name", client.login.Identity.DisplayName)
	}
	return nil
}
