package controlplane

import (
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func requestSigningAuthorization(method string, uri string, body string, auth map[string]string) (string, error) {
	timestamp := time.Now().Unix()
	nonce := requestNonce()
	payload := fmt.Sprintf("%s\n%s\n%d\n%s\n\n%s\n", method, uri, timestamp, nonce, body)
	signature, err := signRequestPayload(payload, auth)
	if err != nil {
		return "", err
	}
	fields := signingHeaderFields(auth)
	fields["nonce_str"] = nonce
	fields["signature"] = signature
	fields["timestamp"] = fmt.Sprintf("%d", timestamp)
	if len(fields) < 3 {
		return "", errors.New("signed case requires auth fields")
	}
	ordered := []string{"credential_id", "mch_id", "nonce_str", "signature", "timestamp", "serial_no"}
	for key := range fields {
		if !stringInList(ordered, key) {
			ordered = append(ordered, key)
		}
	}
	parts := make([]string, 0, len(fields))
	for _, key := range ordered {
		if value := fields[key]; value != "" {
			parts = append(parts, fmt.Sprintf(`%s="%s"`, key, value))
		}
	}
	return "RSA " + strings.Join(parts, ","), nil
}

func signingHeaderFields(auth map[string]string) map[string]string {
	fields := map[string]string{}
	for key, value := range auth {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" || signingSecretKey(key) {
			continue
		}
		fields[snakeCase(key)] = value
	}
	return fields
}

func signingSecretKey(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "keypath", "pfxpath", "pfxpassword", "scheme":
		return true
	default:
		return false
	}
}

func signRequestPayload(payload string, auth map[string]string) (string, error) {
	keyPath, err := requestSigningKeyPath(auth)
	if err != nil {
		return "", err
	}
	cmd := exec.Command("openssl", "dgst", "-sha256", "-sign", keyPath)
	cmd.Stdin = strings.NewReader(payload)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("openssl sign failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return strings.ToUpper(fmt.Sprintf("%x", stdout.Bytes())), nil
}

func requestSigningKeyPath(auth map[string]string) (string, error) {
	if keyPath := firstNonEmpty(auth["keyPath"], os.Getenv("SANDBOX_SIGN_KEY_PATH")); keyPath != "" {
		return keyPath, nil
	}
	keyPath := filepath.Join(runtimeProjectRoot(), ".runtime", "control-plane", "request_signing_key.pem")
	if _, err := os.Stat(keyPath); err == nil {
		return keyPath, nil
	}
	if err := os.MkdirAll(filepath.Dir(keyPath), 0o700); err != nil {
		return "", err
	}
	pfxPath := firstNonEmpty(auth["pfxPath"], os.Getenv("SANDBOX_SIGN_PFX_PATH"))
	pfxPassword := firstNonEmpty(auth["pfxPassword"], os.Getenv("SANDBOX_SIGN_PFX_PASSWORD"))
	if pfxPath == "" || pfxPassword == "" {
		return "", errors.New("signed case requires auth.keyPath or auth.pfxPath/auth.pfxPassword")
	}
	if _, err := os.Stat(pfxPath); err != nil {
		return "", fmt.Errorf("signing pfx not found: %s", pfxPath)
	}
	var lastErr error
	for _, legacy := range []bool{true, false} {
		args := []string{"pkcs12"}
		if legacy {
			args = append(args, "-legacy")
		}
		args = append(args, "-in", pfxPath, "-nocerts", "-nodes", "-password", "pass:"+pfxPassword)
		pkcs12 := exec.Command("openssl", args...)
		pkey := exec.Command("openssl", "pkey", "-out", keyPath)
		pipe, err := pkcs12.StdoutPipe()
		if err != nil {
			return "", err
		}
		pkey.Stdin = pipe
		var pkcs12Err bytes.Buffer
		var pkeyErr bytes.Buffer
		pkcs12.Stderr = &pkcs12Err
		pkey.Stderr = &pkeyErr
		if err := pkey.Start(); err != nil {
			return "", err
		}
		if err := pkcs12.Start(); err != nil {
			pkeyWaitErr := pkey.Wait()
			if pkeyWaitErr != nil {
				return "", fmt.Errorf("start openssl pkcs12: %w; wait pkey: %v", err, pkeyWaitErr)
			}
			return "", err
		}
		pkcs12WaitErr := pkcs12.Wait()
		pkeyWaitErr := pkey.Wait()
		if pkcs12WaitErr == nil && pkeyWaitErr == nil {
			if err := os.Chmod(keyPath, 0o600); err != nil {
				return "", err
			}
			return keyPath, nil
		}
		lastErr = fmt.Errorf("extract key failed legacy=%v pkcs12=%v pkey=%v pkcs12_err=%s pkey_err=%s", legacy, pkcs12WaitErr, pkeyWaitErr, strings.TrimSpace(pkcs12Err.String()), strings.TrimSpace(pkeyErr.String()))
	}
	if err := os.Remove(keyPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	return "", lastErr
}

func runtimeProjectRoot() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	dir := cwd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return cwd
		}
		dir = parent
	}
}

func requestNonce() string {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return serialValue("")[:16]
	}
	for i, value := range raw {
		raw[i] = alphabet[int(value)%len(alphabet)]
	}
	return string(raw)
}

func snakeCase(value string) string {
	var out strings.Builder
	for index, item := range value {
		if item >= 'A' && item <= 'Z' {
			if index > 0 {
				out.WriteByte('_')
			}
			out.WriteRune(item + ('a' - 'A'))
			continue
		}
		if item == '-' || item == ' ' {
			out.WriteByte('_')
			continue
		}
		out.WriteRune(item)
	}
	return out.String()
}

func stringInList(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
