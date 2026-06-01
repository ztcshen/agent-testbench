package main

import (
	"path/filepath"
	"strings"
)

func environmentRestoreComposeWithRepoCheckouts(compose map[string]any, specs []environmentRestoreRepoSpec) map[string]any {
	replacements := environmentRestoreRepoCheckoutReplacements(specs)
	if len(replacements) == 0 {
		return compose
	}
	generated := stringMapFromAny(compose["generatedFiles"])
	if len(generated) == 0 {
		return compose
	}
	rewritten := make(map[string]string, len(generated))
	changed := false
	for path, content := range generated {
		next, ok := environmentRestoreRewriteComposeHostBindSources(content, replacements)
		rewritten[path] = next
		changed = changed || ok
	}
	if !changed {
		return compose
	}
	out := jsonObjectFromAny(compose)
	out["generatedFiles"] = rewritten
	return out
}

func environmentRestoreRepoCheckoutReplacements(specs []environmentRestoreRepoSpec) map[string]string {
	out := map[string]string{}
	for _, spec := range specs {
		checkout := strings.TrimSpace(spec.Checkout)
		if checkout == "" || !filepath.IsAbs(checkout) {
			continue
		}
		if id := strings.TrimSpace(spec.ServiceID); id != "" {
			out[id] = checkout
		}
		if base := filepath.Base(filepath.Clean(checkout)); base != "." && base != "" {
			out[base] = checkout
		}
	}
	return out
}

func environmentRestoreRewriteComposeHostBindSources(content string, replacements map[string]string) (string, bool) {
	lines := strings.Split(content, "\n")
	state := composeBindMountParseState{}
	changed := false
	for index, line := range lines {
		_, source := state.bindSource(line)
		replacement := environmentRestoreRepoCheckoutForBindSource(source, replacements)
		if replacement == "" || replacement == source {
			continue
		}
		next := strings.Replace(line, source, replacement, 1)
		if next != line {
			lines[index] = next
			changed = true
		}
	}
	if !changed {
		return content, false
	}
	return strings.Join(lines, "\n"), true
}

func environmentRestoreRepoCheckoutForBindSource(source string, replacements map[string]string) string {
	source = filepath.Clean(strings.TrimSpace(source))
	if source == "" || !filepath.IsAbs(source) {
		return ""
	}
	if checkout := replacements[filepath.Base(source)]; checkout != "" {
		return checkout
	}
	parts := strings.Split(source, string(filepath.Separator))
	for index, part := range parts {
		checkout := replacements[part]
		if checkout == "" {
			continue
		}
		if index+1 >= len(parts) {
			return checkout
		}
		suffixParts := environmentRestoreNonEmptyPathParts(parts[index+1:])
		if len(suffixParts) == 0 {
			return checkout
		}
		return filepath.Join(append([]string{checkout}, suffixParts...)...)
	}
	return ""
}

func environmentRestoreNonEmptyPathParts(parts []string) []string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			out = append(out, part)
		}
	}
	return out
}
