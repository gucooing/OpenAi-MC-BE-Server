package server

import (
	"context"
	"io"
	"log/slog"
	"testing"

	gtprotocol "github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"

	appworld "gucooing/bds/internal/world"
)

func TestMCPEInventoryInitialSyncAndHeldSlot(t *testing.T) {
	handler := newInventoryTestHandler(t, "Inventory Initial Test", "Survival")
	ctx := context.Background()
	client, conn, _ := spawnTestClient(t, ctx, handler, "InventoryBot", "4c48d991-5246-4909-a232-8d1343259e54")

	invContent := firstPacketOfType[*packet.InventoryContent](t, conn.packets, 0)
	if invContent.WindowID != gtprotocol.WindowIDInventory || len(invContent.Content) != mcpeInventorySlots {
		t.Fatalf("InventoryContent = %+v, want %d empty inventory slots", invContent, mcpeInventorySlots)
	}
	for slot, item := range invContent.Content {
		if !itemIsEmpty(item) {
			t.Fatalf("inventory slot %d = %+v, want empty", slot, item)
		}
	}
	offhand := packetAfter[*packet.InventoryContent](t, conn.packets, indexOfPacket(t, conn.packets, invContent)+1)
	if offhand.WindowID != gtprotocol.WindowIDOffHand || len(offhand.Content) != 1 || !itemIsEmpty(offhand.Content[0]) {
		t.Fatalf("offhand InventoryContent = %+v, want one empty offhand slot", offhand)
	}
	equipment := packetAfter[*packet.MobEquipment](t, conn.packets, indexOfPacket(t, conn.packets, offhand)+1)
	if equipment.EntityRuntimeID != client.runtimeID || equipment.HotBarSlot != 0 || equipment.InventorySlot != 0 || !itemIsEmpty(equipment.NewItem) {
		t.Fatalf("MobEquipment = %+v, want empty selected hotbar slot 0", equipment)
	}
}

func TestMCPEMobEquipmentChangesHeldSlotAndBroadcasts(t *testing.T) {
	handler := newInventoryTestHandler(t, "Inventory Equipment Test", "Survival")
	ctx := context.Background()
	alice, aliceConn, _ := spawnTestClient(t, ctx, handler, "AliceInv", "97e59c71-1972-48e6-b1bd-941369db6e51")
	bob, _, _ := spawnTestClient(t, ctx, handler, "BobInv", "2bd139e6-c596-4bbf-9524-49b47d4f7f2e")

	item := testItemInstance(2, 11, 1)
	if err := bob.inventory.setItem(mcpeInventorySlot{container: gtprotocol.ContainerInventory, slot: 2}, item); err != nil {
		t.Fatalf("seed inventory: %v", err)
	}
	before := len(aliceConn.packets)
	if err := bob.HandlePacket(ctx, &packet.MobEquipment{
		EntityRuntimeID: bob.runtimeID,
		NewItem:         item,
		InventorySlot:   2,
		HotBarSlot:      2,
		WindowID:        byte(gtprotocol.WindowIDInventory),
	}); err != nil {
		t.Fatalf("HandlePacket(MobEquipment) returned error: %v", err)
	}
	if bob.inventory.selectedHotbar != 2 {
		t.Fatalf("selected hotbar = %d, want 2", bob.inventory.selectedHotbar)
	}
	pk := packetAt[*packet.MobEquipment](t, aliceConn.packets, before)
	if pk.EntityRuntimeID != bob.runtimeID || pk.HotBarSlot != 2 || !sameItemInstance(pk.NewItem, item) {
		t.Fatalf("broadcast MobEquipment = %+v, want Bob holding seeded item", pk)
	}

	if alice.runtimeID == 0 {
		t.Fatalf("Alice runtime ID was not initialised")
	}
}

