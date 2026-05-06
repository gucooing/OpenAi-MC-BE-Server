package server

import (
	"context"
	"fmt"
	"sort"
	"strings"

	gtprotocol "github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"

	appcommand "gucooing/bds/internal/command"
)

const maxChatMessageBytes = 512

func newDefaultCommands(handler *MCPEHandler) *appcommand.Registry {
	registry := appcommand.NewRegistry()
	mustRegister := func(cmd appcommand.Command) {
		if err := registry.Register(cmd); err != nil {
			panic(err)
		}
	}

	mustRegister(appcommand.Command{
		Name:        "help",
		Description: "Shows available commands",
		Usage:       "/help",
		Permission:  permissionCommandHelp,
		Run: func(_ context.Context, sender appcommand.Sender, _ appcommand.Invocation) appcommand.Result {
			visible := registry.Visible(sender)
			lines := make([]string, 0, len(visible)+1)
			lines = append(lines, "Available commands:")
			for _, cmd := range visible {
				lines = append(lines, fmt.Sprintf("%s - %s", cmd.Usage, cmd.Description))
			}
			return appcommand.Success(lines...)
		},
	})
	mustRegister(appcommand.Command{
		Name:        "list",
		Description: "Lists online players",
		Usage:       "/list",
		Permission:  permissionCommandList,
		Run: func(_ context.Context, _ appcommand.Sender, _ appcommand.Invocation) appcommand.Result {
			names := handler.onlinePlayerNames()
			return appcommand.Success(fmt.Sprintf("There are %d/%d players online: %s", len(names), handler.maxPlayers, strings.Join(names, ", ")))
		},
	})
	mustRegister(appcommand.Command{
		Name:        "say",
		Description: "Broadcasts a server message",
		Usage:       "/say <message>",
		Permission:  permissionCommandSay,
		Parameters: []appcommand.Parameter{
			appcommand.BasicParameter("message", gtprotocol.CommandArgTypeMessage, false),
		},
		Run: func(_ context.Context, sender appcommand.Sender, invocation appcommand.Invocation) appcommand.Result {
			message := strings.TrimSpace(invocation.RawArgs)
			if message == "" {
				return appcommand.Failure("Usage: /say <message>")
			}
			if err := handler.broadcastText(&packet.Text{
				TextType:   packet.TextTypeAnnouncement,
				SourceName: sender.Name(),
				Message:    message,
			}); err != nil {
				return appcommand.Failure("Failed to broadcast message: " + err.Error())
			}
			return appcommand.Success("Message sent.")
		},
	})
	mustRegister(appcommand.Command{
		Name:        "stop",
		Description: "Stops the server",
		Usage:       "/stop",
		Permission:  permissionCommandStop,
		Run: func(_ context.Context, sender appcommand.Sender, _ appcommand.Invocation) appcommand.Result {
			_ = handler.broadcastSystemText("Server is stopping.")
			if handler.shutdown == nil {
				return appcommand.Failure("Shutdown is not wired for this runtime.")
			}
			handler.shutdown()
			return appcommand.Success("Stopping the server.")
		},
	})
	mustRegister(appcommand.Command{
		Name:        "op",
		Description: "Grants operator permissions to an online player",
		Usage:       "/op <player>",
		Permission:  permissionCommandOp,
		Parameters: []appcommand.Parameter{
			appcommand.BasicParameter("player", gtprotocol.CommandArgTypeTarget, false),
		},
		Run: func(_ context.Context, _ appcommand.Sender, invocation appcommand.Invocation) appcommand.Result {
			targetName := firstArg(invocation.Args)
			if targetName == "" {
				return appcommand.Failure("Usage: /op <player>")
			}
			target := handler.playerByName(targetName)
			if target == nil {
				return appcommand.Failure("Player not found: " + targetName)
			}
			handler.permissions.setOperator(target.player.uuid, true)
			_ = target.sendAvailableCommands()
			_ = target.sendRawText("You are now an operator.")
			return appcommand.Success("Made " + target.login.Identity.DisplayName + " a server operator.")
		},
	})
	mustRegister(appcommand.Command{
		Name:        "deop",
		Description: "Revokes operator permissions from an online player",
		Usage:       "/deop <player>",
		Permission:  permissionCommandDeop,
		Parameters: []appcommand.Parameter{
			appcommand.BasicParameter("player", gtprotocol.CommandArgTypeTarget, false),
		},
		Run: func(_ context.Context, _ appcommand.Sender, invocation appcommand.Invocation) appcommand.Result {
			targetName := firstArg(invocation.Args)
			if targetName == "" {
				return appcommand.Failure("Usage: /deop <player>")
			}
			target := handler.playerByName(targetName)
			if target == nil {
				return appcommand.Failure("Player not found: " + targetName)
			}
			handler.permissions.setOperator(target.player.uuid, false)
			_ = target.sendAvailableCommands()
			_ = target.sendRawText("You are no longer an operator.")
			return appcommand.Success("Removed operator permissions from " + target.login.Identity.DisplayName + ".")
		},
	})
	return registry
}

