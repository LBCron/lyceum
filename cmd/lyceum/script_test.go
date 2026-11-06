package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
	"go.yaml.in/yaml/v3"
)

// binDir holds the lyceum (test build), lyceum-release (dev build) and
// lyceum-published (release build, publishedLdflags) binaries that scripts run.
var binDir string

// modRoot is the directory holding go.mod.
var modRoot string

// scriptPath is the host's PATH with its harness binaries, gh and glab hidden (harnessFreePath).
var scriptPath string

// harnessBinaries are the binaries init detects a harness by (distribution.md, "lyceum init" step 4).
var harnessBinaries = []string{"claude", "codex", "opencode", "pi", "commandcode"}

// hiddenBinaries are hidden from scripts: the harness binaries, gh and glab,
// so close never opens a real pull request, and the audit tools, so audit runs
// only the script's own; scripts put fakes on PATH.
var hiddenBinaries = append(slices.Clone(harnessBinaries), "gh", "glab",
	"npm", "govulncheck", "pip-audit", "cargo", "cargo-audit", "composer", "bundle", "bundler-audit")

// fixedDate pins git author and committer dates in scripts.
const fixedDate = "2026-01-01T00:00:00Z"

// TestMain builds the real binaries once per test run. Scripts run them as child
// processes, never in-process: test-only behaviour comes from the lyceumtest tag,
// which `go test ./...` does not set.
func TestMain(m *testing.M) {
	if len(os.Args) == 5 && os.Args[1] == ttyHelperArg {
		os.Exit(ttyHelper(os.Args[2], os.Args[3], os.Args[4]))
	}
	os.Exit(runMain(m))
}

func runMain(m *testing.M) int {
	dir, err := os.MkdirTemp("", "lyceum-script-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "TestMain:", err)
		return 1
	}
	defer func() { _ = os.RemoveAll(dir) }()
	binDir = dir

	root, err := moduleRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, "TestMain:", err)
		return 1
	}
	modRoot = root
	if scriptPath, err = harnessFreePath(dir, os.Getenv("PATH")); err != nil {
		fmt.Fprintln(os.Stderr, "TestMain:", err)
		return 1
	}
	ldflags, err := publishedLdflags(filepath.Join(root, ".goreleaser.yaml"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "TestMain:", err)
		return 1
	}
	builds := [][]string{
		{"build", "-tags", "lyceumtest", "-o", filepath.Join(dir, "lyceum"), "./cmd/lyceum"},
		{"build", "-o", filepath.Join(dir, "lyceum-release"), "./cmd/lyceum"},
		{"build", "-trimpath", "-ldflags", ldflags, "-o", filepath.Join(dir, "lyceum-published"), "./cmd/lyceum"},
	}
	for _, args := range builds {
		if err := goBuild(args...); err != nil {
			fmt.Fprintln(os.Stderr, "TestMain:", err)
			return 1
		}
	}
	return m.Run()
}

// publishedVersion stands in for the tag's version in the lyceum-published build.
const publishedVersion = "1.2.3-rc.1"

// publishedLdflags returns the ldflags of the release build in the goreleaser
// config at path, with {{.Version}} set to publishedVersion, so the
// lyceum-published binary is built the way a release is (AC182).
func publishedLdflags(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var cfg struct {
		Builds []struct {
			Ldflags []string `yaml:"ldflags"`
		} `yaml:"builds"`
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return "", fmt.Errorf("%s: %w", path, err)
	}
	if len(cfg.Builds) != 1 || len(cfg.Builds[0].Ldflags) == 0 {
		return "", fmt.Errorf("%s: want one build with ldflags", path)
	}
	flags := strings.Join(cfg.Builds[0].Ldflags, " ")
	if !strings.Contains(flags, "{{.Version}}") {
		return "", fmt.Errorf("%s: ldflags do not set {{.Version}}", path)
	}
	return strings.ReplaceAll(flags, "{{.Version}}", publishedVersion), nil
}

// goBuild runs go with args from the module root, without cgo.
func goBuild(args ...string) error {
	// The test binary runs where it was built, so GOROOT is the go running this test.
	cmd := exec.Command(filepath.Join(runtime.GOROOT(), "bin", "go"), args...) //nolint:staticcheck // SA1019, see above
	cmd.Dir = modRoot
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("go %v: %v\n%s", args, err, out)
	}
	return nil
}

