// Package mysqlruntime inspects local Docker and process metadata for MySQL runtime endpoints.
package mysqlruntime

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

const passwordMask = "xxxxx"

type Report struct {
	OK    bool       `json:"ok"`
	Count int        `json:"count"`
	Items []Endpoint `json:"items"`
}

type Endpoint struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	ContainerName  string     `json:"containerName"`
	Image          string     `json:"image"`
	Host           string     `json:"host"`
	Port           int        `json:"port"`
	User           string     `json:"user"`
	PasswordMasked string     `json:"passwordMasked,omitempty"`
	Database       string     `json:"database,omitempty"`
	DSN            string     `json:"dsn"`
	Databases      []Database `json:"databases,omitempty"`
	Warnings       []string   `json:"warnings,omitempty"`
}

type Database struct {
	Name   string   `json:"name"`
	Tables []string `json:"tables"`
}

type dockerPSContainer struct {
	ID    string `json:"ID"`
	Names string `json:"Names"`
	Image string `json:"Image"`
	Ports string `json:"Ports"`
}

type dockerInspectContainer struct {
	ID     string `json:"Id"`
	Name   string `json:"Name"`
	Config struct {
		Image string   `json:"Image"`
		Env   []string `json:"Env"`
	} `json:"Config"`
	NetworkSettings struct {
		Ports map[string][]struct {
			HostIP   string `json:"HostIp"`
			HostPort string `json:"HostPort"`
		} `json:"Ports"`
	} `json:"NetworkSettings"`
}

func DiscoverEndpoints(ctx context.Context, includeTables bool) (Report, error) {
	containers, err := dockerPS(ctx)
	if err != nil {
		return Report{}, err
	}
	report := Report{OK: true, Items: []Endpoint{}}
	for _, container := range containers {
		if !looksLikeMySQLContainer(container) {
			continue
		}
		inspect, err := dockerInspect(ctx, container.ID)
		if err != nil {
			continue
		}
		endpoint, ok := endpointFromInspect(container, inspect)
		if !ok {
			continue
		}
		if includeTables {
			databases, err := containerTables(ctx, endpoint.ID)
			if err != nil {
				endpoint.Warnings = append(endpoint.Warnings, "table inventory unavailable")
			} else {
				endpoint.Databases = databases
			}
		}
		report.Items = append(report.Items, endpoint)
	}
	report.Count = len(report.Items)
	return report, nil
}

func dockerPS(ctx context.Context) ([]dockerPSContainer, error) {
	cmd := exec.CommandContext(ctx, "docker", "ps", "--format", "{{json .}}")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("docker ps: %w", err)
	}
	scanner := bufio.NewScanner(bytes.NewReader(out))
	containers := []dockerPSContainer{}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var container dockerPSContainer
		if err := json.Unmarshal([]byte(line), &container); err != nil {
			return nil, fmt.Errorf("decode docker ps row: %w", err)
		}
		containers = append(containers, container)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read docker ps output: %w", err)
	}
	return containers, nil
}

func dockerInspect(ctx context.Context, containerID string) (dockerInspectContainer, error) {
	cmd := exec.CommandContext(ctx, "docker", "inspect", containerID)
	out, err := cmd.Output()
	if err != nil {
		return dockerInspectContainer{}, fmt.Errorf("docker inspect %s: %w", containerID, err)
	}
	var inspected []dockerInspectContainer
	if err := json.Unmarshal(out, &inspected); err != nil {
		return dockerInspectContainer{}, fmt.Errorf("decode docker inspect %s: %w", containerID, err)
	}
	if len(inspected) == 0 {
		return dockerInspectContainer{}, fmt.Errorf("docker inspect %s returned no containers", containerID)
	}
	return inspected[0], nil
}

func looksLikeMySQLContainer(container dockerPSContainer) bool {
	haystack := strings.ToLower(strings.Join([]string{container.Names, container.Image, container.Ports}, " "))
	return strings.Contains(haystack, "mysql") || strings.Contains(haystack, "mariadb") || strings.Contains(haystack, "3306/tcp")
}

