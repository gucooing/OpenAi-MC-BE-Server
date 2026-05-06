package server

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"

	gtprotocol "github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

const (
	mcpeInventorySlots = 36
	mcpeHotbarSlots    = 9
	mcpeCursorSlot     = 0
	mcpeMaxStackCount  = 64
)

type mcpeInventory struct {
	main             [mcpeInventorySlots]gtprotocol.ItemInstance
	cursor           gtprotocol.ItemInstance
	offhand          gtprotocol.ItemInstance
	selectedHotbar   byte
	nextStackID      int32
	responseStackIDs map[int32]map[mcpeInventorySlot]int32
}

type mcpeInventorySlot struct {
	container byte
	slot      byte
}

type mcpeInventorySnapshot struct {
	main           [mcpeInventorySlots]gtprotocol.ItemInstance
	cursor         gtprotocol.ItemInstance
	offhand        gtprotocol.ItemInstance
	selectedHotbar byte
	nextStackID    int32
}

type itemStackRequestError struct {
	status uint8
	msg    string
}

func (e itemStackRequestError) Error() string {
	return e.msg
}

func newMCPEInventory() *mcpeInventory {
	return &mcpeInventory{
		nextStackID:      1,
		responseStackIDs: make(map[int32]map[mcpeInventorySlot]int32),
	}
}

func (client *MCPEClient) handleItemStackRequest(_ context.Context, pk *packet.ItemStackRequest) error {
	if client.runtimeID == 0 {
		return nil
	}
	return client.processItemStackRequests(pk.Requests)
}

func (client *MCPEClient) handleMobEquipment(_ context.Context, pk *packet.MobEquipment) error {
	if client.runtimeID == 0 {
		return nil
	}
	if pk.EntityRuntimeID != 0 && pk.EntityRuntimeID != client.runtimeID {
		return nil
	}
	switch pk.WindowID {
	case byte(gtprotocol.WindowIDOffHand):
		return nil
	case byte(gtprotocol.WindowIDInventory):
	default:
		return fmt.Errorf("unexpected MobEquipment window=%d", pk.WindowID)
	}
	slot := pk.InventorySlot
	if pk.HotBarSlot != slot {
		return client.resendHeldSlot()
	}
	if slot >= mcpeHotbarSlots {
		return client.resendHeldSlot()
	}

	ref := mcpeInventorySlot{container: gtprotocol.ContainerInventory, slot: slot}
	actual, _ := client.inventory.item(ref)
	if !sameItemInstance(actual, pk.NewItem) {
		if err := client.sendInventorySlot(ref); err != nil {
			return fmt.Errorf("resync selected inventory slot: %w", err)
		}
	}
	client.inventory.selectedHotbar = slot
	return client.broadcastHeldEquipment()
}

func (client *MCPEClient) sendInitialInventory() error {
	if client.inventory == nil {
		client.inventory = newMCPEInventory()
	}
	if err := client.conn.WritePacket(client.inventoryContentPacket(gtprotocol.WindowIDInventory)); err != nil {
		return fmt.Errorf("send InventoryContent inventory: %w", err)
	}
	if err := client.conn.WritePacket(client.inventoryContentPacket(gtprotocol.WindowIDOffHand)); err != nil {
		return fmt.Errorf("send InventoryContent offhand: %w", err)
	}
	if err := client.resendHeldSlot(); err != nil {
		return fmt.Errorf("send MobEquipment held slot: %w", err)
	}
	return nil
}

func (client *MCPEClient) inventoryContentPacket(windowID uint32) *packet.InventoryContent {
	switch windowID {
	case gtprotocol.WindowIDOffHand:
		return &packet.InventoryContent{
			WindowID: windowID,
			Content:  []gtprotocol.ItemInstance{client.inventory.offhand},
			Container: gtprotocol.FullContainerName{
				ContainerID: gtprotocol.ContainerOffhand,
			},
		}
	default:
		return &packet.InventoryContent{
			WindowID:  gtprotocol.WindowIDInventory,
			Content:   client.inventory.mainContent(),
			Container: gtprotocol.FullContainerName{ContainerID: gtprotocol.ContainerInventory},
		}
	}
}

