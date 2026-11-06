package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// BenchmarkStatus200Plans reports AC3: in a repo with 200 committed plans and
// 200 records (repo200Plans), the release binary's status runs cold 10 times
// and the median is reported as ms/median. AC3 is validated by benchmark and does not gate the suite,
// so it reports and never fails on the time.
func BenchmarkStatus200Plans(b *testing.B) {
	_, bin, run := repo200Plans(b)
	var median time.Duration
	for b.Loop() {
		median = medianRun(10, func() { run(bin, "status") })
	}
	b.ReportMetric(float64(median)/float64(time.Millisecond), "ms/median")
}

// repo200Plans sets up a repo with Lyceum, 200 committed plans and 200
// committed understand records for a benchmark. Each record cites one of 200
// source files and the folder holding it, one of 20, so status hashes 200
// files and 20 folders for freshness. It returns the repo, the release binary, and run, which runs a
// command in the repo and fails the benchmark if it fails.
func repo200Plans(b *testing.B) (repo, bin string, run func(name string, args ...string)) {
	b.Helper()
	b.Setenv("HOME", b.TempDir())
	b.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	b.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	b.Setenv("GIT_AUTHOR_DATE", fixedDate)
	b.Setenv("GIT_COMMITTER_DATE", fixedDate)
	bin = filepath.Join(binDir, "lyceum-release")
	repo = b.TempDir()

	run = func(name string, args ...string) {
		b.Helper()
		cmd := exec.Command(name, args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			b.Fatalf("%s %v: %v\n%s", name, args, err, out)
		}
	}
	run("git", "init", "-q", "-b", "main")
	run("git", "config", "user.name", "Test Owner")
	run("git", "config", "user.email", "owner@example.com")
	run("git", "commit", "-q", "--allow-empty", "-m", "initial")
	run(bin, "init")
	dir := filepath.Join(repo, "lyceum", "plan")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		b.Fatal(err)
	}
	for i := range 200 {
		id := fmt.Sprintf("%06x-unit-%d", i, i)
		plan := fmt.Sprintf("---\nid: %s\ntype: feat\ntitle: Unit %d\nbranch: feat/%s\nstatus: planned\ngate: spec\napprovals: []\n---\n\n## What and why\n", id, i, id)
		if err := os.WriteFile(filepath.Join(dir, id+".md"), []byte(plan), 0o644); err != nil {
			b.Fatal(err)
		}
	}
	for i := range 200 {
		src := filepath.Join(repo, "src", fmt.Sprintf("pkg%d", i%20))
		if err := os.MkdirAll(src, 0o755); err != nil {
			b.Fatal(err)
		}
		code := fmt.Sprintf("package pkg%d\n\nfunc F%d() int { return %d }\n", i%20, i, i)
		if err := os.WriteFile(filepath.Join(src, fmt.Sprintf("f%d.go", i)), []byte(code), 0o644); err != nil {
			b.Fatal(err)
		}
	}
	run("git", "add", "-A")
	run("git", "commit", "-q", "-m", "200 plans and code")

	// The records cite the committed blob and tree IDs, which are the work tree's.
	ids := map[string]string{}
	for _, args := range [][]string{{"ls-tree", "-r", "HEAD", "src/"}, {"ls-tree", "-d", "HEAD", "src/"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		out, err := cmd.Output()
		if err != nil {
			b.Fatalf("git %v: %v", args, err)
		}
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			// "<mode> <type> <id>\t<path>"
			meta, path, _ := strings.Cut(line, "\t")
			ids[path] = strings.Fields(meta)[2]
		}
	}
	dir = filepath.Join(repo, "lyceum", "understand")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		b.Fatal(err)
	}
	for i := range 200 {
		id := fmt.Sprintf("%06x-record-%d", i, i)
		pkg := fmt.Sprintf("src/pkg%d", i%20)
		file := fmt.Sprintf("%s/f%d.go", pkg, i)
		rec := fmt.Sprintf("---\nid: %s\nkind: understand\ntitle: Record %d\ncreated: 2026-01-01T00:00:00Z\ncites:\n"+
			"  - path: %s\n    symbol: F%d()\n    blob: \"%s\"\n  - path: %s/\n    tree: \"%s\"\n---\n\n## Summary\n",
			id, i, file, i, ids[file], pkg, ids[pkg])
		if err := os.WriteFile(filepath.Join(dir, id+".md"), []byte(rec), 0o644); err != nil {
			b.Fatal(err)
		}
	}
	run("git", "add", "-A")
	run("git", "commit", "-q", "-m", "200 records")
	cmd := exec.Command(bin, "status")
	cmd.Dir = repo
	if out, err := cmd.Output(); err != nil || !strings.Contains(string(out), "Records: 200 fresh, 0 drifted, 0 broken\n") {
		b.Fatalf("status on the fixture: %v\n%s", err, out)
	}
	return repo, bin, run
}

// medianRun runs f n times, n even, and returns the median duration.
func medianRun(n int, f func()) time.Duration {
	runs := make([]time.Duration, n)
	for i := range runs {
		start := time.Now()
		f()
		runs[i] = time.Since(start)
	}
	slices.Sort(runs)
	return (runs[n/2-1] + runs[n/2]) / 2
}
