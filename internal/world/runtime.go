package world

import (
	"fmt"

	dfworld "github.com/df-mc/dragonfly/server/world"
	_ "github.com/df-mc/dragonfly/server/world/biome"
	dfchunk "github.com/df-mc/dragonfly/server/world/chunk"
)

type BlockState struct {
	Name       string
	Properties map[string]any
}

var (
	AirBlock        = BlockState{Name: "minecraft:air"}
	BedrockBlock    = BlockState{Name: "minecraft:bedrock"}
	DirtBlock       = BlockState{Name: "minecraft:dirt"}
	GrassBlock      = BlockState{Name: "minecraft:grass_block"}
	InfoUpdateBlock = BlockState{Name: "minecraft:info_update"}
)

type Biome struct {
	Name string
	ID   uint32
}

type RuntimeRegistry struct{}

func DefaultRuntimeRegistry() RuntimeRegistry {
	return RuntimeRegistry{}
}

func (RuntimeRegistry) RuntimeID(state BlockState) (uint32, error) {
	if state.Name == "" {
		return 0, fmt.Errorf("block state name cannot be empty")
	}
	if dfchunk.StateToRuntimeID == nil {
		return 0, fmt.Errorf("block runtime registry is not initialised")
	}
	runtimeID, ok := dfchunk.StateToRuntimeID(state.Name, state.Properties)
	if !ok {
		return 0, fmt.Errorf("unknown block state %s %#v", state.Name, state.Properties)
	}
	return runtimeID, nil
}

func BiomeByName(name string) (Biome, error) {
	if name == "" {
		return Biome{}, fmt.Errorf("biome name cannot be empty")
	}
	biome, ok := dfworld.BiomeByName(name)
	if !ok {
		return Biome{}, fmt.Errorf("unknown biome %s", name)
	}
	return Biome{Name: biome.String(), ID: uint32(biome.EncodeBiome())}, nil
}