func endpointFromInspect(container dockerPSContainer, inspected dockerInspectContainer) (Endpoint, bool) {
	published := inspected.NetworkSettings.Ports["3306/tcp"]
	if len(published) == 0 {
		return Endpoint{}, false
	}
	host, port, ok := firstPublishedMySQLBinding(published)
	if !ok {
		return Endpoint{}, false
	}
	env := envMap(inspected.Config.Env)
	user := firstNonEmpty(env["MYSQL_USER"], env["MARIADB_USER"], "root")
	database := firstNonEmpty(env["MYSQL_DATABASE"], env["MARIADB_DATABASE"])
	hasPassword := firstNonEmpty(env["MYSQL_PASSWORD"], env["MARIADB_PASSWORD"], env["MYSQL_ROOT_PASSWORD"], env["MARIADB_ROOT_PASSWORD"]) != ""
	masked := ""
	if hasPassword {
		masked = passwordMask
	}
	image := firstNonEmpty(inspected.Config.Image, container.Image)
	name := strings.TrimPrefix(firstNonEmpty(inspected.Name, container.Names), "/")
	id := firstNonEmpty(inspected.ID, container.ID)
	endpoint := Endpoint{
		ID:             id,
		Name:           name,
		ContainerName:  name,
		Image:          image,
		Host:           host,
		Port:           port,
		User:           user,
		PasswordMasked: masked,
		Database:       database,
	}
	endpoint.DSN = maskedDSN(endpoint)
	return endpoint, true
}

func firstPublishedMySQLBinding(published []struct {
	HostIP   string `json:"HostIp"`
	HostPort string `json:"HostPort"`
}) (string, int, bool) {
	for _, binding := range published {
		port, err := strconv.Atoi(strings.TrimSpace(binding.HostPort))
		if err != nil || port <= 0 {
			continue
		}
		return normalizePublishedHost(binding.HostIP), port, true
	}
	return "", 0, false
}

func normalizePublishedHost(host string) string {
	host = strings.TrimSpace(host)
	switch host {
	case "", "0.0.0.0":
		return "127.0.0.1"
	case "::":
		return "::1"
	default:
		return host
	}
}

func envMap(values []string) map[string]string {
	out := map[string]string{}
	for _, value := range values {
		key, val, ok := strings.Cut(value, "=")
		if !ok {
			continue
		}
		out[key] = val
	}
	return out
}

func maskedDSN(endpoint Endpoint) string {
	dsn := url.URL{Scheme: "mysql", Host: net.JoinHostPort(endpoint.Host, strconv.Itoa(endpoint.Port))}
	if endpoint.PasswordMasked != "" {
		dsn.User = url.UserPassword(endpoint.User, endpoint.PasswordMasked)
	} else {
		dsn.User = url.User(endpoint.User)
	}
	if endpoint.Database != "" {
		dsn.Path = "/" + endpoint.Database
	}
	return dsn.String()
}

func containerTables(ctx context.Context, containerID string) ([]Database, error) {
	const script = `
set -eu
query="select table_schema, table_name from information_schema.tables where table_schema not in ('information_schema','mysql','performance_schema','sys') order by table_schema, table_name"
mysql_user="${MYSQL_USER:-${MARIADB_USER:-root}}"
mysql_password="${MYSQL_PASSWORD:-${MARIADB_PASSWORD:-}}"
root_password="${MYSQL_ROOT_PASSWORD:-${MARIADB_ROOT_PASSWORD:-}}"
if [ "$mysql_user" = "root" ] && [ -z "$mysql_password" ]; then
  mysql_password="$root_password"
fi
run_query() {
  user="$1"
  password="$2"
  if [ -n "$password" ]; then
    MYSQL_PWD="$password" mysql -N -B -u"$user" -e "$query"
  else
    mysql -N -B -u"$user" -e "$query"
  fi
}
if run_query "$mysql_user" "$mysql_password"; then
  exit 0
fi
if [ "$mysql_user" != "root" ] && [ -n "$root_password" ]; then
  run_query root "$root_password"
  exit $?
fi
exit 1
`
	cmd := exec.CommandContext(ctx, "docker", "exec", containerID, "sh", "-lc", script)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return parseTables(out), nil
}

func parseTables(out []byte) []Database {
	tablesByDatabase := map[string][]string{}
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 2 {
			fields = strings.Fields(line)
		}
		if len(fields) < 2 {
			continue
		}
		database := strings.TrimSpace(fields[0])
		table := strings.TrimSpace(fields[1])
		if database == "" || table == "" {
			continue
		}
		tablesByDatabase[database] = append(tablesByDatabase[database], table)
	}
	names := make([]string, 0, len(tablesByDatabase))
	for name := range tablesByDatabase {
		names = append(names, name)
	}
	sort.Strings(names)
	databases := make([]Database, 0, len(names))
	for _, name := range names {
		tables := uniqueSortedStrings(tablesByDatabase[name])
		databases = append(databases, Database{Name: name, Tables: tables})
	}
	return databases
}

func uniqueSortedStrings(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