func TestMCPEMobEquipmentResyncsMismatchedSelectedSlot(t *testing.T) {
	handler := newInventoryTestHandler(t, "Inventory Equipment Resync Test", "Survival")
	ctx := context.Background()
	client, conn, _ := spawnTestClient(t, ctx, handler, "ResyncInv", "9a605df7-9fbb-43f6-8ca6-98119a5a9668")

	before := len(conn.packets)
	if err := client.HandlePacket(ctx, &packet.MobEquipment{
		EntityRuntimeID: client.runtimeID,
		NewItem:         testItemInstance(9, 71, 1),
		InventorySlot:   0,
		HotBarSlot:      0,
		WindowID:        byte(gtprotocol.WindowIDInventory),
	}); err != nil {
		t.Fatalf("HandlePacket(MobEquipment mismatch) returned error: %v", err)
	}
	slot := packetAt[*packet.InventorySlot](t, conn.packets, before)
	if slot.WindowID != gtprotocol.WindowIDInventory || slot.Slot != 0 || !itemIsEmpty(slot.NewItem) {
		t.Fatalf("InventorySlot resync = %+v, want empty slot 0", slot)
	}
	if client.inventory.selectedHotbar != 0 {
		t.Fatalf("selected hotbar = %d, want 0", client.inventory.selectedHotbar)
	}
}

func TestMCPEItemStackRequestMovesItemsBetweenInventoryAndCursor(t *testing.T) {
	handler := newInventoryTestHandler(t, "Inventory StackRequest Test", "Survival")
	ctx := context.Background()
	client, conn, _ := spawnTestClient(t, ctx, handler, "StackBot", "bc52eb7a-ed64-46cd-9618-51b7ed1aca4a")

	item := testItemInstance(3, 21, 4)
	if err := client.inventory.setItem(mcpeInventorySlot{container: gtprotocol.ContainerInventory, slot: 0}, item); err != nil {
		t.Fatalf("seed inventory: %v", err)
	}

	before := len(conn.packets)
	take := &gtprotocol.TakeStackRequestAction{}
	take.Count = 2
	take.Source = stackRequestSlot(gtprotocol.ContainerInventory, 0, 21)
	take.Destination = stackRequestSlot(gtprotocol.ContainerCursor, 0, 0)
	if err := client.HandlePacket(ctx, &packet.ItemStackRequest{Requests: []gtprotocol.ItemStackRequest{{
		RequestID: 7,
		Actions:   []gtprotocol.StackRequestAction{take},
	}}}); err != nil {
		t.Fatalf("HandlePacket(ItemStackRequest take) returned error: %v", err)
	}
	response := packetAt[*packet.ItemStackResponse](t, conn.packets, before)
	assertStackResponseOK(t, response, 7)
	sourceAfter, _ := client.inventory.item(mcpeInventorySlot{container: gtprotocol.ContainerInventory, slot: 0})
	cursorAfter, _ := client.inventory.item(mcpeInventorySlot{container: gtprotocol.ContainerCursor, slot: 0})
	if sourceAfter.Stack.Count != 2 || sourceAfter.StackNetworkID != 21 {
		t.Fatalf("inventory slot 0 = %+v, want original stack reduced to 2", sourceAfter)
	}
	if cursorAfter.Stack.Count != 2 || cursorAfter.StackNetworkID == 0 || cursorAfter.StackNetworkID == 21 {
		t.Fatalf("cursor = %+v, want split stack with a new network ID", cursorAfter)
	}

	before = len(conn.packets)
	place := &gtprotocol.PlaceStackRequestAction{}
	place.Count = 2
	place.Source = stackRequestSlot(gtprotocol.ContainerCursor, 0, cursorAfter.StackNetworkID)
	place.Destination = stackRequestSlot(gtprotocol.ContainerInventory, 1, 0)
	if err := client.HandlePacket(ctx, &packet.ItemStackRequest{Requests: []gtprotocol.ItemStackRequest{{
		RequestID: 8,
		Actions:   []gtprotocol.StackRequestAction{place},
	}}}); err != nil {
		t.Fatalf("HandlePacket(ItemStackRequest place) returned error: %v", err)
	}
	response = packetAt[*packet.ItemStackResponse](t, conn.packets, before)
	assertStackResponseOK(t, response, 8)
	slot1, _ := client.inventory.item(mcpeInventorySlot{container: gtprotocol.ContainerInventory, slot: 1})
	cursorAfter, _ = client.inventory.item(mcpeInventorySlot{container: gtprotocol.ContainerCursor, slot: 0})
	if !itemIsEmpty(cursorAfter) || slot1.Stack.Count != 2 || slot1.StackNetworkID != responseSlotStackID(response, gtprotocol.ContainerInventory, 1) {
		t.Fatalf("slot 1 = %+v cursor = %+v, want placed split stack and empty cursor", slot1, cursorAfter)
	}
}

