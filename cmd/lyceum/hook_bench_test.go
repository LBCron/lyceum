package main

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// BenchmarkHookPreTool200Plans reports NF2 for the hook entry point: in a repo
// with 200 committed plans and a feat at spec on the branch, the release
// binary's `hook pre-tool --harness claude` decides a write of src/x.ts (a
// deny, the full path) cold 20 times, and the median is reported as ms/median.
// Like the status benchmark, it reports and never fails on the time.
func BenchmarkHookPreTool200Plans(b *testing.B) {
	repo, bin, run := repo200Plans(b)
	run(bin, "add", "feat", "Hook bench")
	payload, err := json.Marshal(map[string]any{
		"cwd":             repo,
		"hook_event_name": "PreToolUse",
		"tool_name":       "Write",
		"tool_input":      map[string]string{"file_path": filepath.Join(repo, "src", "x.ts"), "content": "x"},
	})
	if err != nil {
		b.Fatal(err)
	}
	hook := func() {
		cmd := exec.Command(bin, "hook", "pre-tool", "--harness", "claude")
		cmd.Dir = repo
		cmd.Stdin = bytes.NewReader(payload)
		out, err := cmd.Output()
		if err != nil || !strings.Contains(string(out), `"permissionDecision":"deny"`) {
			b.Fatalf("hook pre-tool: %v; stdout %q; want a deny", err, out)
		}
	}
	var median time.Duration
	for b.Loop() {
		median = medianRun(20, hook)
	}
	b.ReportMetric(float64(median)/float64(time.Millisecond), "ms/median")
}