func (client *MCPEClient) sendInventorySlot(ref mcpeInventorySlot) error {
	item, err := client.inventory.item(ref)
	if err != nil {
		return err
	}
	return client.conn.WritePacket(&packet.InventorySlot{
		WindowID: windowIDForInventorySlot(ref),
		Slot:     uint32(ref.slot),
		Container: gtprotocol.Option(gtprotocol.FullContainerName{
			ContainerID: ref.container,
		}),
		NewItem: item,
	})
}

func (client *MCPEClient) resendHeldSlot() error {
	return client.conn.WritePacket(client.heldEquipmentPacket(client.runtimeID))
}

func (client *MCPEClient) broadcastHeldEquipment() error {
	return client.broadcastToSpawnedPeers(client.heldEquipmentPacket(client.runtimeID))
}

func (client *MCPEClient) heldEquipmentPacket(runtimeID uint64) *packet.MobEquipment {
	slot := client.inventory.selectedHotbar
	return &packet.MobEquipment{
		EntityRuntimeID: runtimeID,
		NewItem:         client.inventory.heldItem(),
		InventorySlot:   slot,
		HotBarSlot:      slot,
		WindowID:        byte(gtprotocol.WindowIDInventory),
	}
}

func (client *MCPEClient) processItemStackRequests(requests []gtprotocol.ItemStackRequest) error {
	if len(requests) == 0 {
		return nil
	}
	if client.inventory == nil {
		client.inventory = newMCPEInventory()
	}

	beforeHeld := client.inventory.heldItem()
	responses := make([]gtprotocol.ItemStackResponse, 0, len(requests))
	rejected := false
	for _, request := range requests {
		response, ok := client.applyItemStackRequest(request)
		responses = append(responses, response)
		rejected = rejected || !ok
	}
	if err := client.conn.WritePacket(&packet.ItemStackResponse{Responses: responses}); err != nil {
		return fmt.Errorf("send ItemStackResponse: %w", err)
	}
	if !sameItemInstance(beforeHeld, client.inventory.heldItem()) {
		if err := client.broadcastHeldEquipment(); err != nil {
			return fmt.Errorf("broadcast held equipment: %w", err)
		}
	}
	if rejected {
		return client.resyncInventory()
	}
	return nil
}

func (client *MCPEClient) applyItemStackRequest(request gtprotocol.ItemStackRequest) (gtprotocol.ItemStackResponse, bool) {
	snapshot := client.inventory.snapshot()
	changes := make(map[mcpeInventorySlot]gtprotocol.ItemInstance)
	for _, action := range request.Actions {
		if err := client.applyStackRequestAction(action, changes); err != nil {
			client.inventory.restore(snapshot)
			return gtprotocol.ItemStackResponse{
				Status:    stackRequestStatus(err),
				RequestID: request.RequestID,
			}, false
		}
	}
	client.inventory.rememberResponseStackIDs(request.RequestID, changes)
	return gtprotocol.ItemStackResponse{
		Status:        gtprotocol.ItemStackResponseStatusOK,
		RequestID:     request.RequestID,
		ContainerInfo: stackResponseContainerInfo(changes),
	}, true
}