// harnessFreePath is path with every hidden binary (hiddenBinaries) on it
// hidden, so init in a script detects only the harnesses the script sets up,
// never the host's, and close finds only the script's gh or glab. A directory
// holding one is replaced by a mirror under dir: a symlink to each of its other
// entries.
func harnessFreePath(dir, path string) (string, error) {
	var dirs []string
	for i, d := range filepath.SplitList(path) {
		if d == "" || !slices.ContainsFunc(hiddenBinaries, func(b string) bool {
			_, err := os.Stat(filepath.Join(d, b))
			return err == nil
		}) {
			dirs = append(dirs, d)
			continue
		}
		mirror := filepath.Join(dir, "path", strconv.Itoa(i))
		if err := os.MkdirAll(mirror, 0o755); err != nil {
			return "", err
		}
		entries, err := os.ReadDir(d)
		if err != nil {
			return "", err
		}
		for _, e := range entries {
			if !slices.Contains(hiddenBinaries, e.Name()) {
				if err := os.Symlink(filepath.Join(d, e.Name()), filepath.Join(mirror, e.Name())); err != nil {
					return "", err
				}
			}
		}
		dirs = append(dirs, mirror)
	}
	return strings.Join(dirs, string(os.PathListSeparator)), nil
}

// moduleRoot walks up from the working directory to the directory holding go.mod.
func moduleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("go.mod not found above the working directory")
		}
		dir = parent
	}
}

func TestScripts(t *testing.T) {
	testscript.Run(t, testscript.Params{
		Dir:   filepath.Join("testdata", "script"),
		Setup: setupScript,
		Cmds:  map[string]func(*testscript.TestScript, bool, []string){"jsonobject": jsonObject, "jsonl": jsonLines, "sha256file": sha256File, "payload": payload, "hookrun": hookRun, "ttyrun": ttyRun, "linesub": lineSub, "shorttemp": shortTemp},
	})
}

// lineSub implements the script command `linesub <in> <out> <line> <text>`: it
// writes in to out with each line exactly equal to line replaced by text, where
// \n in text is a newline, and fails when no line matches. in and out may each
// be a glob naming one file. It stands in for sed, whose \n differs on macOS.
func lineSub(ts *testscript.TestScript, neg bool, args []string) {
	if neg || len(args) != 4 {
		ts.Fatalf("usage: linesub <in> <out> <line> <text>")
	}
	one := func(path string) string {
		if !strings.Contains(path, "*") {
			return ts.MkAbs(path)
		}
		matches, err := filepath.Glob(ts.MkAbs(path))
		ts.Check(err)
		if len(matches) != 1 {
			ts.Fatalf("%s names %d files; want 1", path, len(matches))
		}
		return matches[0]
	}
	lines := strings.Split(ts.ReadFile(one(args[0])), "\n")
	found := false
	for i, l := range lines {
		if l == args[2] {
			lines[i], found = strings.ReplaceAll(args[3], `\n`, "\n"), true
		}
	}
	if !found {
		ts.Fatalf("%s has no line %q", args[0], args[2])
	}
	ts.Check(os.WriteFile(one(args[1]), []byte(strings.Join(lines, "\n")), 0o644))
}

// shortTemp implements the script command `shorttemp <var>`: it makes a directory
// under /tmp, removed when the script ends, and sets var to it. $WORK can be too
// long on macOS for a Unix socket path, which has a 104-byte limit.
func shortTemp(ts *testscript.TestScript, neg bool, args []string) {
	if neg || len(args) != 1 {
		ts.Fatalf("usage: shorttemp <var>")
	}
	dir, err := os.MkdirTemp("/tmp", "lyc-")
	ts.Check(err)
	ts.Defer(func() { _ = os.RemoveAll(dir) })
	ts.Setenv(args[0], dir)
}

// jsonObject implements the script command `jsonobject`: stdout of the last exec
// is exactly one JSON object and nothing else.
func jsonObject(ts *testscript.TestScript, neg bool, args []string) {
	if neg || len(args) != 0 {
		ts.Fatalf("usage: jsonobject")
	}
	dec := json.NewDecoder(strings.NewReader(ts.ReadFile("stdout")))
	var obj map[string]any
	if err := dec.Decode(&obj); err != nil {
		ts.Fatalf("stdout is not a JSON object: %v", err)
	}
	if _, err := dec.Token(); err != io.EOF {
		ts.Fatalf("stdout has more than one JSON value")
	}
}

// jsonLines implements the script command `jsonl <file> <n>`: the file is n
// newline-terminated lines, each exactly one JSON object.
func jsonLines(ts *testscript.TestScript, neg bool, args []string) {
	if neg || len(args) != 2 {
		ts.Fatalf("usage: jsonl <file> <n>")
	}
	want, err := strconv.Atoi(args[1])
	if err != nil {
		ts.Fatalf("jsonl: %v", err)
	}
	data := ts.ReadFile(args[0])
	if !strings.HasSuffix(data, "\n") {
		ts.Fatalf("%s does not end with a newline", args[0])
	}
	lines := strings.Split(strings.TrimSuffix(data, "\n"), "\n")
	if len(lines) != want {
		ts.Fatalf("%s has %d lines; want %d", args[0], len(lines), want)
	}
	for i, line := range lines {
		var obj map[string]any
		if err := json.Unmarshal([]byte(line), &obj); err != nil || obj == nil {
			ts.Fatalf("%s line %d is not a JSON object: %q", args[0], i+1, line)
		}
	}
}

