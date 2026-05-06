package bootstrap

import "fmt"

const (
	Name              = "BetterAltay-Go"
	SourceName        = "BetterAltay"
	SourceBaseVersion = "3.28.0"
	SourceForkVersion = "1.39.3"
	MinecraftVersion  = "1.26.20"
	ProtocolVersion   = 975
	BuildChannel      = "master"
)

type VersionInfo struct {
	Name              string
	SourceName        string
	SourceBaseVersion string
	SourceForkVersion string
	MinecraftVersion  string
	ProtocolVersion   int
	BuildChannel      string
}

var CurrentVersion = VersionInfo{
	Name:              Name,
	SourceName:        SourceName,
	SourceBaseVersion: SourceBaseVersion,
	SourceForkVersion: SourceForkVersion,
	MinecraftVersion:  MinecraftVersion,
	ProtocolVersion:   ProtocolVersion,
	BuildChannel:      BuildChannel,
}

func (info VersionInfo) String() string {
	return fmt.Sprintf(
		"%s protocol=%d minecraft=%s source=%s/%s fork=%s channel=%s",
		info.Name,
		info.ProtocolVersion,
		info.MinecraftVersion,
		info.SourceName,
		info.SourceBaseVersion,
		info.SourceForkVersion,
		info.BuildChannel,
	)
}