func (client *MCPEClient) applyStackRequestAction(action gtprotocol.StackRequestAction, changes map[mcpeInventorySlot]gtprotocol.ItemInstance) error {
	switch a := action.(type) {
	case *gtprotocol.TakeStackRequestAction:
		return client.transferStack(a.Source, a.Destination, a.Count, changes)
	case *gtprotocol.PlaceStackRequestAction:
		return client.transferStack(a.Source, a.Destination, a.Count, changes)
	case *gtprotocol.PlaceInContainerStackRequestAction:
		return client.transferStack(a.Source, a.Destination, a.Count, changes)
	case *gtprotocol.TakeOutContainerStackRequestAction:
		return client.transferStack(a.Source, a.Destination, a.Count, changes)
	case *gtprotocol.SwapStackRequestAction:
		return client.swapStacks(a.Source, a.Destination, changes)
	case *gtprotocol.DropStackRequestAction:
		return client.dropStack(a.Source, a.Count, changes)
	case *gtprotocol.DestroyStackRequestAction:
		if !client.creativeInventoryAllowed() {
			return stackRequestError(gtprotocol.ItemStackResponseStatusPlayerNotInCreativeMode, "cannot destroy items outside creative mode")
		}
		return client.destroyStack(a.Source, a.Count, changes)
	case *gtprotocol.MineBlockStackRequestAction:
		return client.ackMineBlockStack(a, changes)
	default:
		return stackRequestError(gtprotocol.ItemStackResponseStatusActionRequestNotAllowed, "unsupported stack request action %T", action)
	}
}

func (client *MCPEClient) transferStack(source, destination gtprotocol.StackRequestSlotInfo, count byte, changes map[mcpeInventorySlot]gtprotocol.ItemInstance) error {
	if count == 0 {
		return stackRequestError(gtprotocol.ItemStackResponseStatusInvalidTransferAmount, "transfer count must be positive")
	}
	sourceRef, sourceItem, err := client.verifyStackRequestSlot(source)
	if err != nil {
		return fmt.Errorf("source slot out of sync: %w", err)
	}
	destinationRef, destinationItem, err := client.verifyStackRequestSlot(destination)
	if err != nil {
		return fmt.Errorf("destination slot out of sync: %w", err)
	}
	if sourceRef == destinationRef {
		return stackRequestError(gtprotocol.ItemStackResponseStatusDstContainerAndSlotEqualToSrcContainerAndSlot, "source and destination slots are identical")
	}
	if itemIsEmpty(sourceItem) || sourceItem.Stack.Count < uint16(count) {
		return stackRequestError(gtprotocol.ItemStackResponseStatusInvalidTransferAmount, "source slot does not contain %d items", count)
	}

	sourceCount := sourceItem.Stack.Count
	if itemIsEmpty(destinationItem) {
		moved := sourceItem
		moved.Stack.Count = uint16(count)
		if uint16(count) != sourceCount {
			moved.StackNetworkID = client.inventory.nextNetworkStackID()
		}
		sourceItem.Stack.Count -= uint16(count)
		if sourceItem.Stack.Count == 0 {
			sourceItem = gtprotocol.ItemInstance{}
		}
		client.setRequestSlot(sourceRef, sourceItem, changes)
		client.setRequestSlot(destinationRef, moved, changes)
		return nil
	}

	if !sameStackType(sourceItem, destinationItem) {
		return stackRequestError(gtprotocol.ItemStackResponseStatusCannotPlaceItem, "cannot place incompatible item stacks")
	}
	if destinationItem.Stack.Count+uint16(count) > mcpeMaxStackCount {
		return stackRequestError(gtprotocol.ItemStackResponseStatusCannotPlaceItem, "destination stack would exceed %d items", mcpeMaxStackCount)
	}
	sourceItem.Stack.Count -= uint16(count)
	if sourceItem.Stack.Count == 0 {
		sourceItem = gtprotocol.ItemInstance{}
	}
	destinationItem.Stack.Count += uint16(count)
	client.setRequestSlot(sourceRef, sourceItem, changes)
	client.setRequestSlot(destinationRef, destinationItem, changes)
	return nil
}

func (client *MCPEClient) swapStacks(source, destination gtprotocol.StackRequestSlotInfo, changes map[mcpeInventorySlot]gtprotocol.ItemInstance) error {
	sourceRef, sourceItem, err := client.verifyStackRequestSlot(source)
	if err != nil {
		return fmt.Errorf("source slot out of sync: %w", err)
	}
	destinationRef, destinationItem, err := client.verifyStackRequestSlot(destination)
	if err != nil {
		return fmt.Errorf("destination slot out of sync: %w", err)
	}
	if sourceRef == destinationRef {
		return stackRequestError(gtprotocol.ItemStackResponseStatusDstContainerAndSlotEqualToSrcContainerAndSlot, "source and destination slots are identical")
	}
	client.setRequestSlot(sourceRef, destinationItem, changes)
	client.setRequestSlot(destinationRef, sourceItem, changes)
	return nil
}

