package direct

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var identityPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,199}$`)
var environmentPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func normalizeBinding(binding Binding) (Binding, error) {
	if !identityPattern.MatchString(binding.ID) {
		return binding, fmt.Errorf("%w: binding id must be a safe identifier", ErrInvalid)
	}
	var err error
	binding.ProjectDir, binding.Executable, err = resolveProcessPaths(binding.ProjectDir, binding.Executable)
	if err != nil {
		return binding, err
	}
	if err := validateTargetURL(binding.TargetURL); err != nil {
		return binding, err
	}
	if binding.EvidenceDir == "" {
		return binding, fmt.Errorf("%w: evidenceDir is required", ErrInvalid)
	}
	binding.EvidenceDir, err = filepath.Abs(binding.EvidenceDir)
	if err != nil {
		return binding, err
	}
	if binding.TimeoutSeconds == 0 {
		binding.TimeoutSeconds = 60
	}
	if binding.TimeoutSeconds < 1 || binding.TimeoutSeconds > 600 {
		return binding, fmt.Errorf("%w: timeoutSeconds must be between 1 and 600", ErrInvalid)
	}
	for _, name := range append(append([]string{}, binding.EnvironmentNames...), binding.TargetEnvNames...) {
		if !environmentPattern.MatchString(name) || strings.HasPrefix(name, "AGENT_TESTBENCH_") {
			return binding, fmt.Errorf("%w: invalid or reserved environment name %q", ErrInvalid, name)
		}
	}
	return binding, nil
}

func resolveProcessPaths(project, executable string) (string, string, error) {
	project, err := filepath.EvalSymlinks(project)
	if err != nil {
		return "", "", fmt.Errorf("%w: project directory: %v", ErrInvalid, err)
	}
	project, err = filepath.Abs(project)
	if err != nil {
		return "", "", err
	}
	info, err := os.Stat(project)
	if err != nil || !info.IsDir() {
		return "", "", fmt.Errorf("%w: projectDir must be a directory", ErrInvalid)
	}
	if !filepath.IsAbs(executable) && strings.ContainsAny(executable, `/\`) {
		executable = filepath.Join(project, executable)
	}
	executable, err = exec.LookPath(executable)
	if err != nil {
		return "", "", fmt.Errorf("%w: executable: %v", ErrInvalid, err)
	}
	executable, err = filepath.Abs(executable)
	return project, executable, err
}

func validateTargetURL(value string) error {
	endpoint, err := url.Parse(value)
	if err != nil || endpoint.Hostname() == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" {
		return fmt.Errorf("%w: targetUrl must be an HTTP(S) base URL without user info, query, or fragment", ErrInvalid)
	}
	return nil
}

func sourceHashes(project string, manifest Manifest) (map[string]string, error) {
	if manifest.ProtocolVersion != ProtocolVersion || len(manifest.Cases) == 0 || len(manifest.Cases) > 1000 || len(manifest.SourceFiles) > 2000 {
		return nil, fmt.Errorf("%w: invalid adapter manifest version or size", ErrInvalid)
	}
	paths := append([]string(nil), manifest.SourceFiles...)
	seen := map[string]bool{}
	for _, item := range manifest.Cases {
		if !identityPattern.MatchString(item.ID) || seen[item.ID] || item.SourcePath == "" {
			return nil, fmt.Errorf("%w: cases need unique safe ids and sourcePath", ErrInvalid)
		}
		seen[item.ID] = true
		paths = append(paths, item.SourcePath)
	}
	out := map[string]string{}
	totalBytes := 0
	for _, path := range paths {
		if _, ok := out[path]; ok {
			continue
		}
		full, err := sourcePath(project, path)
		if err != nil {
			return nil, err
		}
		raw, err := readSourceFile(full)
		if err != nil {
			return nil, fmt.Errorf("source %q: %w", path, err)
		}
		totalBytes += len(raw)
		if totalBytes > 64<<20 {
			return nil, fmt.Errorf("%w: source snapshot exceeds the total byte limit", ErrInvalid)
		}
		out[path] = digestBytes(raw)
	}
	return out, nil
}

func sourcePath(project, relative string) (string, error) {
	if relative == "" || filepath.IsAbs(relative) || !filepath.IsLocal(relative) {
		return "", fmt.Errorf("%w: source paths must remain within projectDir", ErrInvalid)
	}
	full, err := filepath.EvalSymlinks(filepath.Join(project, relative))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(project, full)
	if err != nil || !filepath.IsLocal(rel) {
		return "", fmt.Errorf("%w: source symlink leaves projectDir", ErrInvalid)
	}
	return full, nil
}

func readSourceFile(path string) (raw []byte, err error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("source must be a regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	info, err = file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("source must be a regular file")
	}
	raw, err = io.ReadAll(io.LimitReader(file, (4<<20)+1))
	if err == nil && len(raw) > 4<<20 {
		err = fmt.Errorf("source exceeds 4 MiB")
	}
	return raw, err
}

func digestBytes(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func sourceDigest(hashes map[string]string) string {
	keys := make([]string, 0, len(hashes))
	for key := range hashes {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var encoded strings.Builder
	for _, key := range keys {
		encoded.WriteString(key + "\x00" + hashes[key] + "\x00")
	}
	return digestBytes([]byte(encoded.String()))
}

func adapterEnvironment(binding Binding) []string {
	values := map[string]string{}
	for _, name := range append([]string{"PATH", "HOME", "TMPDIR", "TEMP", "SystemRoot", "LANG"}, binding.EnvironmentNames...) {
		if value, ok := os.LookupEnv(name); ok {
			values[name] = value
		}
	}
	for _, name := range binding.TargetEnvNames {
		values[name] = binding.TargetURL
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]string, 0, len(values))
	for _, key := range keys {
		result = append(result, key+"="+values[key])
	}
	return result
}
