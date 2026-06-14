// Package commandline formats command strings shown to AgentTestBench users.
package commandline

import "strings"

func ShellQuote(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return `''`
	}
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
