package cli

import (
	"strings"
	"testing"
)

const codexGit = "/repo/.git"

const codexMarked = `# lyceum init added "/repo/.git" so Codex can commit in this repo` + "\n"

// init merges the repo's absolute .git path into writable_roots under
// [sandbox_workspace_write] in .codex/config.toml, keeping every other line.
func TestMergeCodexConfig(t *testing.T) {
	for _, tc := range []struct{ name, in, want string }{
		{name: "absent", in: "",
			want: "[sandbox_workspace_write]\n" + codexMarked + "writable_roots = [\"/repo/.git\"]\n"},
		{name: "other tables kept", in: "model = \"o3\"\n\n[tui]\ntheme = \"dark\"",
			want: "model = \"o3\"\n\n[tui]\ntheme = \"dark\"\n\n[sandbox_workspace_write]\n" + codexMarked + "writable_roots = [\"/repo/.git\"]\n"},
		{name: "table without the key", in: "[sandbox_workspace_write]\nnetwork_access = true\n[tui]\n",
			want: "[sandbox_workspace_write]\n" + codexMarked + "writable_roots = [\"/repo/.git\"]\nnetwork_access = true\n[tui]\n"},
		{name: "added to the user's array", in: "[sandbox_workspace_write]\nwritable_roots = ['/tmp/x', \"/data\"] # mine\n",
			want: "[sandbox_workspace_write]\n" + codexMarked + "writable_roots = ['/tmp/x', \"/data\", \"/repo/.git\"] # mine\n"},
		{name: "already listed by the user", in: "[sandbox_workspace_write]\nwritable_roots = [\"/repo/.git\",\"/data\"]\n",
			want: "[sandbox_workspace_write]\nwritable_roots = [\"/repo/.git\",\"/data\"]\n"},
		{name: "re-run unchanged", in: "[sandbox_workspace_write]\n" + codexMarked + "writable_roots = [\"/data\", \"/repo/.git\"]\n",
			want: "[sandbox_workspace_write]\n" + codexMarked + "writable_roots = [\"/data\", \"/repo/.git\"]\n"},
		{name: "repo moved", in: "[sandbox_workspace_write]\n# lyceum init added \"/old/.git\" so Codex can commit in this repo\nwritable_roots = [\"/data\", \"/old/.git\"]\n",
			want: "[sandbox_workspace_write]\n" + codexMarked + "writable_roots = [\"/data\", \"/repo/.git\"]\n"},
		{name: "a multi-line array that holds it", in: "[sandbox_workspace_write]\nwritable_roots = [\n  \"/repo/.git\",\n]\n",
			want: "[sandbox_workspace_write]\nwritable_roots = [\n  \"/repo/.git\",\n]\n"},
		{name: "a profile's table is another table", in: "[profiles.x.sandbox_workspace_write]\nwritable_roots = [\n]\n",
			want: "[profiles.x.sandbox_workspace_write]\nwritable_roots = [\n]\n\n[sandbox_workspace_write]\n" + codexMarked + "writable_roots = [\"/repo/.git\"]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := mergeCodexConfig([]byte(tc.in), codexGit)
			if err != nil || string(got) != tc.want {
				t.Fatalf("merge = %q, %v; want\n%q", got, err, tc.want)
			}
			again, err := mergeCodexConfig(got, codexGit)
			if err != nil || string(again) != string(got) {
				t.Fatalf("re-run = %q, %v; want it unchanged", again, err)
			}
		})
	}
}

// A writable_roots form line editing can't change safely is refused, naming why.
func TestMergeCodexConfigRefuses(t *testing.T) {
	for _, tc := range []struct{ name, in, want string }{
		{name: "multi-line array", in: "[sandbox_workspace_write]\nwritable_roots = [\n  \"/data\",\n]\n", want: "spans several lines"},
		{name: "dotted key", in: "sandbox_workspace_write.writable_roots = [\"/data\"]\n", want: "dotted key or inline table"},
		{name: "inline table", in: "sandbox_workspace_write = { writable_roots = [] }\n", want: "dotted key or inline table"},
		{name: "not an array", in: "[sandbox_workspace_write]\nwritable_roots = \"/data\"\n", want: "not an array"},
		{name: "escape", in: "[sandbox_workspace_write]\nwritable_roots = [\"C:\\temp\"]\n", want: "not a plain string"},
		{name: "text after", in: "[sandbox_workspace_write]\nwritable_roots = [] x\n", want: "text after"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := mergeCodexConfig([]byte(tc.in), codexGit)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("merge = %q, %v; want an error naming %q", got, err, tc.want)
			}
		})
	}
}

// A path with a backslash or quote round-trips through its escaped form.
func TestMergeCodexConfigEscapes(t *testing.T) {
	path := `C:\repo "x"\.git`
	got, err := mergeCodexConfig(nil, path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), `writable_roots = ["C:\\repo \"x\"\\.git"]`) {
		t.Fatalf("merge = %q", got)
	}
	if again, err := mergeCodexConfig(got, path); err != nil || string(again) != string(got) {
		t.Fatalf("re-run = %q, %v; want it unchanged", again, err)
	}
}

// AC111: eject takes out what init merged into .codex/config.toml, giving back
// the user's file as it was, or nothing when init created it.
func TestUnmergeCodexConfig_AC111(t *testing.T) {
	for _, tc := range []struct{ name, in, want string }{
		{name: "created by init", in: "", want: ""},
		{name: "other tables kept", in: "model = \"o3\"\n\n[tui]\ntheme = \"dark\"\n", want: "model = \"o3\"\n\n[tui]\ntheme = \"dark\"\n"},
		{name: "table without the key", in: "[sandbox_workspace_write]\nnetwork_access = true\n[tui]\n", want: "[sandbox_workspace_write]\nnetwork_access = true\n[tui]\n"},
		{name: "the user's array", in: "[sandbox_workspace_write]\nwritable_roots = ['/tmp/x', \"/data\"] # mine\n", want: "[sandbox_workspace_write]\nwritable_roots = ['/tmp/x', \"/data\"] # mine\n"},
		{name: "listed by the user, no marker", in: "[sandbox_workspace_write]\nwritable_roots = [\"/repo/.git\",\"/data\"]\n", want: "[sandbox_workspace_write]\nwritable_roots = [\"/repo/.git\",\"/data\"]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			merged, err := mergeCodexConfig([]byte(tc.in), codexGit)
			if err != nil {
				t.Fatal(err)
			}
			got, err := unmergeCodexConfig(merged)
			if err != nil || string(got) != tc.want || (tc.want == "") != (got == nil) {
				t.Fatalf("unmerge(%q) = %q, %v; want %q", merged, got, err, tc.want)
			}
		})
	}
	// A moved repo: the marker names the path to take out.
	in := "[sandbox_workspace_write]\n# lyceum init added \"/old/.git\" so Codex can commit in this repo\nwritable_roots = [\"/data\", \"/old/.git\"]\n"
	if got, err := unmergeCodexConfig([]byte(in)); err != nil || string(got) != "[sandbox_workspace_write]\nwritable_roots = [\"/data\"]\n" {
		t.Errorf("unmerge(%q) = %q, %v", in, got, err)
	}
	// A marker whose line below is no longer writable_roots can't be edited safely.
	in = "[sandbox_workspace_write]\n" + codexMarked + "network_access = true\n"
	if _, err := unmergeCodexConfig([]byte(in)); err == nil {
		t.Errorf("unmerge(%q): want an error", in)
	}
}
