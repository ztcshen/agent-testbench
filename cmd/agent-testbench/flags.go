package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"strings"
)

func parseInterspersedFlags(flags *flag.FlagSet, args []string) error {
	return flags.Parse(interspersedFlagArgs(flags, args))
}

func interspersedFlagArgs(flags *flag.FlagSet, args []string) []string {
	flagArgs := make([]string, 0, len(args))
	positional := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if !looksLikeFlagToken(arg) {
			positional = append(positional, arg)
			continue
		}
		flagArgs = append(flagArgs, arg)
		name, inlineValue := flagTokenName(arg)
		defined := flags.Lookup(name)
		if defined == nil || inlineValue || isBoolFlagValue(defined.Value) || i+1 >= len(args) {
			continue
		}
		i++
		flagArgs = append(flagArgs, args[i])
	}
	return append(flagArgs, positional...)
}

func looksLikeFlagToken(arg string) bool {
	return strings.HasPrefix(arg, "-") && arg != "-"
}

func flagTokenName(arg string) (string, bool) {
	name := strings.TrimLeft(arg, "-")
	if before, _, ok := strings.Cut(name, "="); ok {
		return before, true
	}
	return name, false
}

func isBoolFlagValue(value flag.Value) bool {
	boolValue, ok := value.(interface {
		IsBoolFlag() bool
	})
	return ok && boolValue.IsBoolFlag()
}

type mapFlag map[string]any

func (m *mapFlag) String() string {
	if m == nil || len(*m) == 0 {
		return ""
	}
	raw, _ := json.Marshal(*m)
	return string(raw)
}

func (m *mapFlag) Set(value string) error {
	key, parsed, ok := strings.Cut(value, "=")
	key = strings.TrimSpace(key)
	if !ok || key == "" {
		return fmt.Errorf("override must use key=value")
	}
	if *m == nil {
		*m = map[string]any{}
	}
	(*m)[key] = parsed
	return nil
}

func (m mapFlag) Values() map[string]any {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]any, len(m))
	for key, value := range m {
		out[key] = value
	}
	return out
}

type stringListFlag []string

func (s *stringListFlag) String() string {
	if s == nil {
		return ""
	}
	return strings.Join(*s, ",")
}

func (s *stringListFlag) Set(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	*s = append(*s, value)
	return nil
}

func (s stringListFlag) Values() []string {
	return normalizeStringList([]string(s))
}

func normalizeStringList(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, value)
	}
	return out
}
