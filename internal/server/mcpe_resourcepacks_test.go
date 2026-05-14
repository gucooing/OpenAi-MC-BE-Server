package server

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	gtprotocol "github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"

	appresourcepack "gucooing/bds/internal/resourcepack"
	appworld "gucooing/bds/internal/world"
)

func TestMCPEClientStreamsConfiguredResourcePackBeforeStartGame(t *testing.T) {
	world, err := appworld.NewFlatGenerator()
	if err != nil {
		t.Fatalf("NewFlatGenerator() returned error: %v", err)
	}

	packUUID := uuid.MustParse("7e7cb7db-8f0d-4e3a-9e32-4a5d0f53f001")
	packs, err := appresourcepack.Normalize([]appresourcepack.Pack{{
		UUID:     packUUID,
		Version:  "1.0.0",
		Data:     []byte("resource-pack-data"),
		PackType: packet.ResourcePackTypeResources,
	}})
	if err != nil {
		t.Fatalf("Normalize() returned error: %v", err)
	}

	handler, err := NewMCPEHandler(MCPEOptions{
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		ServerName:    "MCPE Resource Pack Test",
		ServerBrand:   "BetterAltay-Go",
		GameMode:      "Survival",
		MaxPlayers:    5,
		ViewDistance:  1,
		World:         world,
		ResourcePacks: packs,
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
	if err := client.HandlePacket(ctx, offlineLoginPacket(t, newClientKey(t))); err != nil {
		t.Fatalf("HandlePacket(Login) returned error: %v", err)
	}
	if err := client.HandlePacket(ctx, &packet.ClientToServerHandshake{}); err != nil {
		t.Fatalf("HandlePacket(ClientToServerHandshake) returned error: %v", err)
	}

	info := packetAt[*packet.ResourcePacksInfo](t, conn.packets, 2)
	if len(info.TexturePacks) != 1 || info.TexturePacks[0].UUID != packUUID || info.TexturePacks[0].Version != "1.0.0" {
		t.Fatalf("ResourcePacksInfo = %+v, want configured texture pack", info)
	}

	if err := client.HandlePacket(ctx, &packet.ResourcePackClientResponse{
		Response:        packet.PackResponseSendPacks,
		PacksToDownload: []string{packUUID.String() + "_1.0.0"},
	}); err != nil {
		t.Fatalf("HandlePacket(ResourcePackClientResponse SendPacks) returned error: %v", err)
	}
	dataInfo := packetAt[*packet.ResourcePackDataInfo](t, conn.packets, 3)
	if dataInfo.UUID != packUUID.String() || dataInfo.ChunkCount != 1 || dataInfo.Size != uint64(len(packs[0].Data)) {
		t.Fatalf("ResourcePackDataInfo = %+v, want single-chunk pack info", dataInfo)
	}

	if err := client.HandlePacket(ctx, &packet.ResourcePackChunkRequest{UUID: packUUID.String(), ChunkIndex: 0}); err != nil {
		t.Fatalf("HandlePacket(ResourcePackChunkRequest) returned error: %v", err)
	}
	chunk := packetAt[*packet.ResourcePackChunkData](t, conn.packets, 4)
	if chunk.UUID != packUUID.String() || chunk.ChunkIndex != 0 || string(chunk.Data) != "resource-pack-data" {
		t.Fatalf("ResourcePackChunkData = %+v, want exact pack payload", chunk)
	}

	if err := client.HandlePacket(ctx, &packet.ResourcePackClientResponse{Response: packet.PackResponseAllPacksDownloaded}); err != nil {
		t.Fatalf("HandlePacket(ResourcePackClientResponse AllPacksDownloaded) returned error: %v", err)
	}
	stack := packetAt[*packet.ResourcePackStack](t, conn.packets, 5)
	if len(stack.TexturePacks) != 1 || stack.TexturePacks[0].UUID != packUUID.String() {
		t.Fatalf("ResourcePackStack = %+v, want single configured pack", stack)
	}

	if err := client.HandlePacket(ctx, &packet.ResourcePackClientResponse{Response: packet.PackResponseCompleted}); err != nil {
		t.Fatalf("HandlePacket(ResourcePackClientResponse Completed) returned error: %v", err)
	}
	packetAt[*packet.JigsawStructureData](t, conn.packets, 6)
	packetAt[*packet.VoxelShapes](t, conn.packets, 7)
	packetAt[*packet.StartGame](t, conn.packets, 8)
	if client.state != stateAwaitChunkRadius {
		t.Fatalf("client state = %d, want stateAwaitChunkRadius after resource pack flow", client.state)
	}
}
