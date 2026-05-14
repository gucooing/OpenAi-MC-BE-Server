package resourcepack

import (
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/google/uuid"
	gtprotocol "github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

const DefaultChunkSize uint32 = 1024 * 128

type Pack struct {
	UUID            uuid.UUID
	Version         string
	Data            []byte
	PackType        byte
	ContentKey      string
	SubPackName     string
	ContentIdentity string
	AddonPack       bool
	HasScripts      bool
	Premium         bool
	DownloadURL     string
}

func Normalize(packs []Pack) ([]Pack, error) {
	normalized := make([]Pack, len(packs))
	for i, pack := range packs {
		if pack.UUID == uuid.Nil {
			return nil, fmt.Errorf("resource pack %d has an empty UUID", i)
		}
		if pack.Version == "" {
			pack.Version = "1.0.0"
		}
		if pack.ContentIdentity == "" {
			pack.ContentIdentity = pack.UUID.String()
		}
		if pack.PackType == 0 {
			pack.PackType = packet.ResourcePackTypeResources
		}
		normalized[i] = pack
	}
	return normalized, nil
}

func (pack Pack) TexturePackInfo() gtprotocol.TexturePackInfo {
	contentIdentity := pack.ContentIdentity
	if contentIdentity == "" {
		contentIdentity = pack.UUID.String()
	}
	return gtprotocol.TexturePackInfo{
		UUID:            pack.UUID,
		Version:         pack.Version,
		Size:            uint64(len(pack.Data)),
		ContentKey:      pack.ContentKey,
		SubPackName:     pack.SubPackName,
		ContentIdentity: contentIdentity,
		HasScripts:      pack.HasScripts,
		AddonPack:       pack.AddonPack || pack.PackType == packet.ResourcePackTypeAddon || pack.PackType == packet.ResourcePackTypeBehaviour,
		RTXEnabled:      false,
		DownloadURL:     pack.DownloadURL,
	}
}

func (pack Pack) StackResourcePack() gtprotocol.StackResourcePack {
	return gtprotocol.StackResourcePack{
		UUID:        pack.UUID.String(),
		Version:     pack.Version,
		SubPackName: pack.SubPackName,
	}
}

func (pack Pack) DataInfo(chunkSize uint32) *packet.ResourcePackDataInfo {
	if chunkSize == 0 {
		chunkSize = DefaultChunkSize
	}
	size := uint64(len(pack.Data))
	chunkCount := uint32(0)
	if size != 0 {
		chunkCount = uint32((size + uint64(chunkSize) - 1) / uint64(chunkSize))
	}
	checksum := sha256.Sum256(pack.Data)
	return &packet.ResourcePackDataInfo{
		UUID:          pack.UUID.String(),
		DataChunkSize: chunkSize,
		ChunkCount:    chunkCount,
		Size:          size,
		Hash:          checksum[:],
		Premium:       pack.Premium,
		PackType:      pack.PackType,
	}
}

func (pack Pack) Chunk(chunkSize uint32, chunkIndex uint32) (*packet.ResourcePackChunkData, bool, error) {
	if chunkSize == 0 {
		chunkSize = DefaultChunkSize
	}
	offset := uint64(chunkIndex) * uint64(chunkSize)
	size := uint64(len(pack.Data))
	if offset > size {
		return nil, false, fmt.Errorf("resource pack %s chunk %d out of range", pack.UUID, chunkIndex)
	}
	end := offset + uint64(chunkSize)
	if end > size {
		end = size
	}
	payload := make([]byte, end-offset)
	copy(payload, pack.Data[offset:end])
	last := end >= size
	return &packet.ResourcePackChunkData{
		UUID:       pack.UUID.String(),
		ChunkIndex: chunkIndex,
		DataOffset: offset,
		Data:       payload,
	}, last, nil
}

type Queue struct {
	packs        []Pack
	selected     []int
	current      int
	currentChunk uint32
	chunkSize    uint32
}

func NewQueue(packs []Pack) Queue {
	return Queue{
		packs:     append([]Pack(nil), packs...),
		chunkSize: DefaultChunkSize,
	}
}

func (queue Queue) Info(texturePackRequired bool) *packet.ResourcePacksInfo {
	info := make([]gtprotocol.TexturePackInfo, 0, len(queue.packs))
	var hasAddons bool
	var hasScripts bool
	for _, pack := range queue.packs {
		packInfo := pack.TexturePackInfo()
		info = append(info, packInfo)
		hasAddons = hasAddons || packInfo.AddonPack
		hasScripts = hasScripts || packInfo.HasScripts
	}
	return &packet.ResourcePacksInfo{
		TexturePackRequired: texturePackRequired,
		HasAddons:           hasAddons,
		HasScripts:          hasScripts,
		TexturePacks:        info,
	}
}

func (queue Queue) Stack(texturePackRequired bool, baseGameVersion string) *packet.ResourcePackStack {
	texturePacks := make([]gtprotocol.StackResourcePack, 0, len(queue.packs))
	for _, pack := range queue.packs {
		texturePacks = append(texturePacks, pack.StackResourcePack())
	}
	return &packet.ResourcePackStack{
		TexturePackRequired: texturePackRequired,
		TexturePacks:        texturePacks,
		BaseGameVersion:     baseGameVersion,
	}
}

func (queue *Queue) Begin(requests []string) ([]packet.Packet, error) {
	queue.selected = queue.selected[:0]
	queue.current = 0
	queue.currentChunk = 0
	if len(requests) == 0 {
		return nil, nil
	}

	seen := make(map[int]struct{}, len(requests))
	for _, request := range requests {
		index, ok := queue.findPack(request)
		if !ok {
			return nil, fmt.Errorf("resource pack %q not found", request)
		}
		if _, exists := seen[index]; exists {
			continue
		}
		seen[index] = struct{}{}
		queue.selected = append(queue.selected, index)
	}

	if len(queue.selected) == 0 {
		return nil, nil
	}
	return []packet.Packet{queue.currentPack().DataInfo(queue.chunkSize)}, nil
}

func (queue *Queue) HandleChunkRequest(request *packet.ResourcePackChunkRequest) ([]packet.Packet, error) {
	if len(queue.selected) == 0 {
		return nil, fmt.Errorf("resource pack chunk request received before download started")
	}
	if queue.current >= len(queue.selected) {
		return nil, fmt.Errorf("all selected resource packs have already been downloaded")
	}
	pack := queue.currentPack()
	if request.UUID != pack.UUID.String() {
		return nil, fmt.Errorf("resource pack chunk request uuid mismatch: expected %s, got %s", pack.UUID, request.UUID)
	}
	if request.ChunkIndex != queue.currentChunk {
		return nil, fmt.Errorf("resource pack chunk request index mismatch: expected %d, got %d", queue.currentChunk, request.ChunkIndex)
	}

	chunk, last, err := pack.Chunk(queue.chunkSize, request.ChunkIndex)
	if err != nil {
		return nil, err
	}

	packets := []packet.Packet{chunk}
	queue.currentChunk++
	if last {
		queue.current++
		queue.currentChunk = 0
		if queue.current < len(queue.selected) {
			packets = append(packets, queue.currentPack().DataInfo(queue.chunkSize))
		}
	}
	return packets, nil
}

func (queue Queue) currentPack() *Pack {
	if len(queue.selected) == 0 || queue.current >= len(queue.selected) {
		return nil
	}
	return &queue.packs[queue.selected[queue.current]]
}

func (queue Queue) findPack(request string) (int, bool) {
	id := request
	version := ""
	if before, after, ok := strings.Cut(request, "_"); ok {
		id = before
		version = after
	}
	for index, pack := range queue.packs {
		if pack.UUID.String() != id {
			continue
		}
		if version != "" && pack.Version != version {
			continue
		}
		return index, true
	}
	return 0, false
}