func TestMCPEItemStackRequestSwapsDropsDestroysAndMines(t *testing.T) {
	handler := newInventoryTestHandler(t, "Inventory More StackRequest Test", "Creative")
	ctx := context.Background()
	client, conn, _ := spawnTestClient(t, ctx, handler, "MoreStackBot", "f2fdbbb8-58bb-4c96-a9d0-91b29e4d32ba")

	slot0Item := testItemInstance(6, 51, 3)
	slot1Item := testItemInstance(7, 52, 2)
	if err := client.inventory.setItem(mcpeInventorySlot{container: gtprotocol.ContainerInventory, slot: 0}, slot0Item); err != nil {
		t.Fatalf("seed slot 0: %v", err)
	}
	if err := client.inventory.setItem(mcpeInventorySlot{container: gtprotocol.ContainerInventory, slot: 1}, slot1Item); err != nil {
		t.Fatalf("seed slot 1: %v", err)
	}

	before := len(conn.packets)
	swap := &gtprotocol.SwapStackRequestAction{
		Source:      stackRequestSlot(gtprotocol.ContainerInventory, 0, 51),
		Destination: stackRequestSlot(gtprotocol.ContainerInventory, 1, 52),
	}
	if err := client.HandlePacket(ctx, &packet.ItemStackRequest{Requests: []gtprotocol.ItemStackRequest{{
		RequestID: 11,
		Actions:   []gtprotocol.StackRequestAction{swap},
	}}}); err != nil {
		t.Fatalf("HandlePacket(ItemStackRequest swap) returned error: %v", err)
	}
	assertStackResponseOK(t, packetAt[*packet.ItemStackResponse](t, conn.packets, before), 11)
	slot0, _ := client.inventory.item(mcpeInventorySlot{container: gtprotocol.ContainerInventory, slot: 0})
	slot1, _ := client.inventory.item(mcpeInventorySlot{container: gtprotocol.ContainerInventory, slot: 1})
	if slot0.StackNetworkID != 52 || slot1.StackNetworkID != 51 {
		t.Fatalf("slot0=%+v slot1=%+v, want swapped stack IDs 52 and 51", slot0, slot1)
	}

	before = len(conn.packets)
	drop := &gtprotocol.DropStackRequestAction{
		Count:  1,
		Source: stackRequestSlot(gtprotocol.ContainerInventory, 1, 51),
	}
	mine := &gtprotocol.MineBlockStackRequestAction{
		HotbarSlot:          1,
		StackNetworkID:      51,
		PredictedDurability: 0,
	}
	if err := client.HandlePacket(ctx, &packet.ItemStackRequest{Requests: []gtprotocol.ItemStackRequest{{
		RequestID: 12,
		Actions:   []gtprotocol.StackRequestAction{drop, mine},
	}}}); err != nil {
		t.Fatalf("HandlePacket(ItemStackRequest drop/mine) returned error: %v", err)
	}
	assertStackResponseOK(t, packetAt[*packet.ItemStackResponse](t, conn.packets, before), 12)
	slot1, _ = client.inventory.item(mcpeInventorySlot{container: gtprotocol.ContainerInventory, slot: 1})
	if slot1.Stack.Count != 2 {
		t.Fatalf("slot 1 = %+v, want count reduced to 2", slot1)
	}

	before = len(conn.packets)
	destroy := &gtprotocol.DestroyStackRequestAction{
		Count:  2,
		Source: stackRequestSlot(gtprotocol.ContainerInventory, 1, 51),
	}
	if err := client.HandlePacket(ctx, &packet.ItemStackRequest{Requests: []gtprotocol.ItemStackRequest{{
		RequestID: 13,
		Actions:   []gtprotocol.StackRequestAction{destroy},
	}}}); err != nil {
		t.Fatalf("HandlePacket(ItemStackRequest destroy) returned error: %v", err)
	}
	assertStackResponseOK(t, packetAt[*packet.ItemStackResponse](t, conn.packets, before), 13)
	slot1, _ = client.inventory.item(mcpeInventorySlot{container: gtprotocol.ContainerInventory, slot: 1})
	if !itemIsEmpty(slot1) {
		t.Fatalf("slot 1 = %+v, want empty after destroy", slot1)
	}
}

