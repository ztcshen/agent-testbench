package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

func (s *Store) exec(ctx context.Context, statement string) error {
	out, err := sqliteCommand(ctx, false, s.path, statement)
	if err != nil {
		return fmt.Errorf("run sqlite statement: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func stringDefault(value string, defaultValue string) string {
	if strings.TrimSpace(value) == "" {
		return defaultValue
	}
	return value
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func jsonString(value any, defaultValue string) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return defaultValue
	}
	return string(raw)
}

func stringSliceFromJSON(raw string) []string {
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return []string{}
	}
	return out
}

func firstNonZero(value int, defaultValue int) int {
	if value != 0 {
		return value
	}
	return defaultValue
}

func (s *Store) query(ctx context.Context, statement string, target any) error {
	out, err := sqliteCommand(ctx, true, s.path, statement)
	if err != nil {
		return fmt.Errorf("run sqlite query: %w: %s", err, strings.TrimSpace(string(out)))
	}
	if len(strings.TrimSpace(string(out))) == 0 {
		out = []byte("[]")
	}
	if err := json.Unmarshal(out, target); err != nil {
		return fmt.Errorf("decode sqlite query result: %w", err)
	}
	return nil
}

func sqliteCommand(ctx context.Context, jsonOutput bool, path string, statement string) ([]byte, error) {
	args := []string{"-cmd", ".timeout 5000"}
	if jsonOutput {
		args = append(args, "-json")
	}
	args = append(args, path, "PRAGMA foreign_keys = ON;\n"+statement)
	cmd := exec.CommandContext(ctx, "sqlite3", args...)
	return cmd.CombinedOutput()
}

func sqlString(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func encodeTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func decodeTime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}
	}
	return t
}

func normalizeJSONText(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return value
	}
	var decoded any
	if err := json.Unmarshal([]byte(value), &decoded); err != nil {
		return value
	}
	encoded, err := json.Marshal(decoded)
	if err != nil {
		return value
	}
	return string(encoded)
}

func utcNow() time.Time {
	return time.Now().UTC()
}