// sha256File implements the script command `sha256file <path>`: it prints the
// file's sha256 as "sha256:<hex>", the form approvals record. It keeps scripts
// free of an external hashing tool.
func sha256File(ts *testscript.TestScript, neg bool, args []string) {
	if neg || len(args) != 1 {
		ts.Fatalf("usage: sha256file <path>")
	}
	sum := sha256.Sum256([]byte(ts.ReadFile(args[0])))
	_, _ = fmt.Fprintf(ts.Stdout(), "sha256:%s\n", hex.EncodeToString(sum[:]))
}

// payload implements the script command `payload <harness>/<file> <out> [old=new ...]`:
// it writes the captured pre-tool payload internal/harness/testdata/payloads/<harness>/<file>
// to out, with each old replaced by its new, in order, and then /repo by $REPO.
func payload(ts *testscript.TestScript, neg bool, args []string) {
	if neg || len(args) < 2 {
		ts.Fatalf("usage: payload <harness>/<file> <out> [old=new ...]")
	}
	data, err := os.ReadFile(filepath.Join(modRoot, "internal", "harness", "testdata", "payloads", filepath.FromSlash(args[0])))
	ts.Check(err)
	text := string(data)
	for _, r := range args[2:] {
		old, repl, ok := strings.Cut(r, "=")
		if !ok || !strings.Contains(text, old) {
			ts.Fatalf("payload: %q: not old=new, or %s has no %q", r, args[0], old)
		}
		text = strings.ReplaceAll(text, old, repl)
	}
	text = strings.ReplaceAll(text, "/repo", ts.Getenv("REPO"))
	ts.Check(os.WriteFile(ts.MkAbs(args[1]), []byte(text), 0o644))
}

// hookRun implements the script command `hookrun <settings.json>`: the file is
// valid JSON whose hooks.PreToolUse holds exactly one hook running "lyceum hook
// pre-tool". It runs that hook's command with sh -c, as a harness runs it, on
// the stdin set by `stdin`, and fails when it exits non-zero.
func hookRun(ts *testscript.TestScript, neg bool, args []string) {
	if neg || len(args) != 1 {
		ts.Fatalf("usage: hookrun <settings.json>")
	}
	var settings struct {
		Hooks struct {
			PreToolUse []struct {
				Matcher string `json:"matcher"`
				Hooks   []struct {
					Type    string `json:"type"`
					Command string `json:"command"`
				} `json:"hooks"`
			} `json:"PreToolUse"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal([]byte(ts.ReadFile(args[0])), &settings); err != nil {
		ts.Fatalf("%s: %v", args[0], err)
	}
	var commands []string
	for _, e := range settings.Hooks.PreToolUse {
		for _, h := range e.Hooks {
			if strings.Contains(h.Command, "lyceum hook pre-tool") {
				if h.Type != "command" {
					ts.Fatalf("%s: the lyceum hook has type %q; want command", args[0], h.Type)
				}
				commands = append(commands, h.Command)
			}
		}
	}
	if len(commands) != 1 {
		ts.Fatalf("%s has %d lyceum hooks; want 1", args[0], len(commands))
	}
	ts.Check(ts.Exec("sh", "-c", commands[0]))
}

// setupScript isolates each script: its own HOME, a PATH without the host's
// harness binaries, no global or system git config, fixed time and seed, and a
// fresh git repo at $REPO with one empty commit.
func setupScript(env *testscript.Env) error {
	env.Setenv("PATH", binDir+string(os.PathListSeparator)+scriptPath)
	env.Setenv("HOME", env.WorkDir)
	env.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	env.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	env.Setenv("GIT_AUTHOR_DATE", fixedDate)
	env.Setenv("GIT_COMMITTER_DATE", fixedDate)
	env.Setenv("LYCEUM_NOW", "2026-01-01T00:00:00Z")
	env.Setenv("LYCEUM_SEED", "1")
	env.Setenv("LYCEUM_ACTION_PUBLISHED", "1")
	env.Setenv("TEMPLATES", filepath.Join(modRoot, "internal", "templates"))
	env.Setenv("CHECK_ACTION", filepath.Join(modRoot, "check-action"))

	git, err := exec.LookPath("git")
	if err != nil {
		return fmt.Errorf("git is required for AC scripts: %w", err)
	}
	repo := filepath.Join(env.WorkDir, "repo")
	if err := os.Mkdir(repo, 0o755); err != nil {
		return err
	}
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "user.name", "Test Owner"},
		{"config", "user.email", "owner@example.com"},
		{"commit", "-q", "--allow-empty", "-m", "initial"},
	} {
		cmd := exec.Command(git, args...)
		cmd.Dir = repo
		cmd.Env = env.Vars
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git %v: %v\n%s", args, err, out)
		}
	}
	env.Setenv("REPO", repo)
	return nil
}