func TestMCPEItemStackRequestRejectsMismatchAndResyncs(t *testing.T) {
	handler := newInventoryTestHandler(t, "Inventory Reject Test", "Survival")
	ctx := context.Background()
	client, conn, _ := spawnTestClient(t, ctx, handler, "RejectBot", "f7a76c74-b2ed-44ad-aa75-6e7fdcd9962c")

	item := testItemInstance(4, 31, 1)
	if err := client.inventory.setItem(mcpeInventorySlot{container: gtprotocol.ContainerInventory, slot: 0}, item); err != nil {
		t.Fatalf("seed inventory: %v", err)
	}
	before := len(conn.packets)
	drop := &gtprotocol.DropStackRequestAction{
		Count:  1,
		Source: stackRequestSlot(gtprotocol.ContainerInventory, 0, 999),
	}
	if err := client.HandlePacket(ctx, &packet.ItemStackRequest{Requests: []gtprotocol.ItemStackRequest{{
		RequestID: 9,
		Actions:   []gtprotocol.StackRequestAction{drop},
	}}}); err != nil {
		t.Fatalf("HandlePacket(ItemStackRequest mismatch) returned error: %v", err)
	}
	response := packetAt[*packet.ItemStackResponse](t, conn.packets, before)
	if len(response.Responses) != 1 || response.Responses[0].Status != gtprotocol.ItemStackResponseStatusInvalidItemNetId || response.Responses[0].RequestID != 9 {
		t.Fatalf("ItemStackResponse = %+v, want InvalidItemNetId for request 9", response)
	}
	packetAt[*packet.InventoryContent](t, conn.packets, before+1)
	packetAt[*packet.InventoryContent](t, conn.packets, before+2)
	packetAt[*packet.MobEquipment](t, conn.packets, before+3)
	slot0, _ := client.inventory.item(mcpeInventorySlot{container: gtprotocol.ContainerInventory, slot: 0})
	if !sameItemInstance(slot0, item) {
		t.Fatalf("slot 0 = %+v, want unchanged item after rejected request", slot0)
	}
}

func TestMCPEItemStackRequestRejectsUnsupportedConsume(t *testing.T) {
	handler := newInventoryTestHandler(t, "Inventory Unsupported Test", "Survival")
	ctx := context.Background()
	client, conn, _ := spawnTestClient(t, ctx, handler, "UnsupportedBot", "ce8a3227-a89f-4cd9-9196-8f0a6a512765")

	item := testItemInstance(8, 61, 1)
	if err := client.inventory.setItem(mcpeInventorySlot{container: gtprotocol.ContainerInventory, slot: 0}, item); err != nil {
		t.Fatalf("seed inventory: %v", err)
	}
	before := len(conn.packets)
	consume := &gtprotocol.ConsumeStackRequestAction{}
	consume.Count = 1
	consume.Source = stackRequestSlot(gtprotocol.ContainerInventory, 0, 61)
	if err := client.HandlePacket(ctx, &packet.ItemStackRequest{Requests: []gtprotocol.ItemStackRequest{{
		RequestID: 14,
		Actions:   []gtprotocol.StackRequestAction{consume},
	}}}); err != nil {
		t.Fatalf("HandlePacket(ItemStackRequest consume) returned error: %v", err)
	}
	response := packetAt[*packet.ItemStackResponse](t, conn.packets, before)
	if len(response.Responses) != 1 || response.Responses[0].Status != gtprotocol.ItemStackResponseStatusActionRequestNotAllowed || response.Responses[0].RequestID != 14 {
		t.Fatalf("ItemStackResponse = %+v, want ActionRequestNotAllowed for consume request", response)
	}
	slot0, _ := client.inventory.item(mcpeInventorySlot{container: gtprotocol.ContainerInventory, slot: 0})
	if !sameItemInstance(slot0, item) {
		t.Fatalf("slot 0 = %+v, want unchanged item after unsupported consume", slot0)
	}
}

