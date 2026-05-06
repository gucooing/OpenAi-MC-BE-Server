package world

import (
	"context"
	"fmt"
)

type ChunkPos struct {
	X int32
	Z int32
}

type BlockPos struct {
	X int32
	Y int32
	Z int32
}

type Vec3 struct {
	X float32
	Y float32
	Z float32
}

type BuildRange struct {
	min int16
	max int16
}

func (buildRange BuildRange) Min() int {
	return int(buildRange.min)
}

func (buildRange BuildRange) Max() int {
	return int(buildRange.max)
}

func (buildRange BuildRange) Height() int {
	return int(buildRange.max-buildRange.min) + 1
}

type Dimension struct {
	id   int32
	minY int16
	maxY int16
}

var Overworld = Dimension{id: 0, minY: -64, maxY: 319}

func (dimension Dimension) ID() int32 {
	return dimension.withDefaults().id
}

func (dimension Dimension) Range() BuildRange {
	dimension = dimension.withDefaults()
	return BuildRange{min: dimension.minY, max: dimension.maxY}
}

func (dimension Dimension) MinY() int16 {
	return dimension.withDefaults().minY
}

func (dimension Dimension) MaxY() int16 {
	return dimension.withDefaults().maxY
}

func (dimension Dimension) MinSubY() int32 {
	return floorDiv16(int32(dimension.MinY()))
}

func (dimension Dimension) MaxSubY() int32 {
	return floorDiv16(int32(dimension.MaxY()))
}

func (dimension Dimension) SubChunkCount() int {
	return int(dimension.MaxSubY()-dimension.MinSubY()) + 1
}

func (dimension Dimension) withDefaults() Dimension {
	if dimension.maxY <= dimension.minY {
		return Overworld
	}
	return dimension
}

type ChunkProvider interface {
	Chunk(context.Context, ChunkPos) (*Chunk, error)
	Dimension() Dimension
	Spawn() Vec3
	SpawnBlock() BlockPos
}

type Chunk struct {
	position       ChunkPos
	dimension      Dimension
	subChunks      []subChunk
	biomeOverrides map[uint32]uint32
	defaultBiomeID uint32
	airRuntimeID   uint32
	registry       RuntimeRegistry
}

type subChunk struct {
	layers map[uint8]map[uint16]uint32
}

func NewChunk(position ChunkPos, dimension Dimension, registry RuntimeRegistry) (*Chunk, error) {
	dimension = dimension.withDefaults()
	air, err := registry.RuntimeID(AirBlock)
	if err != nil {
		return nil, err
	}
	return &Chunk{
		position:       position,
		dimension:      dimension,
		subChunks:      make([]subChunk, dimension.SubChunkCount()),
		biomeOverrides: map[uint32]uint32{},
		airRuntimeID:   air,
		registry:       registry,
	}, nil
}

func (chunk *Chunk) Position() ChunkPos {
	return chunk.position
}

func (chunk *Chunk) Dimension() Dimension {
	return chunk.dimension
}

func (chunk *Chunk) AirRuntimeID() uint32 {
	return chunk.airRuntimeID
}

func (chunk *Chunk) SetRuntimeID(x uint8, y int16, z uint8, layer uint8, runtimeID uint32) error {
	if err := chunk.checkBlockPosition(x, y, z); err != nil {
		return err
	}
	index := chunk.SubIndex(y)
	if runtimeID == chunk.airRuntimeID {
		chunk.deleteRuntimeID(index, x, y, z, layer)
		return nil
	}
	sub := &chunk.subChunks[index]
	if sub.layers == nil {
		sub.layers = map[uint8]map[uint16]uint32{}
	}
	blocks := sub.layers[layer]
	if blocks == nil {
		blocks = map[uint16]uint32{}
		sub.layers[layer] = blocks
	}
	blocks[blockOffset(x, y, z)] = runtimeID
	return nil
}

func (chunk *Chunk) SetBlock(x uint8, y int16, z uint8, layer uint8, state BlockState) error {
	runtimeID, err := chunk.registry.RuntimeID(state)
	if err != nil {
		return err
	}
	return chunk.SetRuntimeID(x, y, z, layer, runtimeID)
}

func (chunk *Chunk) RuntimeID(x uint8, y int16, z uint8, layer uint8) uint32 {
	if x > 15 || z > 15 || y < chunk.dimension.MinY() || y > chunk.dimension.MaxY() {
		return chunk.airRuntimeID
	}
	sub := chunk.subChunks[chunk.SubIndex(y)]
	blocks := sub.layers[layer]
	if len(blocks) == 0 {
		return chunk.airRuntimeID
	}
	if runtimeID, ok := blocks[blockOffset(x, y, z)]; ok {
		return runtimeID
	}
	return chunk.airRuntimeID
}