func (client *MCPEClient) dropStack(source gtprotocol.StackRequestSlotInfo, count byte, changes map[mcpeInventorySlot]gtprotocol.ItemInstance) error {
	if count == 0 {
		return stackRequestError(gtprotocol.ItemStackResponseStatusInvalidTransferAmount, "drop count must be positive")
	}
	ref, item, err := client.verifyStackRequestSlot(source)
	if err != nil {
		return fmt.Errorf("source slot out of sync: %w", err)
	}
	if itemIsEmpty(item) || item.Stack.Count < uint16(count) {
		return stackRequestError(gtprotocol.ItemStackResponseStatusCannotDropItem, "source slot does not contain %d droppable items", count)
	}
	item.Stack.Count -= uint16(count)
	if item.Stack.Count == 0 {
		item = gtprotocol.ItemInstance{}
	}
	client.setRequestSlot(ref, item, changes)
	return nil
}

func (client *MCPEClient) destroyStack(source gtprotocol.StackRequestSlotInfo, count byte, changes map[mcpeInventorySlot]gtprotocol.ItemInstance) error {
	if count == 0 {
		return stackRequestError(gtprotocol.ItemStackResponseStatusInvalidTransferAmount, "destroy count must be positive")
	}
	ref, item, err := client.verifyStackRequestSlot(source)
	if err != nil {
		return fmt.Errorf("source slot out of sync: %w", err)
	}
	if itemIsEmpty(item) || item.Stack.Count < uint16(count) {
		return stackRequestError(gtprotocol.ItemStackResponseStatusCannotDestroyItem, "source slot does not contain %d destroyable items", count)
	}
	item.Stack.Count -= uint16(count)
	if item.Stack.Count == 0 {
		item = gtprotocol.ItemInstance{}
	}
	client.setRequestSlot(ref, item, changes)
	return nil
}

func (client *MCPEClient) ackMineBlockStack(action *gtprotocol.MineBlockStackRequestAction, changes map[mcpeInventorySlot]gtprotocol.ItemInstance) error {
	if action.HotbarSlot < 0 || action.HotbarSlot >= mcpeHotbarSlots {
		return stackRequestError(gtprotocol.ItemStackResponseStatusFailedToValidateSrcSlot, "mine block hotbar slot %d out of range", action.HotbarSlot)
	}
	slot := gtprotocol.StackRequestSlotInfo{
		Container:      gtprotocol.FullContainerName{ContainerID: gtprotocol.ContainerInventory},
		Slot:           byte(action.HotbarSlot),
		StackNetworkID: action.StackNetworkID,
	}
	ref, item, err := client.verifyStackRequestSlot(slot)
	if err != nil {
		return err
	}
	changes[ref] = item
	return nil
}

func (client *MCPEClient) verifyStackRequestSlot(slot gtprotocol.StackRequestSlotInfo) (mcpeInventorySlot, gtprotocol.ItemInstance, error) {
	ref, err := client.inventory.resolveSlot(slot.Container.ContainerID, slot.Slot)
	if err != nil {
		return ref, gtprotocol.ItemInstance{}, err
	}
	item, err := client.inventory.item(ref)
	if err != nil {
		return ref, gtprotocol.ItemInstance{}, err
	}
	expectedStackID, err := client.inventory.resolveStackNetworkID(slot.StackNetworkID, ref)
	if err != nil {
		return ref, item, err
	}
	if actual := itemStackNetworkID(item); actual != expectedStackID {
		return ref, item, stackRequestError(gtprotocol.ItemStackResponseStatusInvalidItemNetId, "stack network ID mismatch: client expected %d, server had %d", expectedStackID, actual)
	}
	return ref, item, nil
}

