package commandline

import "testing"

func TestShellQuote(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "empty", value: " ", want: `''`},
		{name: "simple", value: "run-1", want: "'run-1'"},
		{name: "embedded quote", value: "step 'submit'", want: "'step '\\''submit'\\'''"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ShellQuote(tt.value); got != tt.want {
				t.Fatalf("ShellQuote(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}
