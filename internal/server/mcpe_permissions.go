package server

import (
	"sync"

	"github.com/google/uuid"
)

const (
	permissionCommandHelp = "bds.command.help"
	permissionCommandList = "bds.command.list"
	permissionCommandSay  = "bds.command.say"
	permissionCommandStop = "bds.command.stop"
	permissionCommandOp   = "bds.command.op"
	permissionCommandDeop = "bds.command.deop"
)

var defaultPlayerPermissions = map[string]bool{
	permissionCommandHelp: true,
	permissionCommandList: true,
}

type permissionManager struct {
	mu        sync.RWMutex
	operators map[uuid.UUID]bool
}

func newPermissionManager() *permissionManager {
	return &permissionManager{operators: make(map[uuid.UUID]bool)}
}

func (manager *permissionManager) hasPlayerPermission(client *MCPEClient, permission string) bool {
	if permission == "" {
		return true
	}
	if defaultPlayerPermissions[permission] {
		return true
	}
	if client == nil {
		return false
	}
	return manager.isOperator(client.player.uuid)
}

func (manager *permissionManager) isOperator(id uuid.UUID) bool {
	manager.mu.RLock()
	defer manager.mu.RUnlock()

	return manager.operators[id]
}

func (manager *permissionManager) setOperator(id uuid.UUID, value bool) {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	if value {
		manager.operators[id] = true
		return
	}
	delete(manager.operators, id)
}

type playerCommandSender struct {
	client *MCPEClient
}

func (sender playerCommandSender) Name() string {
	return sender.client.login.Identity.DisplayName
}

func (sender playerCommandSender) HasPermission(permission string) bool {
	return sender.client.handler.permissions.hasPlayerPermission(sender.client, permission)
}

type consoleCommandSender struct{}

func (consoleCommandSender) Name() string {
	return "Server"
}

func (consoleCommandSender) HasPermission(string) bool {
	return true
}
