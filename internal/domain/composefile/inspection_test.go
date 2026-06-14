package composefile

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseImageReferencesIgnoresNestedImageKeys(t *testing.T) {
	got := ParseImageReferences(strings.Join([]string{
		"services:",
		"  app:",
		"    image: alpine:3.20",
		"    environment:",
		"      image: not-a-service-image",
	}, "\n"))
	if !reflect.DeepEqual(got, map[string]string{"app": "alpine:3.20"}) {
		t.Fatalf("image references = %#v", got)
	}
}

func TestParseShortVolumeHandlesInterpolationAndAccessMode(t *testing.T) {
	source, target, ok := ParseShortVolume("${CONFIG_DIR:-./config}:/etc/config:ro")
	if !ok || source != "${CONFIG_DIR:-./config}" || target != "/etc/config" {
		t.Fatalf("short volume = source %q target %q ok %t", source, target, ok)
	}
}

func TestParseBindMountSourcesIncludesLongAndShortSyntax(t *testing.T) {
	got := ParseBindMountSources(strings.Join([]string{
		"services:",
		"  app:",
		"    image: alpine:3.20",
		"    volumes:",
		"      - ./config:/etc/config:ro",
		"      - type: bind",
		"        source: ./data",
		"        target: /data",
	}, "\n"))
	want := map[string][]string{"app": {"./config", "./data"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("bind mount sources = %#v", got)
	}
}
