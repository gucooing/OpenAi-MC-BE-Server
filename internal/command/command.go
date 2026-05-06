package command

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	gtprotocol "github.com/sandertv/gophertunnel/minecraft/protocol"
)

const noAliases = ^uint32(0)

type Sender interface {
	Name() string
	HasPermission(string) bool
}

type Invocation struct {
	Label   string
	Args    []string
	RawArgs string
}

type Result struct {
	Success  bool
	Messages []string
}

type Parameter struct {
	Name     string
	Type     uint32
	Optional bool
	Options  byte
}

type Command struct {
	Name        string
	Description string
	Usage       string
	Permission  string
	Aliases     []string
	Parameters  []Parameter
	Run         func(context.Context, Sender, Invocation) Result
}

type Registry struct {
	mu       sync.RWMutex
	commands map[string]Command
	aliases  map[string]string
	order    []string
}

func NewRegistry() *Registry {
	return &Registry{
		commands: make(map[string]Command),
		aliases:  make(map[string]string),
	}
}

func (registry *Registry) Register(cmd Command) error {
	cmd.Name = normaliseName(cmd.Name)
	if cmd.Name == "" {
		return fmt.Errorf("command name cannot be empty")
	}
	if cmd.Run == nil {
		return fmt.Errorf("command %q has no executor", cmd.Name)
	}
	if cmd.Usage == "" {
		cmd.Usage = "/" + cmd.Name
	}

	registry.mu.Lock()
	defer registry.mu.Unlock()

	if _, ok := registry.commands[cmd.Name]; ok {
		return fmt.Errorf("command %q is already registered", cmd.Name)
	}
	for _, alias := range cmd.Aliases {
		alias = normaliseName(alias)
		if alias == "" {
			continue
		}
		if _, ok := registry.commands[alias]; ok {
			return fmt.Errorf("command alias %q conflicts with a command", alias)
		}
		if _, ok := registry.aliases[alias]; ok {
			return fmt.Errorf("command alias %q is already registered", alias)
		}
	}

	registry.commands[cmd.Name] = cmd
	registry.order = append(registry.order, cmd.Name)
	sort.Strings(registry.order)
	for _, alias := range cmd.Aliases {
		if alias = normaliseName(alias); alias != "" {
			registry.aliases[alias] = cmd.Name
		}
	}
	return nil
}

func (registry *Registry) Dispatch(ctx context.Context, sender Sender, line string) Result {
	label, args, rawArgs, err := ParseLine(line)
	if err != nil {
		return Failure(err.Error())
	}
	if label == "" {
		return Failure("No command was entered.")
	}

	cmd, ok := registry.Lookup(label)
	if !ok {
		return Failure("Unknown command. Try /help for a list of commands.")
	}
	if !sender.HasPermission(cmd.Permission) {
		return Failure("You do not have permission to use this command.")
	}

	result := cmd.Run(ctx, sender, Invocation{
		Label:   label,
		Args:    args,
		RawArgs: rawArgs,
	})
	if len(result.Messages) == 0 {
		if result.Success {
			result.Messages = []string{"Command executed."}
		} else {
			result.Messages = []string{"Command failed."}
		}
	}
	return result
}

func (registry *Registry) Lookup(name string) (Command, bool) {
	name = normaliseName(name)

	registry.mu.RLock()
	defer registry.mu.RUnlock()

	if canonical, ok := registry.aliases[name]; ok {
		name = canonical
	}
	cmd, ok := registry.commands[name]
	return cmd, ok
}

func (registry *Registry) Visible(sender Sender) []Command {
	registry.mu.RLock()
	defer registry.mu.RUnlock()

	commands := make([]Command, 0, len(registry.order))
	for _, name := range registry.order {
		cmd := registry.commands[name]
		if sender.HasPermission(cmd.Permission) {
			commands = append(commands, cmd)
		}
	}
	return commands
}

func (registry *Registry) ProtocolCommands(sender Sender) []gtprotocol.Command {
	visible := registry.Visible(sender)
	commands := make([]gtprotocol.Command, 0, len(visible))
	for _, cmd := range visible {
		parameters := make([]gtprotocol.CommandParameter, 0, len(cmd.Parameters))
		for _, parameter := range cmd.Parameters {
			parameters = append(parameters, gtprotocol.CommandParameter{
				Name:     parameter.Name,
				Type:     parameter.Type,
				Optional: parameter.Optional,
				Options:  parameter.Options,
			})
		}
		overloads := []gtprotocol.CommandOverload{{Parameters: parameters}}
		commands = append(commands, gtprotocol.Command{
			Name:            cmd.Name,
			Description:     cmd.Description,
			PermissionLevel: gtprotocol.CommandPermissionLevelAny,
			AliasesOffset:   noAliases,
			Overloads:       overloads,
		})
	}
	return commands
}

func ParseLine(line string) (label string, args []string, rawArgs string, err error) {
	line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "/"))
	if line == "" {
		return "", nil, "", nil
	}

	labelEnd := len(line)
	for i, r := range line {
		if r == ' ' || r == '\t' {
			labelEnd = i
			break
		}
	}
	label = normaliseName(line[:labelEnd])
	rawArgs = strings.TrimSpace(line[labelEnd:])
	args, err = splitArgs(rawArgs)
	if err != nil {
		return "", nil, "", err
	}
	return label, args, rawArgs, nil
}

func BasicParameter(name string, argType uint32, optional bool) Parameter {
	return Parameter{
		Name:     name,
		Type:     gtprotocol.CommandArgValid | argType,
		Optional: optional,
	}
}

func Success(messages ...string) Result {
	return Result{Success: true, Messages: compactMessages(messages)}
}

func Failure(messages ...string) Result {
	return Result{Success: false, Messages: compactMessages(messages)}
}

func normaliseName(name string) string {
	return strings.ToLower(strings.TrimSpace(strings.TrimPrefix(name, "/")))
}

func compactMessages(messages []string) []string {
	out := make([]string, 0, len(messages))
	for _, message := range messages {
		message = strings.TrimSpace(message)
		if message != "" {
			out = append(out, message)
		}
	}
	return out
}

func splitArgs(value string) ([]string, error) {
	var args []string
	var builder strings.Builder
	inQuote := rune(0)
	escaped := false
	tokenOpen := false

	for _, r := range value {
		if escaped {
			builder.WriteRune(r)
			escaped = false
			tokenOpen = true
			continue
		}
		if r == '\\' && inQuote != 0 {
			escaped = true
			continue
		}
		if inQuote != 0 {
			if r == inQuote {
				inQuote = 0
				continue
			}
			builder.WriteRune(r)
			tokenOpen = true
			continue
		}
		switch r {
		case '\'', '"':
			inQuote = r
			tokenOpen = true
		case ' ', '\t':
			if tokenOpen {
				args = append(args, builder.String())
				builder.Reset()
				tokenOpen = false
			}
		default:
			builder.WriteRune(r)
			tokenOpen = true
		}
	}
	if escaped {
		builder.WriteRune('\\')
	}
	if inQuote != 0 {
		return nil, fmt.Errorf("unterminated quoted argument")
	}
	if tokenOpen {
		args = append(args, builder.String())
	}
	return args, nil
}