func (chunk *Chunk) FillBiomeID(biomeID uint32) {
	chunk.defaultBiomeID = biomeID
	clear(chunk.biomeOverrides)
}

func (chunk *Chunk) SetBiomeID(x uint8, y int16, z uint8, biomeID uint32) error {
	if err := chunk.checkBlockPosition(x, y, z); err != nil {
		return err
	}
	key := chunk.biomeOffset(x, y, z)
	if biomeID == chunk.defaultBiomeID {
		delete(chunk.biomeOverrides, key)
		return nil
	}
	chunk.biomeOverrides[key] = biomeID
	return nil
}

func (chunk *Chunk) BiomeID(x uint8, y int16, z uint8) uint32 {
	if x > 15 || z > 15 || y < chunk.dimension.MinY() || y > chunk.dimension.MaxY() {
		return chunk.defaultBiomeID
	}
	if biomeID, ok := chunk.biomeOverrides[chunk.biomeOffset(x, y, z)]; ok {
		return biomeID
	}
	return chunk.defaultBiomeID
}

func (chunk *Chunk) HighestFilledSubChunk() uint16 {
	for index := len(chunk.subChunks) - 1; index > 0; index-- {
		if !chunk.SubChunkEmpty(index) {
			return uint16(index)
		}
	}
	return 0
}

func (chunk *Chunk) SubChunkCount() int {
	return len(chunk.subChunks)
}

func (chunk *Chunk) MinSubY() int32 {
	return chunk.dimension.MinSubY()
}

func (chunk *Chunk) MaxSubY() int32 {
	return chunk.dimension.MaxSubY()
}

func (chunk *Chunk) SubIndex(y int16) int {
	return int(floorDiv16(int32(y)) - chunk.MinSubY())
}

func (chunk *Chunk) SubY(index int) int16 {
	return int16((chunk.MinSubY() + int32(index)) << 4)
}

func (chunk *Chunk) SubChunkEmpty(index int) bool {
	if index < 0 || index >= len(chunk.subChunks) {
		return true
	}
	return chunk.subChunks[index].empty()
}

func (chunk *Chunk) SubChunkLayerCount(index int) int {
	if index < 0 || index >= len(chunk.subChunks) {
		return 0
	}
	highest := -1
	for layer, blocks := range chunk.subChunks[index].layers {
		if len(blocks) != 0 && int(layer) > highest {
			highest = int(layer)
		}
	}
	return highest + 1
}

func (chunk *Chunk) HighestBlockY(x, z uint8, layer uint8) int16 {
	for index := len(chunk.subChunks) - 1; index >= 0; index-- {
		if chunk.SubChunkEmpty(index) {
			continue
		}
		baseY := chunk.SubY(index)
		for y := int16(15); y >= 0; y-- {
			if chunk.RuntimeID(x, baseY+y, z, layer) != chunk.airRuntimeID {
				return baseY + y
			}
		}
	}
	return chunk.dimension.MinY()
}

func (chunk *Chunk) deleteRuntimeID(index int, x uint8, y int16, z uint8, layer uint8) {
	sub := &chunk.subChunks[index]
	blocks := sub.layers[layer]
	if len(blocks) == 0 {
		return
	}
	delete(blocks, blockOffset(x, y, z))
	if len(blocks) == 0 {
		delete(sub.layers, layer)
	}
}

func (chunk *Chunk) checkBlockPosition(x uint8, y int16, z uint8) error {
	if x > 15 || z > 15 {
		return fmt.Errorf("block column %d,%d outside chunk range 0..15", x, z)
	}
	if y < chunk.dimension.MinY() || y > chunk.dimension.MaxY() {
		return fmt.Errorf("block y %d outside dimension range %d..%d", y, chunk.dimension.MinY(), chunk.dimension.MaxY())
	}
	return nil
}

func (chunk *Chunk) biomeOffset(x uint8, y int16, z uint8) uint32 {
	return uint32(chunk.SubIndex(y))<<12 | uint32(blockOffset(x, y, z))
}

func (sub subChunk) empty() bool {
	for _, blocks := range sub.layers {
		if len(blocks) != 0 {
			return false
		}
	}
	return true
}

func blockOffset(x uint8, y int16, z uint8) uint16 {
	return (uint16(x&15) << 8) | (uint16(z&15) << 4) | uint16(y&15)
}

func floorDiv16(value int32) int32 {
	if value >= 0 {
		return value >> 4
	}
	return -((-value + 15) >> 4)
}
