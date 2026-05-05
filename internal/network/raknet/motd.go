package raknet

import (
	"strconv"
	"strings"
)

const editionMinecraftBedrock = "MCPE"

type PongInfo struct {
	MOTD             string
	ProtocolVersion  int
	MinecraftVersion string
	OnlinePlayers    int
	MaxPlayers       int
	ServerID         int64
	ServerName       string
	GameMode         string
}

func (info PongInfo) Data() []byte {
	fields := []string{
		editionMinecraftBedrock,
		escapePongField(info.MOTD),
		strconv.Itoa(info.ProtocolVersion),
		info.MinecraftVersion,
		strconv.Itoa(info.OnlinePlayers),
		strconv.Itoa(info.MaxPlayers),
		strconv.FormatInt(info.ServerID, 10),
		escapePongField(info.ServerName),
		escapePongField(info.GameMode),
	}
	return []byte(strings.Join(fields, ";") + ";")
}

func escapePongField(value string) string {
	return strings.TrimRight(strings.ReplaceAll(value, ";", `\;`), `\`)
}
