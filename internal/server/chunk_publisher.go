package server

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net"

	dfchunk "github.com/df-mc/dragonfly/server/world/chunk"
	gtprotocol "github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"

	appworld "gucooing/bds/internal/world"
)

type packetWriter interface {
	WritePacket(packet.Packet) error
	Flush() error
	RemoteAddr() net.Addr
}

type chunkPublisher struct {
	source appworld.ChunkProvider
	radius int32
	logger *slog.Logger
}

const initialWorldTime int32 = 0

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
	if err := conn.WritePacket(&packet.SetTime{Time: initialWorldTime}); err != nil {
		return fmt.Errorf("send set time: %w", err)
	}
	if err := conn.WritePacket(setSpawnPositionPacket(publisher.source)); err != nil {
		return fmt.Errorf("send set spawn position: %w", err)
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

func (publisher chunkPublisher) SendSubChunks(ctx context.Context, conn packetWriter, request *packet.SubChunkRequest) error {
	pk, err := subChunkPacket(ctx, publisher.source, request)
	if err != nil {
		return err
	}
	if err := conn.WritePacket(pk); err != nil {
		return fmt.Errorf("send sub chunks: %w", err)
	}
	return nil
}

func (publisher chunkPublisher) SendBlockRuntimeID(conn packetWriter, position appworld.BlockPos, runtimeID uint32, layer uint32) error {
	if err := conn.WritePacket(updateBlockPacket(position, runtimeID, layer)); err != nil {
		return fmt.Errorf("send block update: %w", err)
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

func subChunkPacket(ctx context.Context, source appworld.ChunkProvider, request *packet.SubChunkRequest) (*packet.SubChunk, error) {
	entries := make([]gtprotocol.SubChunkEntry, 0, len(request.Offsets))
	dimension := source.Dimension()
	for _, offset := range request.Offsets {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		entry, err := subChunkEntry(ctx, source, dimension, request, offset)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return &packet.SubChunk{
		CacheEnabled:    false,
		Dimension:       request.Dimension,
		Position:        request.Position,
		SubChunkEntries: entries,
	}, nil
}

func subChunkEntry(ctx context.Context, source appworld.ChunkProvider, dimension appworld.Dimension, request *packet.SubChunkRequest, offset gtprotocol.SubChunkOffset) (gtprotocol.SubChunkEntry, error) {
	entry := gtprotocol.SubChunkEntry{Offset: offset}
	if request.Dimension != dimension.ID() {
		entry.Result = gtprotocol.SubChunkResultInvalidDimension
		return entry, nil
	}

	targetY := request.Position[1] + int32(offset[1])
	minSubY := int32(dimension.Range().Min() >> 4)
	maxSubY := int32(dimension.Range().Max() >> 4)
	if targetY < minSubY || targetY > maxSubY {
		entry.Result = gtprotocol.SubChunkResultIndexOutOfBounds
		return entry, nil
	}

	position := appworld.ChunkPos{
		X: request.Position[0] + int32(offset[0]),
		Z: request.Position[2] + int32(offset[2]),
	}
	chunk, err := source.Chunk(ctx, position)
	if err != nil {
		return entry, fmt.Errorf("generate chunk %d,%d for sub chunk: %w", position.X, position.Z, err)
	}

	index := int(targetY - minSubY)
	subChunks := chunk.Native().Sub()
	if index < 0 || index >= len(subChunks) {
		entry.Result = gtprotocol.SubChunkResultIndexOutOfBounds
		return entry, nil
	}
	if subChunks[index].Empty() {
		entry.Result = gtprotocol.SubChunkResultSuccessAllAir
		return entry, nil
	}
	encoded := dfchunk.Encode(chunk.Native(), dfchunk.NetworkEncoding)
	entry.Result = gtprotocol.SubChunkResultSuccess
	entry.RawPayload = encoded.SubChunks[index]
	return entry, nil
}

func updateBlockPacket(position appworld.BlockPos, runtimeID uint32, layer uint32) *packet.UpdateBlock {
	return &packet.UpdateBlock{
		Position:          blockPos(position),
		NewBlockRuntimeID: runtimeID,
		Flags:             packet.BlockUpdateNetwork,
		Layer:             layer,
	}
}

func setSpawnPositionPacket(source appworld.ChunkProvider) *packet.SetSpawnPosition {
	spawn := blockPos(source.SpawnBlock())
	return &packet.SetSpawnPosition{
		SpawnType:     packet.SpawnTypeWorld,
		Position:      spawn,
		Dimension:     source.Dimension().ID(),
		SpawnPosition: spawn,
	}
}

func blockPos(position appworld.BlockPos) gtprotocol.BlockPos {
	return gtprotocol.BlockPos{position.X, position.Y, position.Z}
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
