package command

import (
	"context"
	"testing"

	gtprotocol "github.com/sandertv/gophertunnel/minecraft/protocol"
)

type testSender struct {
	permissions map[string]bool
}

func (sender testSender) Name() string {
	return "Tester"
}

func (sender testSender) HasPermission(permission string) bool {
	return permission == "" || sender.permissions[permission]
}

func TestRegistryDispatchParsesQuotedArgumentsAndRawArgs(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(Command{
		Name:       "say",
		Permission: "bds.command.say",
		Run: func(_ context.Context, _ Sender, invocation Invocation) Result {
			if invocation.RawArgs != `"hello there" now` {
				t.Fatalf("RawArgs = %q, want quoted raw arguments", invocation.RawArgs)
			}
			if len(invocation.Args) != 2 || invocation.Args[0] != "hello there" || invocation.Args[1] != "now" {
				t.Fatalf("Args = %#v, want parsed quoted arguments", invocation.Args)
			}
			return Success("ok")
		},
	}); err != nil {
		t.Fatalf("Register() returned error: %v", err)
	}

	result := registry.Dispatch(context.Background(), testSender{permissions: map[string]bool{"bds.command.say": true}}, `/say "hello there" now`)
	if !result.Success {
		t.Fatalf("Dispatch() = %#v, want success", result)
	}
}

func TestRegistryChecksPermissionsAndExportsVisibleProtocolCommands(t *testing.T) {
	registry := NewRegistry()
	for _, cmd := range []Command{
		{Name: "help", Description: "Shows help", Permission: "bds.command.help", Run: func(context.Context, Sender, Invocation) Result { return Success("help") }},
		{
			Name:        "say",
			Description: "Broadcasts a message",
			Permission:  "bds.command.say",
			Parameters:  []Parameter{BasicParameter("message", gtprotocol.CommandArgTypeMessage, false)},
			Run:         func(context.Context, Sender, Invocation) Result { return Success("say") },
		},
	} {
		if err := registry.Register(cmd); err != nil {
			t.Fatalf("Register(%s) returned error: %v", cmd.Name, err)
		}
	}

	sender := testSender{permissions: map[string]bool{"bds.command.help": true}}
	if result := registry.Dispatch(context.Background(), sender, "/say hello"); result.Success {
		t.Fatalf("Dispatch(/say) succeeded without permission: %#v", result)
	}

	commands := registry.ProtocolCommands(sender)
	if len(commands) != 1 || commands[0].Name != "help" {
		t.Fatalf("ProtocolCommands() = %#v, want only help", commands)
	}
}
