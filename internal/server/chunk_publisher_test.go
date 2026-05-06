package server

import (
	"bytes"
	"context"
	"encoding/binary"
	"net"
	"testing"

	gtprotocol "github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"

	appworld "gucooing/bds/internal/world"
)

func TestChunkPublisherInitialWorldPackets(t *testing.T) {
	world, err := appworld.NewFlatGenerator()
	if err != nil {
		t.Fatalf("NewFlatGenerator() returned error: %v", err)
	}
	writer := &recordingPacketWriter{}
	if err := newChunkPublisher(world, 1, nil).SendInitial(context.Background(), writer); err != nil {
		t.Fatalf("SendInitial() returned error: %v", err)
	}

	if len(writer.packets) != 12 {
		t.Fatalf("SendInitial() wrote %d packets, want 12", len(writer.packets))
	}
	if _, ok := writer.packets[0].(*packet.NetworkChunkPublisherUpdate); !ok {
		t.Fatalf("first packet = %T, want NetworkChunkPublisherUpdate", writer.packets[0])
	}
	if pk, ok := writer.packets[1].(*packet.SetTime); !ok || pk.Time != initialWorldTime {
		t.Fatalf("second packet = %T %+v, want SetTime(%d)", writer.packets[1], writer.packets[1], initialWorldTime)
	}
	if pk, ok := writer.packets[2].(*packet.SetSpawnPosition); !ok || pk.SpawnPosition != (gtprotocol.BlockPos{0, 64, 0}) {
		t.Fatalf("third packet = %T %+v, want SetSpawnPosition at 0,64,0", writer.packets[2], writer.packets[2])
	}
	for i, pk := range writer.packets[3:] {
		if _, ok := pk.(*packet.LevelChunk); !ok {
			t.Fatalf("packet %d = %T, want LevelChunk", i+3, pk)
		}
	}
}

func TestLevelChunkPacketEncodesFlatSpawnChunk(t *testing.T) {
	world, err := appworld.NewFlatGenerator()
	if err != nil {
		t.Fatalf("NewFlatGenerator() returned error: %v", err)
	}
	chunk, err := world.Chunk(context.Background(), appworld.ChunkPos{X: 0, Z: 0})
	if err != nil {
		t.Fatalf("Chunk() returned error: %v", err)
	}
	pk, err := levelChunkPacket(chunk)
	if err != nil {
		t.Fatalf("levelChunkPacket() returned error: %v", err)
	}
	if pk.SubChunkCount != gtprotocol.SubChunkRequestModeLimited {
		t.Fatalf("SubChunkCount = %d, want limited sub-chunk request mode", pk.SubChunkCount)
	}
	if pk.HighestSubChunk != chunk.HighestFilledSubChunk() {
		t.Fatalf("HighestSubChunk = %d, want %d", pk.HighestSubChunk, chunk.HighestFilledSubChunk())
	}
	if len(pk.RawPayload) == 0 || pk.RawPayload[len(pk.RawPayload)-1] != 0 {
		t.Fatalf("limited LevelChunk payload should contain biome data followed by border block count")
	}
}

func TestSubChunkPacketEncodesFlatSpawnChunk(t *testing.T) {
	world, err := appworld.NewFlatGenerator()
	if err != nil {
		t.Fatalf("NewFlatGenerator() returned error: %v", err)
	}
	pk, err := subChunkPacket(context.Background(), world, &packet.SubChunkRequest{
		Dimension: world.Dimension().ID(),
		Position:  gtprotocol.SubChunkPos{0, 3, 0},
		Offsets:   []gtprotocol.SubChunkOffset{{0, 0, 0}},
	})
	if err != nil {
		t.Fatalf("subChunkPacket() returned error: %v", err)
	}
	entry := pk.SubChunkEntries[0]
	if entry.Result != gtprotocol.SubChunkResultSuccess {
		t.Fatalf("SubChunk result = %d, want success", entry.Result)
	}
	if entry.HeightMapType != gtprotocol.HeightMapDataHasData || len(entry.HeightMapData) != 256 {
		t.Fatalf("HeightMap = type %d len %d, want full height map", entry.HeightMapType, len(entry.HeightMapData))
	}
	if entry.RenderHeightMapType != entry.HeightMapType || len(entry.RenderHeightMapData) != len(entry.HeightMapData) {
		t.Fatalf("RenderHeightMap does not mirror HeightMap")
	}

	grass, err := appworld.DefaultRuntimeRegistry().RuntimeID(appworld.GrassBlock)
	if err != nil {
		t.Fatalf("RuntimeID(grass) returned error: %v", err)
	}
	decoded, err := decodeBlockLayer(entry.RawPayload, 0)
	if err != nil {
		t.Fatalf("decodeBlockLayer() returned error: %v", err)
	}
	if got := decoded[(0<<8)|(0<<4)|15]; got != grass {
		t.Fatalf("decoded spawn surface runtime ID = %d, want %d", got, grass)
	}
}

