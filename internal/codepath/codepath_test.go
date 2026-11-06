package codepath

import "testing"

func TestIsCode(t *testing.T) {
	for _, tc := range []struct {
		path string
		want bool
	}{
		{"src/a.go", true},
		{"main.go", true},
		{"go.mod", true},
		{"go.sum", true},
		{"package-lock.json", true},
		{".github/workflows/ci.yml", true},
		{".gitignore", true},
		{"Makefile", true},
		{"lyceum", true},
		{"lyceumx/a.go", true},
		{"docsite/a.go", true},
		{"src/docs/a.go", true},
		{"src/lyceum/a.go", true},
		{"README.md", false},
		{"NOTES.txt", false},
		{"src/deep/notes.md", false},
		{"src/deep/notes.txt", false},
		{".github/workflows/README.md", false},
		{"lyceum/plan/aaaaaa-x.md", false},
		{"lyceum/config.yaml", false},
		{"lyceum/events.jsonl", false},
		{"docs/x.md", false},
		{"docs/img/diagram.png", false},
	} {
		if got := IsCode(tc.path); got != tc.want {
			t.Errorf("IsCode(%q) = %v; want %v", tc.path, got, tc.want)
		}
	}
}
