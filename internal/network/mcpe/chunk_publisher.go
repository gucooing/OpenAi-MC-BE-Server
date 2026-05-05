package mcpe

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net"

	dfchunk "github.com/df-mc/dragonfly/server/world/chunk"
	gtprotocol "github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"

	appprotocol "gucooing/bds/internal/protocol"
	appworld "gucooing/bds/internal/world"
)

type packetWriter interface {
	WritePacket(appprotocol.Packet) error
	Flush() error
	RemoteAddr() net.Addr
}

type chunkPublisher struct {
	source appworld.ChunkProvider
	radius int32
	logger *slog.Logger
}

func newChunkPublisher(source appworld.ChunkProvider, radius int32, logger *slog.Logger) chunkPublisher {
	if radius < 1 {
		radius = 1
	}
	return chunkPublisher{source: source, radius: radius, logger: logger}
}

func (publisher chunkPublisher) SendInitial(ctx context.Context, conn packetWriter) error {
	spawn := publisher.source.SpawnBlock()
	center := gtprotocol.BlockPos{spawn.X, spawn.Y, spawn.Z}
	if err := conn.WritePacket(&packet.NetworkChunkPublisherUpdate{
		Position: center,
		Radius:   uint32(publisher.radius << 4),
	}); err != nil {
		return fmt.Errorf("send network chunk publisher update: %w", err)
	}

	centerChunk := appworld.ChunkPos{X: spawn.X >> 4, Z: spawn.Z >> 4}
	for _, position := range chunkPositions(centerChunk, publisher.radius) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		chunk, err := publisher.source.Chunk(ctx, position)
		if err != nil {
			return fmt.Errorf("generate chunk %d,%d: %w", position.X, position.Z, err)
		}
		pk, err := levelChunkPacket(chunk)
		if err != nil {
			return fmt.Errorf("encode chunk %d,%d: %w", position.X, position.Z, err)
		}
		if err := conn.WritePacket(pk); err != nil {
			return fmt.Errorf("send chunk %d,%d: %w", position.X, position.Z, err)
		}
	}
	if err := conn.Flush(); err != nil {
		return fmt.Errorf("flush initial chunks: %w", err)
	}
	if publisher.logger != nil {
		count := (publisher.radius*2 + 1) * (publisher.radius*2 + 1)
		publisher.logger.Info("mcpe initial chunks sent", "remote", conn.RemoteAddr(), "chunks", count, "radius", publisher.radius)
	}
	return nil
}

func levelChunkPacket(chunk *appworld.Chunk) (*packet.LevelChunk, error) {
	encoded := dfchunk.Encode(chunk.Native(), dfchunk.NetworkEncoding)
	raw := bytes.NewBuffer(make([]byte, 0, encodedPayloadSize(encoded)))
	for _, subChunk := range encoded.SubChunks {
		_, _ = raw.Write(subChunk)
	}
	_, _ = raw.Write(encoded.Biomes)
	raw.WriteByte(0)

	position := chunk.Position()
	return &packet.LevelChunk{
		Position:      gtprotocol.ChunkPos{position.X, position.Z},
		Dimension:     chunk.Dimension().ID(),
		SubChunkCount: uint32(len(encoded.SubChunks)),
		CacheEnabled:  false,
		RawPayload:    raw.Bytes(),
	}, nil
}

func encodedPayloadSize(data dfchunk.SerialisedData) int {
	n := len(data.Biomes) + 1
	for _, subChunk := range data.SubChunks {
		n += len(subChunk)
	}
	return n
}

func chunkPositions(center appworld.ChunkPos, radius int32) []appworld.ChunkPos {
	positions := make([]appworld.ChunkPos, 0, (radius*2+1)*(radius*2+1))
	for distance := int32(0); distance <= radius; distance++ {
		for x := -distance; x <= distance; x++ {
			for z := -distance; z <= distance; z++ {
				if max(abs(x), abs(z)) != distance {
					continue
				}
				positions = append(positions, appworld.ChunkPos{X: center.X + x, Z: center.Z + z})
			}
		}
	}
	return positions
}

func abs(value int32) int32 {
	if value < 0 {
		return -value
	}
	return value
}