func (client *MCPEClient) setRequestSlot(ref mcpeInventorySlot, item gtprotocol.ItemInstance, changes map[mcpeInventorySlot]gtprotocol.ItemInstance) {
	_ = client.inventory.setItem(ref, item)
	actual, _ := client.inventory.item(ref)
	changes[ref] = actual
}

func (client *MCPEClient) resyncInventory() error {
	if err := client.conn.WritePacket(client.inventoryContentPacket(gtprotocol.WindowIDInventory)); err != nil {
		return fmt.Errorf("resync InventoryContent inventory: %w", err)
	}
	if err := client.conn.WritePacket(client.inventoryContentPacket(gtprotocol.WindowIDOffHand)); err != nil {
		return fmt.Errorf("resync InventoryContent offhand: %w", err)
	}
	if err := client.resendHeldSlot(); err != nil {
		return fmt.Errorf("resync MobEquipment held slot: %w", err)
	}
	return nil
}

func (client *MCPEClient) creativeInventoryAllowed() bool {
	return gameModeID(client.handler.gameMode) == packet.GameTypeCreative
}

func (inv *mcpeInventory) snapshot() mcpeInventorySnapshot {
	return mcpeInventorySnapshot{
		main:           inv.main,
		cursor:         inv.cursor,
		offhand:        inv.offhand,
		selectedHotbar: inv.selectedHotbar,
		nextStackID:    inv.nextStackID,
	}
}

func (inv *mcpeInventory) restore(snapshot mcpeInventorySnapshot) {
	inv.main = snapshot.main
	inv.cursor = snapshot.cursor
	inv.offhand = snapshot.offhand
	inv.selectedHotbar = snapshot.selectedHotbar
	inv.nextStackID = snapshot.nextStackID
}

func (inv *mcpeInventory) mainContent() []gtprotocol.ItemInstance {
	content := make([]gtprotocol.ItemInstance, 0, len(inv.main))
	for _, item := range inv.main {
		content = append(content, cloneItemInstance(item))
	}
	return content
}

func (inv *mcpeInventory) heldItem() gtprotocol.ItemInstance {
	if inv.selectedHotbar >= mcpeHotbarSlots {
		return gtprotocol.ItemInstance{}
	}
	return cloneItemInstance(inv.main[inv.selectedHotbar])
}

func (inv *mcpeInventory) resolveSlot(container, slot byte) (mcpeInventorySlot, error) {
	ref := mcpeInventorySlot{container: container, slot: slot}
	switch container {
	case gtprotocol.ContainerInventory, gtprotocol.ContainerCombinedHotBarAndInventory:
		if slot >= mcpeInventorySlots {
			return ref, stackRequestError(gtprotocol.ItemStackResponseStatusFailedToValidateSrcSlot, "inventory slot %d out of range", slot)
		}
	case gtprotocol.ContainerHotBar:
		if slot >= mcpeHotbarSlots {
			return ref, stackRequestError(gtprotocol.ItemStackResponseStatusFailedToValidateSrcSlot, "hotbar slot %d out of range", slot)
		}
	case gtprotocol.ContainerCursor:
		if slot != mcpeCursorSlot {
			return ref, stackRequestError(gtprotocol.ItemStackResponseStatusFailedToValidateSrcSlot, "cursor slot %d out of range", slot)
		}
	case gtprotocol.ContainerOffhand:
		if slot != 0 {
			return ref, stackRequestError(gtprotocol.ItemStackResponseStatusFailedToValidateSrcSlot, "offhand slot %d out of range", slot)
		}
	default:
		return ref, stackRequestError(gtprotocol.ItemStackResponseStatusInvalidSourceContainer, "unsupported container %d", container)
	}
	return ref, nil
}

