package environmentfiles

import "strings"

// InterpolateComposeText resolves Compose-style variable expressions from env.
func InterpolateComposeText(value string, env map[string]string) (string, bool) {
	return interpolateComposeText(value, env, 0)
}

func interpolateComposeText(value string, env map[string]string, depth int) (string, bool) {
	if depth > 8 {
		return value, false
	}
	var out strings.Builder
	for i := 0; i < len(value); {
		if value[i] != '$' {
			out.WriteByte(value[i])
			i++
			continue
		}
		if i+1 >= len(value) {
			out.WriteByte(value[i])
			i++
			continue
		}
		if value[i+1] == '$' {
			out.WriteByte('$')
			i += 2
			continue
		}
		replacement, next, ok := composeInterpolationReplacement(value, i, env, depth)
		if !ok {
			return value, false
		}
		if next == i {
			out.WriteByte(value[i])
			i++
			continue
		}
		out.WriteString(replacement)
		i = next
	}
	return out.String(), true
}

func composeInterpolationReplacement(value string, start int, env map[string]string, depth int) (string, int, bool) {
	if value[start+1] == '{' {
		return composeBracedInterpolationReplacement(value, start, env, depth)
	}
	nameEnd := start + 1
	for nameEnd < len(value) && composeVariableNameByte(value[nameEnd]) {
		nameEnd++
	}
	if nameEnd == start+1 {
		return "", start, true
	}
	replacement, ok := env[value[start+1:nameEnd]]
	return replacement, nameEnd, ok
}

func composeBracedInterpolationReplacement(value string, start int, env map[string]string, depth int) (string, int, bool) {
	end := composeExpressionEnd(value, start+2)
	if end < 0 {
		return "", start, false
	}
	replacement, ok := resolveComposeVariableExpression(value[start+2:end], env)
	if !ok {
		return "", start, false
	}
	if strings.Contains(replacement, "$") {
		replacement, ok = interpolateComposeText(replacement, env, depth+1)
		if !ok {
			return "", start, false
		}
	}
	return replacement, end + 1, true
}

func composeExpressionEnd(value string, start int) int {
	depth := 0
	for i := start; i < len(value); i++ {
		if value[i] == '$' && i+1 < len(value) && value[i+1] == '{' {
			depth++
			i++
			continue
		}
		if value[i] != '}' {
			continue
		}
		if depth == 0 {
			return i
		}
		depth--
	}
	return -1
}

func resolveComposeVariableExpression(expr string, env map[string]string) (string, bool) {
	nameEnd := 0
	for nameEnd < len(expr) && composeVariableNameByte(expr[nameEnd]) {
		nameEnd++
	}
	if nameEnd == 0 {
		return "", false
	}
	name := expr[:nameEnd]
	opArg := expr[nameEnd:]
	value, exists := env[name]
	switch {
	case opArg == "":
		return value, exists
	case strings.HasPrefix(opArg, ":-"):
		if !exists || value == "" {
			return opArg[2:], true
		}
		return value, true
	case strings.HasPrefix(opArg, "-"):
		if !exists {
			return opArg[1:], true
		}
		return value, true
	case strings.HasPrefix(opArg, ":?"):
		return value, exists && value != ""
	case strings.HasPrefix(opArg, "?"):
		return value, exists
	case strings.HasPrefix(opArg, ":+"):
		if exists && value != "" {
			return opArg[2:], true
		}
		return "", true
	case strings.HasPrefix(opArg, "+"):
		if exists {
			return opArg[1:], true
		}
		return "", true
	default:
		return "", false
	}
}

func composeVariableNameByte(value byte) bool {
	return value == '_' || value >= '0' && value <= '9' || value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z'
}

func projectionComposeEnv(compose map[string]any, generated map[string]string, assetContent map[string]string) map[string]string {
	env := stringMap(compose["env"])
	for _, envFile := range stringSlice(compose["envFiles"]) {
		content := generated[cleanPath(envFile)]
		if content == "" {
			content = assetContent[cleanPath(envFile)]
		}
		if content == "" {
			continue
		}
		for key, value := range parseComposeEnvFile(content) {
			env[key] = value
		}
	}
	return env
}

func parseComposeEnvFile(content string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(content, "\n") {
		key, value, ok := parseComposeEnvLine(line)
		if ok {
			out[key] = value
		}
	}
	return out
}

func parseComposeEnvLine(line string) (string, string, bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false
	}
	line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
	sep := strings.IndexByte(line, '=')
	if sep <= 0 {
		return "", "", false
	}
	key := strings.TrimSpace(line[:sep])
	if !composeEnvKey(key) {
		return "", "", false
	}
	return key, cleanComposeEnvValue(line[sep+1:]), true
}

func composeEnvKey(key string) bool {
	if key == "" {
		return false
	}
	for i := 0; i < len(key); i++ {
		if !composeVariableNameByte(key[i]) {
			return false
		}
	}
	return true
}

func cleanComposeEnvValue(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'')) {
		return strings.Trim(value[1:len(value)-1], "\r")
	}
	if hash := strings.Index(value, " #"); hash >= 0 {
		value = strings.TrimSpace(value[:hash])
	}
	return strings.Trim(value, "\r")
}
