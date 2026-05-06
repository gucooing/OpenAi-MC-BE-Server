package world

import (
	"context"
	"math"
)

type FlatGenerator struct {
	Registry      RuntimeRegistry
	DimensionData Dimension
	BaseY         int16
	Layers        []BlockState
	BiomeData     Biome
	SpawnPosition Vec3
}

func NewFlatGenerator() (FlatGenerator, error) {
	biome, err := BiomeByName("plains")
	if err != nil {
		return FlatGenerator{}, err
	}
	return FlatGenerator{
		Registry:      DefaultRuntimeRegistry(),
		DimensionData: Overworld,
		BaseY:         60,
		Layers: []BlockState{
			BedrockBlock,
			DirtBlock,
			DirtBlock,
			GrassBlock,
		},
		BiomeData:     biome,
		SpawnPosition: Vec3{X: 0.5, Y: 64, Z: 0.5},
	}, nil
}

func (generator FlatGenerator) Chunk(ctx context.Context, position ChunkPos) (*Chunk, error) {
	generator = generator.withDefaults()
	chunk, err := NewChunk(position, generator.DimensionData, generator.Registry)
	if err != nil {
		return nil, err
	}
	chunk.FillBiomeID(generator.BiomeData.ID)

	layers := make([]uint32, len(generator.Layers))
	for i, state := range generator.Layers {
		runtimeID, err := generator.Registry.RuntimeID(state)
		if err != nil {
			return nil, err
		}
		layers[i] = runtimeID
	}

	for x := uint8(0); x < 16; x++ {
		for z := uint8(0); z < 16; z++ {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}
			for layer, runtimeID := range layers {
				y := generator.BaseY + int16(layer)
				if err := chunk.SetRuntimeID(x, y, z, 0, runtimeID); err != nil {
					return nil, err
				}
			}
		}
	}
	return chunk, nil
}

func (generator FlatGenerator) Dimension() Dimension {
	return generator.withDefaults().DimensionData
}

func (generator FlatGenerator) Spawn() Vec3 {
	return generator.withDefaults().SpawnPosition
}

func (generator FlatGenerator) SpawnBlock() BlockPos {
	spawn := generator.Spawn()
	return BlockPos{
		X: int32(math.Floor(float64(spawn.X))),
		Y: int32(math.Floor(float64(spawn.Y))),
		Z: int32(math.Floor(float64(spawn.Z))),
	}
}

func (generator FlatGenerator) withDefaults() FlatGenerator {
	if generator.Registry == (RuntimeRegistry{}) {
		generator.Registry = DefaultRuntimeRegistry()
	}
	if generator.DimensionData.Range().Max() <= generator.DimensionData.Range().Min() {
		generator.DimensionData = Overworld
	}
	if len(generator.Layers) == 0 {
		generator.Layers = []BlockState{BedrockBlock, DirtBlock, DirtBlock, GrassBlock}
	}
	if generator.BiomeData.Name == "" {
		biome, err := BiomeByName("plains")
		if err == nil {
			generator.BiomeData = biome
		}
	}
	if generator.SpawnPosition == (Vec3{}) {
		generator.SpawnPosition = Vec3{X: 0.5, Y: float32(generator.BaseY + int16(len(generator.Layers))), Z: 0.5}
	}
	return generator
}
