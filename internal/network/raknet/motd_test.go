package raknet

import (
	"strings"
	"testing"
)

func TestPongInfoDataMatchesBetterAltayFormat(t *testing.T) {
	info := PongInfo{
		MOTD:             "Local Test",
		ProtocolVersion:  975,
		MinecraftVersion: "1.26.20",
		OnlinePlayers:    3,
		MaxPlayers:       20,
		ServerID:         42,
		ServerName:       "BetterAltay-Go",
		GameMode:         "Survival",
	}

	got := string(info.Data())
	want := "MCPE;Local Test;975;1.26.20;3;20;42;BetterAltay-Go;Survival;"
	if got != want {
		t.Fatalf("PongInfo.Data() = %q, want %q", got, want)
	}
}

func TestPongInfoEscapesSemicolons(t *testing.T) {
	info := PongInfo{
		MOTD:             `Name;`,
		ProtocolVersion:  975,
		MinecraftVersion: "1.26.20",
		ServerID:         42,
		ServerName:       `Engine;`,
		GameMode:         `Survival;`,
	}

	got := string(info.Data())
	for _, want := range []string{`Name\;`, `Engine\;`, `Survival\;`} {
		if !strings.Contains(got, want) {
			t.Fatalf("PongInfo.Data() = %q, want escaped field %q", got, want)
		}
	}
}