func (inv *mcpeInventory) item(ref mcpeInventorySlot) (gtprotocol.ItemInstance, error) {
	switch ref.container {
	case gtprotocol.ContainerInventory, gtprotocol.ContainerCombinedHotBarAndInventory, gtprotocol.ContainerHotBar:
		if ref.slot >= mcpeInventorySlots {
			return gtprotocol.ItemInstance{}, fmt.Errorf("inventory slot %d out of range", ref.slot)
		}
		return cloneItemInstance(inv.main[ref.slot]), nil
	case gtprotocol.ContainerCursor:
		return cloneItemInstance(inv.cursor), nil
	case gtprotocol.ContainerOffhand:
		return cloneItemInstance(inv.offhand), nil
	default:
		return gtprotocol.ItemInstance{}, fmt.Errorf("unsupported inventory container %d", ref.container)
	}
}

func (inv *mcpeInventory) setItem(ref mcpeInventorySlot, item gtprotocol.ItemInstance) error {
	item = inv.normaliseItem(item)
	switch ref.container {
	case gtprotocol.ContainerInventory, gtprotocol.ContainerCombinedHotBarAndInventory, gtprotocol.ContainerHotBar:
		if ref.slot >= mcpeInventorySlots {
			return fmt.Errorf("inventory slot %d out of range", ref.slot)
		}
		inv.main[ref.slot] = item
	case gtprotocol.ContainerCursor:
		inv.cursor = item
	case gtprotocol.ContainerOffhand:
		inv.offhand = item
	default:
		return fmt.Errorf("unsupported inventory container %d", ref.container)
	}
	return nil
}

func (inv *mcpeInventory) normaliseItem(item gtprotocol.ItemInstance) gtprotocol.ItemInstance {
	if itemIsEmpty(item) {
		return gtprotocol.ItemInstance{}
	}
	item = cloneItemInstance(item)
	if item.StackNetworkID <= 0 {
		item.StackNetworkID = inv.nextNetworkStackID()
		return item
	}
	if item.StackNetworkID >= inv.nextStackID {
		inv.nextStackID = item.StackNetworkID + 1
	}
	return item
}

func (inv *mcpeInventory) nextNetworkStackID() int32 {
	if inv.nextStackID < 1 {
		inv.nextStackID = 1
	}
	id := inv.nextStackID
	inv.nextStackID++
	return id
}

func (inv *mcpeInventory) resolveStackNetworkID(id int32, ref mcpeInventorySlot) (int32, error) {
	if id >= 0 {
		return id, nil
	}
	for _, key := range append([]mcpeInventorySlot{ref}, aliasInventorySlots(ref)...) {
		if slots := inv.responseStackIDs[id]; slots != nil {
			if resolved, ok := slots[key]; ok {
				return resolved, nil
			}
		}
	}
	return 0, stackRequestError(gtprotocol.ItemStackResponseStatusInvalidItemNetId, "unknown predicted stack network ID %d", id)
}

func (inv *mcpeInventory) rememberResponseStackIDs(requestID int32, changes map[mcpeInventorySlot]gtprotocol.ItemInstance) {
	if len(changes) == 0 {
		return
	}
	if len(inv.responseStackIDs) > 256 {
		inv.responseStackIDs = make(map[int32]map[mcpeInventorySlot]int32)
	}
	slots := make(map[mcpeInventorySlot]int32, len(changes)*3)
	for ref, item := range changes {
		id := itemStackNetworkID(item)
		for _, alias := range append([]mcpeInventorySlot{ref}, aliasInventorySlots(ref)...) {
			slots[alias] = id
		}
	}
	inv.responseStackIDs[requestID] = slots
	if requestID != 0 {
		inv.responseStackIDs[-requestID] = slots
	}
}