func TestSubChunkPacketRespondsToRequestedSubChunks(t *testing.T) {
	world, err := appworld.NewFlatGenerator()
	if err != nil {
		t.Fatalf("NewFlatGenerator() returned error: %v", err)
	}
	pk, err := subChunkPacket(context.Background(), world, &packet.SubChunkRequest{
		Dimension: world.Dimension().ID(),
		Position:  gtprotocol.SubChunkPos{0, 3, 0},
		Offsets: []gtprotocol.SubChunkOffset{
			{0, 0, 0},
			{0, 2, 0},
		},
	})
	if err != nil {
		t.Fatalf("subChunkPacket() returned error: %v", err)
	}
	if len(pk.SubChunkEntries) != 2 {
		t.Fatalf("SubChunk entries = %d, want 2", len(pk.SubChunkEntries))
	}
	if got := pk.SubChunkEntries[0].Result; got != gtprotocol.SubChunkResultSuccess {
		t.Fatalf("solid sub chunk result = %d, want success", got)
	}
	if len(pk.SubChunkEntries[0].RawPayload) == 0 {
		t.Fatalf("solid sub chunk payload is empty")
	}
	if got := pk.SubChunkEntries[1].Result; got != gtprotocol.SubChunkResultSuccessAllAir {
		t.Fatalf("air sub chunk result = %d, want success all air", got)
	}
	if got := pk.SubChunkEntries[1].HeightMapType; got != gtprotocol.HeightMapDataTooLow {
		t.Fatalf("air sub chunk height map type = %d, want too low", got)
	}
}

func TestUpdateBlockPacketUsesNetworkFlag(t *testing.T) {
	grass, err := appworld.DefaultRuntimeRegistry().RuntimeID(appworld.GrassBlock)
	if err != nil {
		t.Fatalf("RuntimeID(grass) returned error: %v", err)
	}
	pk := updateBlockPacket(appworld.BlockPos{X: 1, Y: 64, Z: -2}, grass, 0)
	if pk.Position != (gtprotocol.BlockPos{1, 64, -2}) || pk.NewBlockRuntimeID != grass || pk.Flags != packet.BlockUpdateNetwork || pk.Layer != 0 {
		t.Fatalf("UpdateBlock = %+v, want network block update", pk)
	}
}

func decodeBlockLayer(payload []byte, layer int) ([4096]uint32, error) {
	var blocks [4096]uint32
	buf := bytes.NewBuffer(payload)
	version, err := buf.ReadByte()
	if err != nil {
		return blocks, err
	}
	if version != subChunkVersion {
		return blocks, errUnexpected("sub-chunk version", version, subChunkVersion)
	}
	storageCount, err := buf.ReadByte()
	if err != nil {
		return blocks, err
	}
	if _, err := buf.ReadByte(); err != nil {
		return blocks, err
	}
	if layer >= int(storageCount) {
		return blocks, errUnexpected("storage count", byte(storageCount), byte(layer+1))
	}
	for storageLayer := 0; storageLayer <= layer; storageLayer++ {
		storage, err := decodePalettedStorage(buf)
		if err != nil {
			return blocks, err
		}
		if storageLayer == layer {
			return storage, nil
		}
	}
	return blocks, nil
}

func decodePalettedStorage(buf *bytes.Buffer) ([4096]uint32, error) {
	var blocks [4096]uint32
	header, err := buf.ReadByte()
	if err != nil {
		return blocks, err
	}
	bitsPerIndex := header >> 1
	var indices [4096]uint16
	if bitsPerIndex != 0 {
		indicesPerWord := 32 / int(bitsPerIndex)
		wordCount := (len(indices) + indicesPerWord - 1) / indicesPerWord
		mask := uint32((1 << bitsPerIndex) - 1)
		for wordIndex := 0; wordIndex < wordCount; wordIndex++ {
			var word uint32
			if err := binary.Read(buf, binary.LittleEndian, &word); err != nil {
				return blocks, err
			}
			for indexInWord := 0; indexInWord < indicesPerWord; indexInWord++ {
				blockIndex := wordIndex*indicesPerWord + indexInWord
				if blockIndex >= len(indices) {
					break
				}
				indices[blockIndex] = uint16((word >> uint(indexInWord*int(bitsPerIndex))) & mask)
			}
		}
	}
	paletteCount := int32(1)
	if bitsPerIndex != 0 {
		if err := gtprotocol.Varint32(buf, &paletteCount); err != nil {
			return blocks, err
		}
	}
	palette := make([]uint32, paletteCount)
	for i := range palette {
		var value int32
		if err := gtprotocol.Varint32(buf, &value); err != nil {
			return blocks, err
		}
		palette[i] = uint32(value)
	}
	for i, paletteIndex := range indices {
		blocks[i] = palette[paletteIndex]
	}
	return blocks, nil
}

func errUnexpected(name string, got, want byte) error {
	return &unexpectedByteError{name: name, got: got, want: want}
}

type unexpectedByteError struct {
	name      string
	got, want byte
}

func (err *unexpectedByteError) Error() string {
	return err.name + " mismatch"
}

type recordingPacketWriter struct {
	packets []packet.Packet
}

func (writer *recordingPacketWriter) WritePacket(pk packet.Packet) error {
	writer.packets = append(writer.packets, pk)
	return nil
}

func (writer *recordingPacketWriter) Flush() error {
	return nil
}

func (writer *recordingPacketWriter) RemoteAddr() net.Addr {
	return nil
}