func TestMCPEPlayerAuthInputProcessesEmbeddedItemStackRequest(t *testing.T) {
	handler := newInventoryTestHandler(t, "Inventory AuthInput Test", "Survival")
	ctx := context.Background()
	client, conn, _ := spawnTestClient(t, ctx, handler, "AuthStackBot", "5f64f2c5-fb14-4a58-8272-8515ba4a7130")

	item := testItemInstance(5, 41, 1)
	if err := client.inventory.setItem(mcpeInventorySlot{container: gtprotocol.ContainerInventory, slot: 0}, item); err != nil {
		t.Fatalf("seed inventory: %v", err)
	}
	input := gtprotocol.NewBitset(packet.PlayerAuthInputBitsetSize)
	input.Set(packet.InputFlagPerformItemStackRequest)
	drop := &gtprotocol.DropStackRequestAction{
		Count:  1,
		Source: stackRequestSlot(gtprotocol.ContainerInventory, 0, 41),
	}
	before := len(conn.packets)
	if err := client.HandlePacket(ctx, &packet.PlayerAuthInput{
		Position:  client.player.position,
		InputData: input,
		ItemStackRequest: gtprotocol.ItemStackRequest{
			RequestID: 10,
			Actions:   []gtprotocol.StackRequestAction{drop},
		},
	}); err != nil {
		t.Fatalf("HandlePacket(PlayerAuthInput embedded stack request) returned error: %v", err)
	}
	response := packetAt[*packet.ItemStackResponse](t, conn.packets, before)
	assertStackResponseOK(t, response, 10)
	slot0, _ := client.inventory.item(mcpeInventorySlot{container: gtprotocol.ContainerInventory, slot: 0})
	if !itemIsEmpty(slot0) {
		t.Fatalf("slot 0 = %+v, want empty after embedded drop request", slot0)
	}
}

func newInventoryTestHandler(t *testing.T, name, gameMode string) *MCPEHandler {
	t.Helper()
	world, err := appworld.NewFlatGenerator()
	if err != nil {
		t.Fatalf("NewFlatGenerator() returned error: %v", err)
	}
	handler, err := NewMCPEHandler(MCPEOptions{
		Logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		ServerName:   name,
		ServerBrand:  "BetterAltay-Go",
		GameMode:     gameMode,
		MaxPlayers:   5,
		ViewDistance: 1,
		World:        world,
	})
	if err != nil {
		t.Fatalf("NewMCPEHandler() returned error: %v", err)
	}
	return handler
}

func testItemInstance(networkID, stackID int32, count uint16) gtprotocol.ItemInstance {
	return gtprotocol.ItemInstance{
		StackNetworkID: stackID,
		Stack: gtprotocol.ItemStack{
			ItemType: gtprotocol.ItemType{NetworkID: networkID},
			Count:    count,
		},
	}
}

func stackRequestSlot(container, slot byte, stackID int32) gtprotocol.StackRequestSlotInfo {
	return gtprotocol.StackRequestSlotInfo{
		Container:      gtprotocol.FullContainerName{ContainerID: container},
		Slot:           slot,
		StackNetworkID: stackID,
	}
}

func assertStackResponseOK(t *testing.T, pk *packet.ItemStackResponse, requestID int32) {
	t.Helper()
	if len(pk.Responses) != 1 || pk.Responses[0].Status != gtprotocol.ItemStackResponseStatusOK || pk.Responses[0].RequestID != requestID {
		t.Fatalf("ItemStackResponse = %+v, want OK for request %d", pk, requestID)
	}
}

func responseSlotStackID(pk *packet.ItemStackResponse, container, slot byte) int32 {
	for _, response := range pk.Responses {
		for _, info := range response.ContainerInfo {
			if info.Container.ContainerID != container {
				continue
			}
			for _, slotInfo := range info.SlotInfo {
				if slotInfo.Slot == slot {
					return slotInfo.StackNetworkID
				}
			}
		}
	}
	return 0
}

func firstPacketOfType[T packet.Packet](t *testing.T, packets []packet.Packet, start int) T {
	t.Helper()
	return packetAfter[T](t, packets, start)
}

func packetAfter[T packet.Packet](t *testing.T, packets []packet.Packet, start int) T {
	t.Helper()
	for i := start; i < len(packets); i++ {
		if pk, ok := packets[i].(T); ok {
			return pk
		}
	}
	var want T
	t.Fatalf("no packet of type %T after index %d", want, start)
	var zero T
	return zero
}

func indexOfPacket(t *testing.T, packets []packet.Packet, target packet.Packet) int {
	t.Helper()
	for i, pk := range packets {
		if pk == target {
			return i
		}
	}
	t.Fatalf("packet %T was not found", target)
	return -1
}