func stackResponseContainerInfo(changes map[mcpeInventorySlot]gtprotocol.ItemInstance) []gtprotocol.StackResponseContainerInfo {
	if len(changes) == 0 {
		return nil
	}
	containers := make(map[byte][]gtprotocol.StackResponseSlotInfo)
	for ref, item := range changes {
		containers[ref.container] = append(containers[ref.container], stackResponseSlotInfo(ref, item))
	}

	ids := make([]int, 0, len(containers))
	for id := range containers {
		ids = append(ids, int(id))
	}
	sort.Ints(ids)

	info := make([]gtprotocol.StackResponseContainerInfo, 0, len(ids))
	for _, id := range ids {
		slots := containers[byte(id)]
		sort.Slice(slots, func(i, j int) bool {
			return slots[i].Slot < slots[j].Slot
		})
		info = append(info, gtprotocol.StackResponseContainerInfo{
			Container: gtprotocol.FullContainerName{ContainerID: byte(id)},
			SlotInfo:  slots,
		})
	}
	return info
}

func stackResponseSlotInfo(ref mcpeInventorySlot, item gtprotocol.ItemInstance) gtprotocol.StackResponseSlotInfo {
	count := byte(0)
	if !itemIsEmpty(item) {
		count = byte(item.Stack.Count)
	}
	return gtprotocol.StackResponseSlotInfo{
		Slot:           ref.slot,
		HotbarSlot:     ref.slot,
		Count:          count,
		StackNetworkID: itemStackNetworkID(item),
	}
}

func windowIDForInventorySlot(ref mcpeInventorySlot) uint32 {
	switch ref.container {
	case gtprotocol.ContainerOffhand:
		return gtprotocol.WindowIDOffHand
	case gtprotocol.ContainerCursor:
		return gtprotocol.WindowIDUI
	default:
		return gtprotocol.WindowIDInventory
	}
}

func aliasInventorySlots(ref mcpeInventorySlot) []mcpeInventorySlot {
	switch ref.container {
	case gtprotocol.ContainerInventory, gtprotocol.ContainerCombinedHotBarAndInventory, gtprotocol.ContainerHotBar:
		aliases := []mcpeInventorySlot{
			{container: gtprotocol.ContainerInventory, slot: ref.slot},
			{container: gtprotocol.ContainerCombinedHotBarAndInventory, slot: ref.slot},
		}
		if ref.slot < mcpeHotbarSlots {
			aliases = append(aliases, mcpeInventorySlot{container: gtprotocol.ContainerHotBar, slot: ref.slot})
		}
		return aliases
	default:
		return nil
	}
}

func itemStackNetworkID(item gtprotocol.ItemInstance) int32 {
	if itemIsEmpty(item) {
		return 0
	}
	return item.StackNetworkID
}

func itemIsEmpty(item gtprotocol.ItemInstance) bool {
	return item.Stack.NetworkID == 0 || item.Stack.Count == 0
}

func sameItemInstance(a, b gtprotocol.ItemInstance) bool {
	if itemIsEmpty(a) && itemIsEmpty(b) {
		return true
	}
	return reflect.DeepEqual(a, b)
}

func sameStackType(a, b gtprotocol.ItemInstance) bool {
	if itemIsEmpty(a) || itemIsEmpty(b) {
		return itemIsEmpty(a) && itemIsEmpty(b)
	}
	a.Stack.Count, b.Stack.Count = 0, 0
	a.StackNetworkID, b.StackNetworkID = 0, 0
	return reflect.DeepEqual(a.Stack, b.Stack)
}

func cloneItemInstance(item gtprotocol.ItemInstance) gtprotocol.ItemInstance {
	if len(item.Stack.NBTData) != 0 {
		nbtData := make(map[string]any, len(item.Stack.NBTData))
		for key, value := range item.Stack.NBTData {
			nbtData[key] = value
		}
		item.Stack.NBTData = nbtData
	}
	item.Stack.CanBePlacedOn = append([]string(nil), item.Stack.CanBePlacedOn...)
	item.Stack.CanBreak = append([]string(nil), item.Stack.CanBreak...)
	return item
}

func stackRequestError(status int, format string, args ...any) error {
	return itemStackRequestError{
		status: uint8(status),
		msg:    fmt.Sprintf(format, args...),
	}
}

func stackRequestStatus(err error) uint8 {
	var requestErr itemStackRequestError
	if errors.As(err, &requestErr) {
		return requestErr.status
	}
	return gtprotocol.ItemStackResponseStatusError
}
