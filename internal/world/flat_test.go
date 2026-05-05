package world

import (
	"context"
	"testing"
)

func TestFlatGeneratorCreatesSpawnGround(t *testing.T) {
	generator, err := NewFlatGenerator()
	if err != nil {
		t.Fatalf("NewFlatGenerator() returned error: %v", err)
	}
	chunk, err := generator.Chunk(context.Background(), ChunkPos{})
	if err != nil {
		t.Fatalf("Chunk() returned error: %v", err)
	}

	registry := DefaultRuntimeRegistry()
	grass, err := registry.RuntimeID(GrassBlock)
	if err != nil {
		t.Fatalf("RuntimeID(grass) returned error: %v", err)
	}
	if got := chunk.Native().Block(0, 63, 0, 0); got != grass {
		t.Fatalf("spawn surface runtime ID = %d, want %d", got, grass)
	}
	if got := chunk.Native().Biome(0, 63, 0); got != generator.BiomeData.ID {
		t.Fatalf("spawn biome = %d, want %d", got, generator.BiomeData.ID)
	}
	if got := generator.SpawnBlock(); got != (BlockPos{X: 0, Y: 64, Z: 0}) {
		t.Fatalf("SpawnBlock() = %#v, want 0,64,0", got)
	}
}
