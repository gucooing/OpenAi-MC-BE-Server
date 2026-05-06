package server

import (
	"context"
	"encoding/base64"
	"fmt"
	"math"
	"sort"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/google/uuid"
	gtprotocol "github.com/sandertv/gophertunnel/minecraft/protocol"
	gtlogin "github.com/sandertv/gophertunnel/minecraft/protocol/login"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

const (
	playerCollisionWidth  float32 = 0.6
	playerCollisionHeight float32 = 1.8
	playerInventoryType   byte    = 0xff
)

type mcpePlayerState struct {
	uuid     uuid.UUID
	position mgl32.Vec3
	velocity mgl32.Vec3
	pitch    float32
	yaw      float32
	headYaw  float32
	onGround bool
	target   uint64
	metadata map[uint32]any
}

func (client *MCPEClient) initPlayerState() error {
	id, err := uuid.Parse(client.login.Identity.Identity)
	if err != nil {
		return fmt.Errorf("parse player identity UUID: %w", err)
	}
	spawn := client.handler.world.Spawn()
	client.player = mcpePlayerState{
		uuid:     id,
		position: mgl32.Vec3{spawn.X, spawn.Y, spawn.Z},
		metadata: playerMetadata(client.login.Identity.DisplayName),
	}
	return nil
}

func (handler *MCPEHandler) addPlayer(client *MCPEClient) []*MCPEClient {
	handler.playersMu.Lock()
	defer handler.playersMu.Unlock()

	players := make([]*MCPEClient, 0, len(handler.players))
	for _, other := range handler.players {
		if other != client {
			players = append(players, other)
		}
	}
	handler.players[client.runtimeID] = client
	sortClientsByRuntimeID(players)
	return players
}

func (handler *MCPEHandler) allPlayers() []*MCPEClient {
	handler.playersMu.RLock()
	defer handler.playersMu.RUnlock()

	players := make([]*MCPEClient, 0, len(handler.players))
	for _, player := range handler.players {
		players = append(players, player)
	}
	sortClientsByRuntimeID(players)
	return players
}

func (handler *MCPEHandler) spawnedPlayersExcept(client *MCPEClient) []*MCPEClient {
	handler.playersMu.RLock()
	defer handler.playersMu.RUnlock()

	players := make([]*MCPEClient, 0, len(handler.players))
	for _, other := range handler.players {
		if other != client && other.state == stateSpawned {
			players = append(players, other)
		}
	}
	sortClientsByRuntimeID(players)
	return players
}

func sortClientsByRuntimeID(players []*MCPEClient) {
	sort.Slice(players, func(i, j int) bool {
		return players[i].runtimeID < players[j].runtimeID
	})
}

func (client *MCPEClient) sendInitialPlayerSync(existing []*MCPEClient) error {
	if len(existing) > 0 {
		if err := writePacketToClients(existing, &packet.PlayerList{
			ActionType: packet.PlayerListActionAdd,
			Entries:    []gtprotocol.PlayerListEntry{client.playerListEntry()},
		}); err != nil {
			return fmt.Errorf("broadcast PlayerList add: %w", err)
		}
	}

	allPlayers := client.handler.allPlayers()
	entries := make([]gtprotocol.PlayerListEntry, 0, len(allPlayers))
	for _, player := range allPlayers {
		entries = append(entries, player.playerListEntry())
	}
	if err := client.conn.WritePacket(&packet.PlayerList{ActionType: packet.PlayerListActionAdd, Entries: entries}); err != nil {
		return fmt.Errorf("send PlayerList: %w", err)
	}
	if err := client.conn.WritePacket(client.setActorDataPacket(0)); err != nil {
		return fmt.Errorf("send SetActorData: %w", err)
	}
	if err := client.conn.WritePacket(client.setActorMotionPacket(0)); err != nil {
		return fmt.Errorf("send SetActorMotion: %w", err)
	}
	return nil
}

func (client *MCPEClient) spawnToInitialisedPlayers() error {
	players := client.handler.spawnedPlayersExcept(client)
	for _, other := range players {
		if err := writePlayerSpawn(client.conn, other); err != nil {
			return fmt.Errorf("send existing player spawn: %w", err)
		}
	}
	for _, other := range players {
		if err := writePlayerSpawn(other.conn, client); err != nil {
			return fmt.Errorf("broadcast player spawn: %w", err)
		}
	}
	return nil
}

func writePlayerSpawn(conn MCPEConn, player *MCPEClient) error {
	if err := conn.WritePacket(player.addPlayerPacket()); err != nil {
		return err
	}
	if err := conn.WritePacket(player.setActorDataPacket(0)); err != nil {
		return err
	}
	if err := conn.WritePacket(player.setActorMotionPacket(0)); err != nil {
		return err
	}
	return nil
}

func writePacketToClients(clients []*MCPEClient, pk packet.Packet) error {
	for _, client := range clients {
		if err := client.conn.WritePacket(pk); err != nil {
			return err
		}
	}
	return nil
}

func (client *MCPEClient) playerListEntry() gtprotocol.PlayerListEntry {
	return gtprotocol.PlayerListEntry{
		UUID:           client.player.uuid,
		EntityUniqueID: int64(client.runtimeID),
		Username:       client.login.Identity.DisplayName,
		XUID:           client.login.Identity.XUID,
		BuildPlatform:  int32(client.login.Client.DeviceOS),
		Skin:           skinFromLoginData(client.login.Client),
	}
}

func (client *MCPEClient) addPlayerPacket() *packet.AddPlayer {
	return &packet.AddPlayer{
		UUID:             client.player.uuid,
		Username:         client.login.Identity.DisplayName,
		EntityRuntimeID:  client.runtimeID,
		Position:         client.player.position,
		Velocity:         client.player.velocity,
		Pitch:            client.player.pitch,
		Yaw:              client.player.yaw,
		HeadYaw:          client.player.headYaw,
		GameType:         gameModeID(client.handler.gameMode),
		EntityMetadata:   cloneMetadata(client.player.metadata),
		AbilityData:      playerAbilityData(int64(client.runtimeID)),
		DeviceID:         string(client.login.Client.DeviceID),
		BuildPlatform:    int32(client.login.Client.DeviceOS),
		EntityProperties: gtprotocol.EntityProperties{},
	}
}

func (client *MCPEClient) movePlayerPacket(mode byte, tick uint64) *packet.MovePlayer {
	return &packet.MovePlayer{
		EntityRuntimeID: client.runtimeID,
		Position:        client.player.position,
		Pitch:           client.player.pitch,
		Yaw:             client.player.yaw,
		HeadYaw:         client.player.headYaw,
		Mode:            mode,
		OnGround:        client.player.onGround,
		Tick:            tick,
	}
}

func (client *MCPEClient) setActorDataPacket(tick uint64) *packet.SetActorData {
	return &packet.SetActorData{
		EntityRuntimeID:  client.runtimeID,
		EntityMetadata:   cloneMetadata(client.player.metadata),
		EntityProperties: gtprotocol.EntityProperties{},
		Tick:             tick,
	}
}

func (client *MCPEClient) setActorMotionPacket(tick uint64) *packet.SetActorMotion {
	return &packet.SetActorMotion{
		EntityRuntimeID: client.runtimeID,
		Velocity:        client.player.velocity,
		Tick:            tick,
	}
}

func (client *MCPEClient) handlePlayerAuthInput(_ context.Context, pk *packet.PlayerAuthInput) error {
	if client.runtimeID == 0 {
		return nil
	}
	if !finiteVec3(pk.Position) || !finiteFloat(pk.Pitch) || !finiteFloat(pk.Yaw) || !finiteFloat(pk.HeadYaw) {
		return fmt.Errorf("invalid PlayerAuthInput movement")
	}
	if client.state != stateSpawned {
		return client.conn.WritePacket(client.movePlayerPacket(packet.MoveModeReset, pk.Tick))
	}

	oldPosition, oldVelocity := client.player.position, client.player.velocity
	oldPitch, oldYaw, oldHeadYaw := client.player.pitch, client.player.yaw, client.player.headYaw

	client.player.position = pk.Position
	client.player.pitch = normaliseDegrees(pk.Pitch)
	client.player.yaw = normaliseDegrees(pk.Yaw)
	client.player.headYaw = normaliseDegrees(pk.HeadYaw)
	client.player.velocity = pk.Delta
	client.player.onGround = !inputFlag(pk.InputData, packet.InputFlagJumping)

	if client.updatePlayerMetadataFromInput(pk.InputData) {
		if err := client.broadcastToSpawnedPeers(client.setActorDataPacket(pk.Tick)); err != nil {
			return fmt.Errorf("broadcast SetActorData: %w", err)
		}
	}
	if oldPosition != client.player.position || oldPitch != client.player.pitch || oldYaw != client.player.yaw || oldHeadYaw != client.player.headYaw {
		if err := client.broadcastToSpawnedPeers(client.movePlayerPacket(packet.MoveModeNormal, pk.Tick)); err != nil {
			return fmt.Errorf("broadcast MovePlayer: %w", err)
		}
	}
	if oldVelocity != client.player.velocity {
		if err := client.broadcastToSpawnedPeers(client.setActorMotionPacket(pk.Tick)); err != nil {
			return fmt.Errorf("broadcast SetActorMotion: %w", err)
		}
	}
	return nil
}

func (client *MCPEClient) handleMovePlayer(_ context.Context, pk *packet.MovePlayer) error {
	if client.runtimeID == 0 {
		return nil
	}
	if pk.EntityRuntimeID != 0 && pk.EntityRuntimeID != client.runtimeID {
		return fmt.Errorf("MovePlayer runtime ID mismatch: expected %d, got %d", client.runtimeID, pk.EntityRuntimeID)
	}
	if !finiteVec3(pk.Position) || !finiteFloat(pk.Pitch) || !finiteFloat(pk.Yaw) || !finiteFloat(pk.HeadYaw) {
		return fmt.Errorf("invalid MovePlayer movement")
	}
	if client.state != stateSpawned {
		return client.conn.WritePacket(client.movePlayerPacket(packet.MoveModeReset, pk.Tick))
	}

	oldPosition := client.player.position
	client.player.position = pk.Position
	client.player.pitch = normaliseDegrees(pk.Pitch)
	client.player.yaw = normaliseDegrees(pk.Yaw)
	client.player.headYaw = normaliseDegrees(pk.HeadYaw)
	client.player.onGround = pk.OnGround
	client.player.velocity = pk.Position.Sub(oldPosition)

	if err := client.broadcastToSpawnedPeers(client.movePlayerPacket(pk.Mode, pk.Tick)); err != nil {
		return fmt.Errorf("broadcast MovePlayer: %w", err)
	}
	return nil
}

func (client *MCPEClient) handleInteract(_ context.Context, pk *packet.Interact) error {
	if client.runtimeID == 0 {
		return nil
	}
	switch pk.ActionType {
	case packet.InteractActionMouseOverEntity:
		return client.handleMouseOverEntity(pk)
	case packet.InteractActionOpenInventory:
		return client.handleOpenInventory()
	case packet.InteractActionLeaveVehicle:
		return client.handleLeaveVehicle(pk)
	default:
		return fmt.Errorf("unexpected Interact action %d", pk.ActionType)
	}
}

func (client *MCPEClient) handleMouseOverEntity(pk *packet.Interact) error {
	if pk.TargetEntityRuntimeID != 0 && !client.handler.playerExists(pk.TargetEntityRuntimeID) {
		return nil
	}
	if position, ok := pk.Position.Value(); ok && !finiteVec3(position) {
		return fmt.Errorf("invalid Interact mouse-over position")
	}
	client.player.target = pk.TargetEntityRuntimeID
	return nil
}

func (client *MCPEClient) handleOpenInventory() error {
	if client.state < stateAwaitInitialised || client.inventoryOpen {
		return nil
	}
	client.inventoryOpen = true
	return client.conn.WritePacket(&packet.ContainerOpen{
		WindowID:                0,
		ContainerType:           playerInventoryType,
		ContainerPosition:       gtprotocol.BlockPos{int32(client.player.position.X()), int32(client.player.position.Y()), int32(client.player.position.Z())},
		ContainerEntityUniqueID: -1,
	})
}

func (client *MCPEClient) handleContainerClose(_ context.Context, pk *packet.ContainerClose) error {
	switch pk.WindowID {
	case 0:
		client.inventoryOpen = false
		return client.conn.WritePacket(&packet.ContainerClose{WindowID: pk.WindowID})
	case 0xff:
		client.inventoryOpen = false
		return nil
	default:
		return fmt.Errorf("unexpected ContainerClose window=%d type=%d", pk.WindowID, pk.ContainerType)
	}
}

func (client *MCPEClient) handleLeaveVehicle(pk *packet.Interact) error {
	if position, ok := pk.Position.Value(); ok {
		if !finiteVec3(position) {
			return fmt.Errorf("invalid Interact leave-vehicle position")
		}
		client.player.position = position
	}
	return client.broadcastToSpawnedPeers(client.movePlayerPacket(packet.MoveModeNormal, 0))
}

func (client *MCPEClient) broadcastToSpawnedPeers(pk packet.Packet) error {
	return writePacketToClients(client.handler.spawnedPlayersExcept(client), pk)
}

func (handler *MCPEHandler) playerExists(runtimeID uint64) bool {
	handler.playersMu.RLock()
	defer handler.playersMu.RUnlock()
	_, ok := handler.players[runtimeID]
	return ok
}

func (client *MCPEClient) updatePlayerMetadataFromInput(input gtprotocol.Bitset) bool {
	changed := false
	changed = setGenericMetadataFlag(client.player.metadata, gtprotocol.EntityDataFlagSneaking, inputFlag(input, packet.InputFlagSneaking)) || changed
	changed = applyInputToggle(client.player.metadata, gtprotocol.EntityDataFlagSprinting, input, packet.InputFlagStartSprinting, packet.InputFlagStopSprinting) || changed
	changed = applyInputToggle(client.player.metadata, gtprotocol.EntityDataFlagSwimming, input, packet.InputFlagStartSwimming, packet.InputFlagStopSwimming) || changed
	changed = applyInputToggle(client.player.metadata, gtprotocol.EntityDataFlagGliding, input, packet.InputFlagStartGliding, packet.InputFlagStopGliding) || changed
	changed = applyInputToggle(client.player.metadata, gtprotocol.EntityDataFlagFly, input, packet.InputFlagStartFlying, packet.InputFlagStopFlying) || changed
	changed = applyInputToggle(client.player.metadata, gtprotocol.EntityDataFlagCrawling, input, packet.InputFlagStartCrawling, packet.InputFlagStopCrawling) || changed
	return changed
}

func applyInputToggle(metadata map[uint32]any, metadataFlag int, input gtprotocol.Bitset, startFlag, stopFlag int) bool {
	started := inputFlag(input, startFlag)
	stopped := inputFlag(input, stopFlag)
	if started == stopped {
		return false
	}
	return setGenericMetadataFlag(metadata, metadataFlag, started)
}

func inputFlag(input gtprotocol.Bitset, flag int) bool {
	if input.Len() <= flag {
		return false
	}
	return input.Load(flag)
}

func playerMetadata(name string) map[uint32]any {
	metadata := map[uint32]any(gtprotocol.NewEntityMetadata())
	metadata[gtprotocol.EntityDataKeyName] = name
	metadata[gtprotocol.EntityDataKeyScale] = float32(1)
	metadata[gtprotocol.EntityDataKeyWidth] = playerCollisionWidth
	metadata[gtprotocol.EntityDataKeyHeight] = playerCollisionHeight
	metadata[gtprotocol.EntityDataKeyAirSupply] = int16(300)
	metadata[gtprotocol.EntityDataKeyAirSupplyMax] = int16(400)
	_ = setGenericMetadataFlag(metadata, gtprotocol.EntityDataFlagShowName, true)
	_ = setGenericMetadataFlag(metadata, gtprotocol.EntityDataFlagAlwaysShowName, true)
	_ = setGenericMetadataFlag(metadata, gtprotocol.EntityDataFlagHasCollision, true)
	_ = setGenericMetadataFlag(metadata, gtprotocol.EntityDataFlagHasGravity, true)
	return metadata
}

func setGenericMetadataFlag(metadata map[uint32]any, flag int, value bool) bool {
	key := uint32(gtprotocol.EntityDataKeyFlags)
	index := uint(flag)
	if flag >= 64 {
		key = gtprotocol.EntityDataKeyFlagsTwo
		index = uint(flag - 64)
	}
	current, _ := metadata[key].(int64)
	mask := int64(1) << index
	next := current &^ mask
	if value {
		next = current | mask
	}
	if next == current {
		return false
	}
	metadata[key] = next
	return true
}

func playerAbilityData(runtimeID int64) gtprotocol.AbilityData {
	abilities := uint32(gtprotocol.AbilityBuild |
		gtprotocol.AbilityMine |
		gtprotocol.AbilityDoorsAndSwitches |
		gtprotocol.AbilityOpenContainers |
		gtprotocol.AbilityAttackPlayers |
		gtprotocol.AbilityAttackMobs |
		gtprotocol.AbilityWalkSpeed)
	return gtprotocol.AbilityData{
		EntityUniqueID:     runtimeID,
		PlayerPermissions:  byte(packet.PermissionLevelMember),
		CommandPermissions: gtprotocol.CommandPermissionLevelAny,
		Layers: []gtprotocol.AbilityLayer{{
			Type:             gtprotocol.AbilityLayerTypeBase,
			Abilities:        abilities,
			Values:           abilities,
			FlySpeed:         gtprotocol.AbilityBaseFlySpeed,
			VerticalFlySpeed: gtprotocol.AbilityBaseVerticalFlySpeed,
			WalkSpeed:        gtprotocol.AbilityBaseWalkSpeed,
		}},
	}
}

func skinFromLoginData(data gtlogin.ClientData) gtprotocol.Skin {
	skinData, _ := base64.StdEncoding.DecodeString(data.SkinData)
	capeData, _ := base64.StdEncoding.DecodeString(data.CapeData)
	resourcePatch, _ := base64.StdEncoding.DecodeString(data.SkinResourcePatch)
	geometry, _ := base64.StdEncoding.DecodeString(data.SkinGeometry)
	geometryVersion, _ := base64.StdEncoding.DecodeString(data.SkinGeometryVersion)
	animationData, _ := base64.StdEncoding.DecodeString(data.SkinAnimationData)

	animations := make([]gtprotocol.SkinAnimation, 0, len(data.AnimatedImageData))
	for _, animation := range data.AnimatedImageData {
		image, _ := base64.StdEncoding.DecodeString(animation.Image)
		animations = append(animations, gtprotocol.SkinAnimation{
			ImageWidth:     uint32(animation.ImageWidth),
			ImageHeight:    uint32(animation.ImageHeight),
			ImageData:      image,
			AnimationType:  uint32(animation.Type),
			FrameCount:     float32(animation.Frames),
			ExpressionType: uint32(animation.AnimationExpression),
		})
	}

	personaPieces := make([]gtprotocol.PersonaPiece, 0, len(data.PersonaPieces))
	for _, piece := range data.PersonaPieces {
		personaPieces = append(personaPieces, gtprotocol.PersonaPiece{
			PieceID:   piece.PieceID,
			PieceType: piece.PieceType,
			PackID:    piece.PackID,
			Default:   piece.Default,
			ProductID: piece.ProductID,
		})
	}

	tints := make([]gtprotocol.PersonaPieceTintColour, 0, len(data.PieceTintColours))
	for _, tint := range data.PieceTintColours {
		tints = append(tints, gtprotocol.PersonaPieceTintColour{
			PieceType: tint.PieceType,
			Colours:   append([]string(nil), tint.Colours[:]...),
		})
	}

	return gtprotocol.Skin{
		SkinID:                    data.SkinID,
		PlayFabID:                 data.PlayFabID,
		SkinResourcePatch:         resourcePatch,
		SkinImageWidth:            uint32(data.SkinImageWidth),
		SkinImageHeight:           uint32(data.SkinImageHeight),
		SkinData:                  skinData,
		Animations:                animations,
		CapeImageWidth:            uint32(data.CapeImageWidth),
		CapeImageHeight:           uint32(data.CapeImageHeight),
		CapeData:                  capeData,
		SkinGeometry:              geometry,
		AnimationData:             animationData,
		GeometryDataEngineVersion: geometryVersion,
		PremiumSkin:               data.PremiumSkin,
		PersonaSkin:               data.PersonaSkin,
		PersonaCapeOnClassicSkin:  data.CapeOnClassicSkin,
		PrimaryUser:               true,
		CapeID:                    data.CapeID,
		ArmSize:                   data.ArmSize,
		SkinColour:                data.SkinColour,
		PersonaPieces:             personaPieces,
		PieceTintColours:          tints,
		Trusted:                   data.TrustedSkin,
		OverrideAppearance:        data.OverrideSkin,
	}
}

func cloneMetadata(metadata map[uint32]any) map[uint32]any {
	cloned := make(map[uint32]any, len(metadata))
	for key, value := range metadata {
		cloned[key] = value
	}
	return cloned
}

func finiteVec3(value mgl32.Vec3) bool {
	return finiteFloat(value.X()) && finiteFloat(value.Y()) && finiteFloat(value.Z())
}

func finiteFloat(value float32) bool {
	return !math.IsNaN(float64(value)) && !math.IsInf(float64(value), 0)
}

func normaliseDegrees(value float32) float32 {
	value = float32(math.Mod(float64(value), 360))
	if value < 0 {
		value += 360
	}
	return value
}