func (handler *MCPEHandler) ExecuteConsoleCommand(ctx context.Context, line string) appcommand.Result {
	return handler.commands.Dispatch(ctx, consoleCommandSender{}, line)
}

func (client *MCPEClient) handleText(ctx context.Context, pk *packet.Text) error {
	if client.state != stateSpawned || pk.TextType != packet.TextTypeChat {
		return nil
	}
	message := cleanChatMessage(pk.Message)
	if message == "" {
		return nil
	}
	if strings.HasPrefix(message, "/") {
		result := client.handler.commands.Dispatch(ctx, playerCommandSender{client: client}, message)
		for _, output := range result.Messages {
			if err := client.sendRawText(output); err != nil {
				return err
			}
		}
		return nil
	}
	return client.handler.broadcastText(&packet.Text{
		TextType:        packet.TextTypeChat,
		SourceName:      client.login.Identity.DisplayName,
		Message:         message,
		XUID:            client.login.Identity.XUID,
		PlatformChatID:  pk.PlatformChatID,
		FilteredMessage: pk.FilteredMessage,
	})
}

func (client *MCPEClient) handleCommandRequest(ctx context.Context, pk *packet.CommandRequest) error {
	if client.state != stateSpawned {
		return nil
	}
	if pk.CommandOrigin.Origin != gtprotocol.CommandOriginPlayer {
		return client.writeCommandOutput(pk.CommandOrigin, appcommand.Failure("Only player command requests are supported."))
	}
	if !strings.HasPrefix(strings.TrimSpace(pk.CommandLine), "/") {
		return client.writeCommandOutput(pk.CommandOrigin, appcommand.Failure("Command requests must start with /."))
	}
	result := client.handler.commands.Dispatch(ctx, playerCommandSender{client: client}, pk.CommandLine)
	return client.writeCommandOutput(pk.CommandOrigin, result)
}

func (client *MCPEClient) sendAvailableCommands() error {
	return client.conn.WritePacket(&packet.AvailableCommands{
		Commands: client.handler.commands.ProtocolCommands(playerCommandSender{client: client}),
	})
}

func (client *MCPEClient) writeCommandOutput(origin gtprotocol.CommandOrigin, result appcommand.Result) error {
	messages := make([]gtprotocol.CommandOutputMessage, 0, len(result.Messages))
	for _, message := range result.Messages {
		messages = append(messages, gtprotocol.CommandOutputMessage{
			Success: result.Success,
			Message: message,
		})
	}
	return client.conn.WritePacket(&packet.CommandOutput{
		CommandOrigin:  origin,
		OutputType:     packet.CommandOutputTypeAllOutput,
		SuccessCount:   commandSuccessCount(result),
		OutputMessages: messages,
	})
}

func (client *MCPEClient) sendRawText(message string) error {
	return client.conn.WritePacket(&packet.Text{
		TextType: packet.TextTypeRaw,
		Message:  message,
	})
}

func (handler *MCPEHandler) broadcastSystemText(message string) error {
	return handler.broadcastText(&packet.Text{
		TextType: packet.TextTypeSystem,
		Message:  message,
	})
}

func (handler *MCPEHandler) broadcastText(pk *packet.Text) error {
	return writePacketToClients(handler.spawnedPlayers(), pk)
}

func (handler *MCPEHandler) spawnedPlayers() []*MCPEClient {
	handler.playersMu.RLock()
	defer handler.playersMu.RUnlock()

	players := make([]*MCPEClient, 0, len(handler.players))
	for _, player := range handler.players {
		if player.state == stateSpawned {
			players = append(players, player)
		}
	}
	sortClientsByRuntimeID(players)
	return players
}

func (handler *MCPEHandler) onlinePlayerNames() []string {
	players := handler.allPlayers()
	names := make([]string, 0, len(players))
	for _, player := range players {
		names = append(names, player.login.Identity.DisplayName)
	}
	sort.Strings(names)
	return names
}

func (handler *MCPEHandler) playerByName(name string) *MCPEClient {
	name = strings.ToLower(strings.TrimSpace(name))
	handler.playersMu.RLock()
	defer handler.playersMu.RUnlock()

	for _, player := range handler.players {
		if strings.ToLower(player.login.Identity.DisplayName) == name {
			return player
		}
	}
	return nil
}

func cleanChatMessage(message string) string {
	message = strings.TrimSpace(strings.ReplaceAll(message, "\x00", ""))
	if len(message) > maxChatMessageBytes {
		message = message[:maxChatMessageBytes]
	}
	return message
}

func commandSuccessCount(result appcommand.Result) uint32 {
	if result.Success {
		return 1
	}
	return 0
}

func firstArg(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}
