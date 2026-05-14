package resourcepack

import (
	"bytes"
	"testing"

	"github.com/google/uuid"
	gtprotocol "github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

func TestNormalizeFillsDefaultsAndBuildsPackets(t *testing.T) {
	packUUID := uuid.MustParse("d6e8a631-7783-4c5d-9f71-2ce9f1d7f111")
	packs, err := Normalize([]Pack{{
		UUID:     packUUID,
		Data:     []byte("abc"),
		PackType: packet.ResourcePackTypeResources,
	}})
	if err != nil {
		t.Fatalf("Normalize() returned error: %v", err)
	}
	if packs[0].Version != "1.0.0" {
		t.Fatalf("Version = %q, want default 1.0.0", packs[0].Version)
	}
	if packs[0].ContentIdentity != packUUID.String() {
		t.Fatalf("ContentIdentity = %q, want UUID string", packs[0].ContentIdentity)
	}

	queue := NewQueue(packs)
	info := queue.Info(true)
	if !info.TexturePackRequired || len(info.TexturePacks) != 1 {
		t.Fatalf("ResourcePacksInfo = %+v, want one required pack", info)
	}
	stack := queue.Stack(true, gtprotocol.CurrentVersion)
	if !stack.TexturePackRequired || len(stack.TexturePacks) != 1 || stack.TexturePacks[0].UUID != packUUID.String() {
		t.Fatalf("ResourcePackStack = %+v, want one pack entry", stack)
	}
}

func TestQueueDownloadsSelectedPacksSequentially(t *testing.T) {
	packOneUUID := uuid.MustParse("d6e8a631-7783-4c5d-9f71-2ce9f1d7f112")
	packTwoUUID := uuid.MustParse("d6e8a631-7783-4c5d-9f71-2ce9f1d7f113")
	packs, err := Normalize([]Pack{
		{
			UUID:     packOneUUID,
			Version:  "1.0.0",
			Data:     bytes.Repeat([]byte{0x01}, int(DefaultChunkSize)+1),
			PackType: packet.ResourcePackTypeResources,
		},
		{
			UUID:     packTwoUUID,
			Version:  "2.0.0",
			Data:     []byte{0x02, 0x03},
			PackType: packet.ResourcePackTypeResources,
		},
	})
	if err != nil {
		t.Fatalf("Normalize() returned error: %v", err)
	}
	queue := NewQueue(packs)

	packets, err := queue.Begin([]string{packOneUUID.String() + "_1.0.0", packTwoUUID.String() + "_2.0.0"})
	if err != nil {
		t.Fatalf("Begin() returned error: %v", err)
	}
	if len(packets) != 1 {
		t.Fatalf("Begin() packet count = %d, want 1", len(packets))
	}
	firstInfo, ok := packets[0].(*packet.ResourcePackDataInfo)
	if !ok {
		t.Fatalf("Begin() packet = %T, want *ResourcePackDataInfo", packets[0])
	}
	if firstInfo.UUID != packOneUUID.String() || firstInfo.ChunkCount != 2 {
		t.Fatalf("first ResourcePackDataInfo = %+v, want pack one with two chunks", firstInfo)
	}

	packets, err = queue.HandleChunkRequest(&packet.ResourcePackChunkRequest{UUID: packOneUUID.String(), ChunkIndex: 0})
	if err != nil {
		t.Fatalf("HandleChunkRequest(0) returned error: %v", err)
	}
	if len(packets) != 1 {
		t.Fatalf("HandleChunkRequest(0) packet count = %d, want 1", len(packets))
	}
	chunk, ok := packets[0].(*packet.ResourcePackChunkData)
	if !ok || chunk.ChunkIndex != 0 || len(chunk.Data) != int(DefaultChunkSize) {
		t.Fatalf("first chunk = %+v, want full-sized chunk", chunk)
	}

	packets, err = queue.HandleChunkRequest(&packet.ResourcePackChunkRequest{UUID: packOneUUID.String(), ChunkIndex: 1})
	if err != nil {
		t.Fatalf("HandleChunkRequest(1) returned error: %v", err)
	}
	if len(packets) != 2 {
		t.Fatalf("HandleChunkRequest(1) packet count = %d, want 2", len(packets))
	}
	if _, ok := packets[0].(*packet.ResourcePackChunkData); !ok {
		t.Fatalf("packet[0] = %T, want *ResourcePackChunkData", packets[0])
	}
	nextInfo, ok := packets[1].(*packet.ResourcePackDataInfo)
	if !ok || nextInfo.UUID != packTwoUUID.String() {
		t.Fatalf("next packet = %+v, want second pack info", nextInfo)
	}

	packets, err = queue.HandleChunkRequest(&packet.ResourcePackChunkRequest{UUID: packTwoUUID.String(), ChunkIndex: 0})
	if err != nil {
		t.Fatalf("HandleChunkRequest(second pack) returned error: %v", err)
	}
	if len(packets) != 1 {
		t.Fatalf("second pack chunk packet count = %d, want 1", len(packets))
	}
	if chunk, ok := packets[0].(*packet.ResourcePackChunkData); !ok || chunk.ChunkIndex != 0 || len(chunk.Data) != 2 {
		t.Fatalf("second pack chunk = %+v, want 2-byte payload", chunk)
	}
}
