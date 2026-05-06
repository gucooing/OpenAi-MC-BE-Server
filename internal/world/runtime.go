package world

import (
	"bytes"
	"fmt"
	"sort"

	"github.com/sandertv/gophertunnel/minecraft/nbt"
)

const (
	fnv1a32Init  uint32 = 0x811c9dc5
	fnv1a32Prime uint32 = 0x01000193
)

type BlockState struct {
	Name       string
	Properties map[string]any
}

var (
	AirBlock        = BlockState{Name: "minecraft:air"}
	BedrockBlock    = BlockState{Name: "minecraft:bedrock", Properties: map[string]any{"infiniburn_bit": byte(0)}}
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
	return BlockStateHash(state), nil
}

func BlockStateHash(state BlockState) uint32 {
	if state.Name == "minecraft:unknown" || state.Name == "unknown" {
		return ^uint32(1)
	}

	states := state.Properties
	if states == nil {
		states = map[string]any{}
	}
	keys := make([]string, 0, len(states))
	for key := range states {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	buf := bytes.NewBuffer(nil)
	buf.Write([]byte{10, 0, 0})
	buf.Write(marshalNBTField("name", state.Name))
	buf.Write([]byte{10, 6, 0})
	buf.WriteString("states")
	for _, key := range keys {
		buf.Write(marshalNBTField(key, states[key]))
	}
	buf.WriteByte(0)
	buf.WriteByte(0)

	hash := fnv1a32Init
	for _, value := range buf.Bytes() {
		hash ^= uint32(value)
		hash *= fnv1a32Prime
	}
	return hash
}

func marshalNBTField(key string, value any) []byte {
	buf := bytes.NewBuffer(nil)
	_ = nbt.NewEncoderWithEncoding(buf, nbt.LittleEndian).Encode(map[string]any{key: value})
	return buf.Bytes()[3 : buf.Len()-1]
}

func BiomeByName(name string) (Biome, error) {
	if name == "" {
		return Biome{}, fmt.Errorf("biome name cannot be empty")
	}
	switch name {
	case "plains", "minecraft:plains":
		return Biome{Name: "plains", ID: 1}, nil
	default:
		return Biome{}, fmt.Errorf("unknown biome %s", name)
	}
}
