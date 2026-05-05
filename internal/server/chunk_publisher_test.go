package server

import (
	"context"
	"net"
	"testing"

	dfchunk "github.com/df-mc/dragonfly/server/world/chunk"
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

	air, err := appworld.DefaultRuntimeRegistry().RuntimeID(appworld.AirBlock)
	if err != nil {
		t.Fatalf("RuntimeID(air) returned error: %v", err)
	}
	decoded, err := dfchunk.NetworkDecode(air, pk.RawPayload, int(pk.SubChunkCount), world.Dimension().Range())
	if err != nil {
		t.Fatalf("NetworkDecode(spawn chunk) returned error: %v", err)
	}
	grass, err := appworld.DefaultRuntimeRegistry().RuntimeID(appworld.GrassBlock)
	if err != nil {
		t.Fatalf("RuntimeID(grass) returned error: %v", err)
	}
	if got := decoded.Block(0, 63, 0, 0); got != grass {
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
