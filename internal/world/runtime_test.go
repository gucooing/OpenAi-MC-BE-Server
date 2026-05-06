package world

import "testing"

func TestRuntimeRegistryUsesBlockStateHashes(t *testing.T) {
	registry := DefaultRuntimeRegistry()
	tests := []struct {
		name  string
		state BlockState
		want  uint32
	}{
		{name: "air", state: AirBlock, want: 3690217760},
		{name: "bedrock", state: BedrockBlock, want: 4121722107},
		{name: "dirt", state: DirtBlock, want: 2186211206},
		{name: "grass_block", state: GrassBlock, want: 3727763636},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := registry.RuntimeID(tt.state)
			if err != nil {
				t.Fatalf("RuntimeID() returned error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("RuntimeID() = %d, want %d", got, tt.want)
			}
		})
	}
}
