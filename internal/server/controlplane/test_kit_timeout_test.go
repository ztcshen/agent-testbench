package controlplane

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestTestKitTimeoutUsesBoundedIntegerSeconds(t *testing.T) {
	for _, test := range []struct {
		name       string
		payload    map[string]any
		configured int
		want       time.Duration
	}{
		{name: "default", payload: map[string]any{}, want: 90 * time.Second},
		{name: "explicit zero uses default", payload: map[string]any{"timeoutSeconds": json.Number("0")}, configured: 30, want: 90 * time.Second},
		{name: "explicit maximum", payload: map[string]any{"timeoutSeconds": json.Number("600")}, want: 600 * time.Second},
		{name: "catalog configured", payload: map[string]any{}, configured: 30, want: 30 * time.Second},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := testKitTimeout(test.payload, test.configured)
			if err != nil || got != test.want {
				t.Fatalf("testKitTimeout() = %s, %v; want %s", got, err, test.want)
			}
		})
	}
}

func TestTestKitTimeoutRejectsUnsafeValues(t *testing.T) {
	for _, test := range []struct {
		name       string
		payload    map[string]any
		configured int
	}{
		{name: "negative", payload: map[string]any{"timeoutSeconds": json.Number("-1")}},
		{name: "over maximum", payload: map[string]any{"timeoutSeconds": json.Number("601")}},
		{name: "integer overflow", payload: map[string]any{"timeoutSeconds": json.Number("9223372036854775808")}},
		{name: "exponent overflow", payload: map[string]any{"timeoutSeconds": json.Number("1e100")}},
		{name: "fractional", payload: map[string]any{"timeoutSeconds": json.Number("1.5")}},
		{name: "catalog over maximum", payload: map[string]any{}, configured: 601},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := testKitTimeout(test.payload, test.configured); err == nil || !strings.Contains(err.Error(), "timeoutSeconds") {
				t.Fatalf("testKitTimeout() error = %v", err)
			}
		})
	}
}
