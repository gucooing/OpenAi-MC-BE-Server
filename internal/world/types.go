package world

import (
	"context"
	"fmt"

	"github.com/df-mc/dragonfly/server/block/cube"
	dfworld "github.com/df-mc/dragonfly/server/world"
	dfchunk "github.com/df-mc/dragonfly/server/world/chunk"
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

type Dimension struct {
	id         int32
	buildRange cube.Range
}

var Overworld = Dimension{
	id:         0,
	buildRange: dfworld.Overworld.Range(),
}

func (dimension Dimension) ID() int32 {
	return dimension.id
}

func (dimension Dimension) Range() cube.Range {
	if dimension.buildRange.Max() <= dimension.buildRange.Min() {
		return Overworld.buildRange
	}
	return dimension.buildRange
}

type ChunkProvider interface {
	Chunk(context.Context, ChunkPos) (*Chunk, error)
	Dimension() Dimension
	Spawn() Vec3
	SpawnBlock() BlockPos
}

type Chunk struct {
	position  ChunkPos
	dimension Dimension
	data      *dfchunk.Chunk
	registry  RuntimeRegistry
}

func NewChunk(position ChunkPos, dimension Dimension, registry RuntimeRegistry) (*Chunk, error) {
	air, err := registry.RuntimeID(AirBlock)
	if err != nil {
		return nil, err
	}
	return &Chunk{
		position:  position,
		dimension: dimension,
		data:      dfchunk.New(air, dimension.Range()),
		registry:  registry,
	}, nil
}

func (chunk *Chunk) Position() ChunkPos {
	return chunk.position
}

func (chunk *Chunk) Dimension() Dimension {
	return chunk.dimension
}

func (chunk *Chunk) Native() *dfchunk.Chunk {
	return chunk.data
}

func (chunk *Chunk) SetRuntimeID(x uint8, y int16, z uint8, layer uint8, runtimeID uint32) error {
	if y < int16(chunk.dimension.Range().Min()) || y > int16(chunk.dimension.Range().Max()) {
		return fmt.Errorf("block y %d outside dimension range %d..%d", y, chunk.dimension.Range().Min(), chunk.dimension.Range().Max())
	}
	chunk.data.SetBlock(x, y, z, layer, runtimeID)
	return nil
}

func (chunk *Chunk) SetBlock(x uint8, y int16, z uint8, layer uint8, state BlockState) error {
	runtimeID, err := chunk.registry.RuntimeID(state)
	if err != nil {
		return err
	}
	return chunk.SetRuntimeID(x, y, z, layer, runtimeID)
}

func (chunk *Chunk) SetBiomeID(x uint8, y int16, z uint8, biomeID uint32) error {
	if y < int16(chunk.dimension.Range().Min()) || y > int16(chunk.dimension.Range().Max()) {
		return fmt.Errorf("biome y %d outside dimension range %d..%d", y, chunk.dimension.Range().Min(), chunk.dimension.Range().Max())
	}
	chunk.data.SetBiome(x, y, z, biomeID)
	return nil
}
