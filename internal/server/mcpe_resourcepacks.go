package server

import (
	"context"
	"fmt"

	gtprotocol "github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

func (client *MCPEClient) sendResourcePackInfo() error {
	if err := client.conn.WritePacket(client.resourceDownloads.Info(client.handler.texturePacksRequired)); err != nil {
		return fmt.Errorf("send ResourcePacksInfo: %w", err)
	}
	return nil
}

func (client *MCPEClient) handleResourcePackClientResponse(ctx context.Context, pk *packet.ResourcePackClientResponse) error {
	switch pk.Response {
	case packet.PackResponseSendPacks:
		if len(pk.PacksToDownload) == 0 {
			return client.sendResourcePackStack()
		}
		packets, err := client.resourceDownloads.Begin(pk.PacksToDownload)
		if err != nil {
			return fmt.Errorf("select resource packs: %w", err)
		}
		return writeClientPackets(client.conn, packets...)
	case packet.PackResponseAllPacksDownloaded:
		return client.sendResourcePackStack()
	case packet.PackResponseCompleted:
		return client.finishResourcePackFlow(ctx)
	case packet.PackResponseRefused:
		return fmt.Errorf("client refused resource packs")
	default:
		return fmt.Errorf("unknown resource pack response %d", pk.Response)
	}
}

func (client *MCPEClient) handleResourcePacksReadyForValidation(ctx context.Context, _ *packet.ResourcePacksReadyForValidation) error {
	if !client.resourcePackStackSent {
		return nil
	}
	return client.finishResourcePackFlow(ctx)
}

func (client *MCPEClient) handleResourcePackChunkRequest(_ context.Context, pk *packet.ResourcePackChunkRequest) error {
	packets, err := client.resourceDownloads.HandleChunkRequest(pk)
	if err != nil {
		return fmt.Errorf("handle ResourcePackChunkRequest: %w", err)
	}
	return writeClientPackets(client.conn, packets...)
}

func (client *MCPEClient) sendResourcePackStack() error {
	if client.resourcePackStackSent {
		return nil
	}
	client.resourcePackStackSent = true
	if err := client.conn.WritePacket(client.resourceDownloads.Stack(client.handler.texturePacksRequired, gtprotocol.CurrentVersion)); err != nil {
		return fmt.Errorf("send ResourcePackStack: %w", err)
	}
	return nil
}

func (client *MCPEClient) finishResourcePackFlow(ctx context.Context) error {
	if client.state != stateAwaitResourcePackResponse {
		return nil
	}
	if len(client.handler.resourcePacks) != 0 && !client.resourcePackStackSent {
		if err := client.sendResourcePackStack(); err != nil {
			return err
		}
	}
	return client.startGame(ctx)
}

func writeClientPackets(conn MCPEConn, packets ...packet.Packet) error {
	for _, pk := range packets {
		if err := conn.WritePacket(pk); err != nil {
			return fmt.Errorf("send packet %T: %w", pk, err)
		}
	}
	return nil
}
