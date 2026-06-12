// Package apicasecommand builds user-facing commands for API Case follow-up work.
package apicasecommand

import (
	"strconv"
	"strings"

	"agent-testbench/internal/domain/profile"
)

func SuggestedRunCommand(item profile.APICase) string {
	casePath := strings.TrimSpace(item.CasePath)
	if casePath == "" {
		return ""
	}
	parts := []string{"agent-testbench case run --case " + strconv.Quote(casePath)}
	parts = appendFlag(parts, "--base-url", item.BaseURL)
	parts = appendFlag(parts, "--evidence-dir", item.EvidenceDir)
	return strings.Join(parts, " ")
}

func appendFlag(parts []string, flag string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return parts
	}
	return append(parts, flag+" "+strconv.Quote(value))
}
