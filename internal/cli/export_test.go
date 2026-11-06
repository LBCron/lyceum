package cli

import "testing"

// TestHasSequenceDiagram_AC81: only a sequenceDiagram inside a ```mermaid
// fenced block counts.
func TestHasSequenceDiagram_AC81(t *testing.T) {
	for _, c := range []struct {
		body string
		want bool
	}{
		{"```mermaid\nsequenceDiagram\n  A->>B: hi\n```\n", true},
		{"````mermaid\nsequenceDiagram\n```\nstill in\n````\n", true},
		{"```mermaid\nflowchart LR\n  A-->B\n```\n", false},
		{"sequenceDiagram\n", false},
		{"```text\nsequenceDiagram\n```\n", false},
		{"```mermaid\nflowchart LR\n```\nsequenceDiagram\n", false},
		{"no diagram\n", false},
	} {
		if got := hasSequenceDiagram([]byte(c.body)); got != c.want {
			t.Errorf("hasSequenceDiagram(%q) = %v; want %v", c.body, got, c.want)
		}
	}
}
