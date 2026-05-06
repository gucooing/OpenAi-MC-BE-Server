package server

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"log/slog"
	"net"

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

const (
	subChunkVersion byte = 9
	networkEncoding byte = 1
)

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
	raw := encodeBiomes(chunk)
	raw = append(raw, 0)

	position := chunk.Position()
	return &packet.LevelChunk{
		Position:        gtprotocol.ChunkPos{position.X, position.Z},
		Dimension:       chunk.Dimension().ID(),
		SubChunkCount:   gtprotocol.SubChunkRequestModeLimited,
		HighestSubChunk: chunk.HighestFilledSubChunk(),
		CacheEnabled:    false,
		RawPayload:      raw,
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
	minSubY := dimension.MinSubY()
	maxSubY := dimension.MaxSubY()
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
	if index < 0 || index >= chunk.SubChunkCount() {
		entry.Result = gtprotocol.SubChunkResultIndexOutOfBounds
		return entry, nil
	}
	heightMapType, heightMapData := subChunkHeightMap(chunk, index)
	if chunk.SubChunkEmpty(index) {
		entry.Result = gtprotocol.SubChunkResultSuccessAllAir
		entry.HeightMapType = heightMapType
		entry.HeightMapData = heightMapData
		entry.RenderHeightMapType = heightMapType
		entry.RenderHeightMapData = heightMapData
		return entry, nil
	}
	entry.Result = gtprotocol.SubChunkResultSuccess
	entry.RawPayload = encodeSubChunk(chunk, index)
	entry.HeightMapType = heightMapType
	entry.HeightMapData = heightMapData
	entry.RenderHeightMapType = heightMapType
	entry.RenderHeightMapData = heightMapData
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

func subChunkHeightMap(chunk *appworld.Chunk, index int) (byte, []int8) {
	data := make([]int8, 256)
	higher, lower := true, true
	for x := uint8(0); x < 16; x++ {
		for z := uint8(0); z < 16; z++ {
			y := chunk.HighestBlockY(x, z, 0)
			i := (uint16(z) << 4) | uint16(x)
			otherIndex := chunk.SubIndex(y)
			if otherIndex > index {
				data[i], lower = 16, false
			} else if otherIndex < index {
				data[i], higher = -1, false
			} else {
				data[i], lower, higher = int8(y-chunk.SubY(otherIndex)), false, false
			}
		}
	}
	if higher {
		return gtprotocol.HeightMapDataTooHigh, nil
	}
	if lower {
		return gtprotocol.HeightMapDataTooLow, nil
	}
	return gtprotocol.HeightMapDataHasData, data
}

func encodeSubChunk(chunk *appworld.Chunk, index int) []byte {
	buf := bytes.NewBuffer(make([]byte, 0, 1024))
	storageCount := chunk.SubChunkLayerCount(index)
	_ = buf.WriteByte(subChunkVersion)
	_ = buf.WriteByte(byte(storageCount))
	_ = buf.WriteByte(byte(int32(index) + chunk.MinSubY()))
	for layer := uint8(0); int(layer) < storageCount; layer++ {
		encodeBlockStorage(buf, chunk, index, layer)
	}
	return append([]byte(nil), buf.Bytes()...)
}

func encodeBlockStorage(buf *bytes.Buffer, chunk *appworld.Chunk, index int, layer uint8) {
	baseY := chunk.SubY(index)
	values := make([]uint32, 4096)
	for i := range values {
		x := uint8(i >> 8)
		z := uint8((i >> 4) & 15)
		y := int16(i & 15)
		values[i] = chunk.RuntimeID(x, baseY+y, z, layer)
	}
	encodePalettedStorage(buf, values)
}

func encodeBiomes(chunk *appworld.Chunk) []byte {
	buf := bytes.NewBuffer(make([]byte, 0, chunk.SubChunkCount()*4))
	for index := 0; index < chunk.SubChunkCount(); index++ {
		baseY := chunk.SubY(index)
		values := make([]uint32, 4096)
		for i := range values {
			x := uint8(i >> 8)
			z := uint8((i >> 4) & 15)
			y := int16(i & 15)
			values[i] = chunk.BiomeID(x, baseY+y, z)
		}
		encodePalettedStorage(buf, values)
	}
	return append([]byte(nil), buf.Bytes()...)
}

func encodePalettedStorage(buf *bytes.Buffer, values []uint32) {
	palette, indices := paletteIndices(values)
	bitsPerIndex := paletteBits(len(palette))
	_ = buf.WriteByte(byte(bitsPerIndex<<1) | networkEncoding)
	if bitsPerIndex != 0 {
		writePackedIndices(buf, indices, bitsPerIndex)
		_ = gtprotocol.WriteVarint32(buf, int32(len(palette)))
	}
	for _, value := range palette {
		_ = gtprotocol.WriteVarint32(buf, int32(value))
	}
}

func paletteIndices(values []uint32) ([]uint32, []uint16) {
	palette := make([]uint32, 0, 4)
	lookup := make(map[uint32]uint16, 4)
	indices := make([]uint16, len(values))
	for i, value := range values {
		index, ok := lookup[value]
		if !ok {
			index = uint16(len(palette))
			lookup[value] = index
			palette = append(palette, value)
		}
		indices[i] = index
	}
	return palette, indices
}

func paletteBits(valueCount int) uint8 {
	for _, bits := range []uint8{0, 1, 2, 3, 4, 5, 6, 8, 16} {
		if valueCount <= 1<<bits {
			return bits
		}
	}
	return 16
}

func writePackedIndices(buf *bytes.Buffer, indices []uint16, bitsPerIndex uint8) {
	indicesPerWord := 32 / int(bitsPerIndex)
	mask := uint32((1 << bitsPerIndex) - 1)
	words := make([]uint32, (len(indices)+indicesPerWord-1)/indicesPerWord)
	for i, index := range indices {
		wordIndex := i / indicesPerWord
		bitOffset := uint((i % indicesPerWord) * int(bitsPerIndex))
		words[wordIndex] |= (uint32(index) & mask) << bitOffset
	}
	for _, word := range words {
		_ = binary.Write(buf, binary.LittleEndian, word)
	}
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
