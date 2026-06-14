package mysqlruntime

import (
	"strings"
	"testing"
)

func TestEndpointUsesFirstUsablePublishedBinding(t *testing.T) {
	container := dockerPSContainer{ID: "mysql-container-id", Names: "mysql-1", Image: "mysql:8.4"}
	inspect := dockerInspectContainer{ID: "mysql-container-id", Name: "/mysql-1"}
	inspect.NetworkSettings.Ports = map[string][]struct {
		HostIP   string `json:"HostIp"`
		HostPort string `json:"HostPort"`
	}{
		"3306/tcp": {
			{HostIP: "0.0.0.0", HostPort: ""},
			{HostIP: "127.0.0.1", HostPort: "33307"},
		},
	}

	endpoint, ok := endpointFromInspect(container, inspect)

	if !ok {
		t.Fatal("expected endpoint from second published binding")
	}
	if endpoint.Host != "127.0.0.1" || endpoint.Port != 33307 {
		t.Fatalf("endpoint binding = %s:%d", endpoint.Host, endpoint.Port)
	}
}

func TestEndpointKeepsIPv6LoopbackForIPv6WildcardBinding(t *testing.T) {
	container := dockerPSContainer{ID: "mysql-container-id", Names: "mysql-1", Image: "mysql:8.4"}
	inspect := dockerInspectContainer{ID: "mysql-container-id", Name: "/mysql-1"}
	inspect.NetworkSettings.Ports = map[string][]struct {
		HostIP   string `json:"HostIp"`
		HostPort string `json:"HostPort"`
	}{
		"3306/tcp": {
			{HostIP: "::", HostPort: "33308"},
		},
	}

	endpoint, ok := endpointFromInspect(container, inspect)

	if !ok {
		t.Fatal("expected endpoint from IPv6 published binding")
	}
	if endpoint.Host != "::1" || endpoint.Port != 33308 || endpoint.DSN != "mysql://root@[::1]:33308" {
		t.Fatalf("IPv6 endpoint = %#v", endpoint)
	}
}

func TestParseTablesPreservesCaseDistinctTableNames(t *testing.T) {
	tables := parseTables([]byte("appdb\tUsers\nappdb\tusers\nappdb\tUsers\n"))

	if len(tables) != 1 || tables[0].Name != "appdb" || strings.Join(tables[0].Tables, ",") != "Users,users" {
		t.Fatalf("case-distinct table inventory = %#v", tables)
	}
}
