package approval

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/oritromax/lyceum/internal/gate"
	"github.com/oritromax/lyceum/internal/out"
	"github.com/oritromax/lyceum/internal/unit"
)

const planPath = "lyceum/plan/a3f9c2-csv-export.md"

const plan = `---
id: a3f9c2-csv-export
type: feat
title: CSV export
branch: feat/a3f9c2-csv-export
status: planned
# gate comment
gate: spec # current gate
areas: [orders, billing]
files: [src/orders/export.ts]
approvals: []
acs: [AC1]
---

## What and why

Export orders as CSV.
`

// writeFiles writes name → content under root and returns root.
func writeFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, content := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func readPlan(t *testing.T, root string) *unit.Plan {
	t.Helper()
	p, err := unit.ReadPlan(filepath.Join(root, filepath.FromSlash(planPath)))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func sha(s string) string {
	sum := sha256.Sum256([]byte(s))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func TestFilesFor_AC42(t *testing.T) {
	root := writeFiles(t, map[string]string{
		planPath:                 plan,
		"lyceum/spec/orders.md":  "# Orders\n",
		"lyceum/spec/billing.md": "# Billing\n",
		SpecPath:                 registry,
	})
	p := readPlan(t, root)
	for _, tc := range []struct {
		typ  gate.Type
		g    gate.Gate
		want []string
	}{
		{gate.Feat, gate.Spec, []string{planPath, "lyceum/spec/orders.md", "lyceum/spec/billing.md", SpecPath}},
		{gate.Refactor, gate.Intent, []string{planPath}},
		{gate.Refactor, gate.Plan, []string{planPath}},
		{gate.PlannedChore, gate.Plan, []string{planPath}},
		{gate.Feat, Testing, []string{"TESTING.md"}},
	} {
		got, err := FilesFor(root, tc.typ, tc.g, planPath, p)
		if err != nil || !slices.Equal(got, tc.want) {
			t.Errorf("FilesFor(%s, %s) = %q, %v; want %q", tc.typ, tc.g, got, err, tc.want)
		}
	}
}

const registry = `# Specification index

## Area registry

The CLI owns this table.

| Area | File | Covers | ACs |
|---|---|---|---|
| orders | ` + "`spec/orders.md`" + ` | Orders | AC1 |
| billing | spec/billing.md | Billing | |

## Open items

| Item | File |
|---|---|
| ignored | spec/nope.md |
`

func TestRegisteredAreas_AC38(t *testing.T) {
	root := writeFiles(t, map[string]string{SpecPath: registry})
	got, err := RegisteredAreas(root)
	if want := []string{"lyceum/spec/orders.md", "lyceum/spec/billing.md"}; err != nil || !slices.Equal(got, want) {
		t.Errorf("RegisteredAreas = %q, %v; want %q", got, err, want)
	}
	for name, content := range map[string]string{
		"empty registry": "## Area registry\n\n| Area | File | Covers | ACs |\n|---|---|---|---|\n",
		"no registry":    "# Specification index\n",
	} {
		root := writeFiles(t, map[string]string{SpecPath: content})
		if got, err := RegisteredAreas(root); err != nil || len(got) != 0 {
			t.Errorf("%s: RegisteredAreas = %q, %v; want none", name, got, err)
		}
	}
	if got, err := RegisteredAreas(t.TempDir()); err != nil || len(got) != 0 {
		t.Errorf("no SPEC.md: RegisteredAreas = %q, %v; want none", got, err)
	}
	root = writeFiles(t, map[string]string{SpecPath: "## Area registry\n\n| Area | Covers |\n|---|---|\n"})
	if _, err := RegisteredAreas(root); err == nil || !strings.Contains(err.Error(), "no File column") {
		t.Errorf("no File column: error = %v; want one naming the column", err)
	}
}

// project is repo-level: it hashes SpecPath first, then the registered areas,
// with no plan.
func TestFilesForProject_AC38(t *testing.T) {
	root := writeFiles(t, map[string]string{
		SpecPath:                 registry,
		"lyceum/spec/orders.md":  "# Orders\n",
		"lyceum/spec/billing.md": "# Billing\n",
	})
	got, err := FilesFor(root, "", Project, "", nil)
	if want := []string{"lyceum/SPEC.md", "lyceum/spec/orders.md", "lyceum/spec/billing.md"}; err != nil || !slices.Equal(got, want) {
		t.Errorf("FilesFor(project) = %q, %v; want %q", got, err, want)
	}

	var r *out.Refusal
	root = writeFiles(t, map[string]string{SpecPath: registry, "lyceum/spec/orders.md": "# Orders\n"})
	if _, err := FilesFor(root, "", Project, "", nil); !errors.As(err, &r) || !strings.Contains(err.Error(), "lists lyceum/spec/billing.md, but it does not exist") {
		t.Errorf("missing area file: error = %v; want a refusal naming it", err)
	}
	root = writeFiles(t, map[string]string{SpecPath: "## Area registry\n"})
	if _, err := FilesFor(root, "", Project, "", nil); !errors.As(err, &r) || !strings.Contains(err.Error(), "no area is registered in the area registry of lyceum/SPEC.md") {
		t.Errorf("empty registry: error = %v; want a refusal naming the registry", err)
	}
}

func TestFilesForMissingAreaRefuses(t *testing.T) {
	root := writeFiles(t, map[string]string{planPath: plan, "lyceum/spec/orders.md": "# Orders\n", SpecPath: registry})
	_, err := FilesFor(root, gate.Feat, gate.Spec, planPath, readPlan(t, root))
	var r *out.Refusal
	if !errors.As(err, &r) || !strings.Contains(err.Error(), `area "billing"`) || !strings.Contains(err.Error(), "lyceum/spec/billing.md does not exist") {
		t.Errorf("error = %v; want a refusal naming billing and its file", err)
	}
}

func TestFilesForUnregisteredArea_AC37(t *testing.T) {
	spec := strings.Replace(registry, "| billing | spec/billing.md | Billing | |\n", "", 1)
	root := writeFiles(t, map[string]string{planPath: plan, "lyceum/spec/orders.md": "# Orders\n", "lyceum/spec/billing.md": "# Billing\n", SpecPath: spec})
	p := readPlan(t, root)
	for _, g := range []gate.Gate{gate.Spec, gate.Plan, gate.Intent} {
		_, err := FilesFor(root, gate.Refactor, g, planPath, p)
		var r *out.Refusal
		if !errors.As(err, &r) || !strings.Contains(err.Error(), `area "billing", which is not in the area registry`) ||
			!strings.Contains(err.Error(), "the registered areas are orders") {
			t.Errorf("FilesFor(%s) error = %v; want a refusal listing orders", g, err)
		}
	}
	// No registry at all: nothing is registered.
	root = writeFiles(t, map[string]string{planPath: plan})
	if _, err := FilesFor(root, gate.Feat, gate.Spec, planPath, readPlan(t, root)); err == nil || !strings.Contains(err.Error(), "no area is registered yet") {
		t.Errorf("no registry: error = %v; want no area is registered yet", err)
	}
}

func TestRegisterArea(t *testing.T) {
	root := writeFiles(t, map[string]string{SpecPath: registry})
	if err := RegisterArea(root, "shipping"); err != nil {
		t.Fatal(err)
	}
	got, err := RegisteredAreas(root)
	want := []string{"lyceum/spec/orders.md", "lyceum/spec/billing.md", "lyceum/spec/shipping.md"}
	if err != nil || !slices.Equal(got, want) {
		t.Errorf("RegisteredAreas = %q, %v; want %q", got, err, want)
	}
	data, err := os.ReadFile(filepath.Join(root, SpecPath))
	if err != nil {
		t.Fatal(err)
	}
	// The row goes right after the registry's last row; the next section is untouched.
	if !strings.Contains(string(data), "| billing | spec/billing.md | Billing | |\n| shipping | `spec/shipping.md` | | |\n\n## Open items\n") {
		t.Errorf("SPEC.md =\n%s", data)
	}
	if err := RegisterArea(root, "orders"); err == nil {
		t.Error("RegisterArea(orders) succeeded; orders is already registered")
	}
	if err := RegisterArea(writeFiles(t, map[string]string{SpecPath: "# Index\n"}), "orders"); err == nil {
		t.Error("RegisterArea with no registry table succeeded")
	}
}

func TestFilesForNotBuiltRefuses(t *testing.T) {
	root := writeFiles(t, map[string]string{planPath: plan})
	p := readPlan(t, root)
	for _, tc := range []struct {
		typ gate.Type
		g   gate.Gate
	}{{gate.Refactor, gate.Verify}} {
		_, err := FilesFor(root, tc.typ, tc.g, planPath, p)
		var r *out.Refusal
		if !errors.As(err, &r) || !strings.Contains(err.Error(), "not built yet") {
			t.Errorf("FilesFor(%s, %s) error = %v; want a not-built refusal", tc.typ, tc.g, err)
		}
	}
	if _, err := FilesFor(root, gate.Feat, gate.Build, planPath, p); err == nil {
		t.Error("FilesFor(feat, build) succeeded; build is not approved")
	}
	// cause's files come from the branch diff, which the CLI lists.
	if _, err := FilesFor(root, gate.Bug, gate.Cause, planPath, p); err == nil {
		t.Error("FilesFor(bug, cause) succeeded; the CLI lists cause's files")
	}
}

func TestHash(t *testing.T) {
	root := writeFiles(t, map[string]string{"a.txt": "hello\n"})
	got, err := Hash(filepath.Join(root, "a.txt"))
	if want := "sha256:5891b5b522d5df086d0ff0b110fbd9d21bb4fc7163af34d08286a2e846f6be03"; err != nil || got != want {
		t.Errorf("Hash = %q, %v; want %q", got, err, want)
	}
	if _, err := Hash(filepath.Join(root, "missing.txt")); err == nil {
		t.Error("Hash of a missing file succeeded")
	}
}

func TestHashesPlanOnBasis_AC42(t *testing.T) {
	root := writeFiles(t, map[string]string{planPath: plan, "lyceum/spec/orders.md": "# Orders\n"})
	got, err := Hashes(root, planPath, gate.Spec, []string{planPath, "lyceum/spec/orders.md"})
	if err != nil {
		t.Fatal(err)
	}
	basis := strings.NewReplacer("status: planned\n", "", "gate: spec # current gate\n", "", "approvals: []\n", "", "files: [src/orders/export.ts]\n", "").Replace(plan)
	want := map[string]string{planPath: sha(basis), "lyceum/spec/orders.md": sha("# Orders\n")}
	if len(got) != len(want) || got[planPath] != want[planPath] || got["lyceum/spec/orders.md"] != want["lyceum/spec/orders.md"] {
		t.Errorf("Hashes = %v; want %v", got, want)
	}
}

func TestPlanHashBasis(t *testing.T) {
	basis, err := PlanHashBasis([]byte(plan))
	if err != nil {
		t.Fatal(err)
	}
	// The comment line above gate belongs to no key and stays.
	want := strings.NewReplacer("status: planned\n", "", "gate: spec # current gate\n", "", "approvals: []\n", "", "files: [src/orders/export.ts]\n", "").Replace(plan)
	if string(basis) != want {
		t.Errorf("basis:\n%s\nwant:\n%s", basis, want)
	}

	// Changing only CLI-owned approvals, gate and status keeps the basis, whether
	// the CLI writes them or they are absent or edited by hand.
	path := filepath.Join(t.TempDir(), "plan.md")
	if err := os.WriteFile(path, []byte(plan), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := unit.ReadPlan(path)
	if err != nil {
		t.Fatal(err)
	}
	a := unit.Approval{Gate: "spec", Hashes: map[string]string{planPath: sha(want)}, Commit: "1a2b3c4",
		By: "owner@example.com", At: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	for _, set := range []struct {
		key   string
		value any
	}{{"approvals", []unit.Approval{a}}, {"gate", "build"}, {"status", unit.StatusInProgress}} {
		if err := p.SetCLIField(set.key, set.value); err != nil {
			t.Fatal(err)
		}
	}
	if err := unit.WritePlan(path, p); err != nil {
		t.Fatal(err)
	}
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	same := map[string]string{
		"approve via SetCLIField": string(written),
		"keys absent":             want,
		"approvals by hand":       strings.Replace(plan, "approvals: []\n", "approvals:\n  - gate: spec\n    by: x\n", 1),
		"gate and status by hand": strings.NewReplacer("status: planned", "status: closed", "gate: spec # current gate", "gate: close").Replace(plan),
		"a file added to files:":  strings.Replace(plan, "files: [src/orders/export.ts]", "files: [src/orders/export.ts, src/x.ts]", 1),
	}
	for name, content := range same {
		got, err := PlanHashBasis([]byte(content))
		if err != nil || string(got) != want {
			t.Errorf("%s: basis = %q, %v; want unchanged", name, got, err)
		}
	}

	// Changing anything else changes it: agent-written keys, other CLI-owned keys, comments, the body.
	changed := map[string]string{
		"areas":         strings.Replace(plan, "areas: [orders, billing]", "areas: [orders]", 1),
		"acs":           strings.Replace(plan, "acs: [AC1]", "acs: [AC1, AC2]", 1),
		"new agent key": strings.Replace(plan, "acs: [AC1]\n", "acs: [AC1]\nnotes: x\n", 1),
		"title":         strings.Replace(plan, "title: CSV export", "title: CSV export v2", 1),
		"id":            strings.Replace(plan, "id: a3f9c2-csv-export", "id: b4e0d3-csv-export", 1),
		"comment":       strings.Replace(plan, "# gate comment", "# a gate comment", 1),
		"body":          strings.Replace(plan, "Export orders as CSV.", "Export orders as CSV and JSON.", 1),
		"body newline":  plan + "\n",
	}
	for name, content := range changed {
		got, err := PlanHashBasis([]byte(content))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if string(got) == want {
			t.Errorf("%s: basis unchanged; want the edit to change it", name)
		}
	}

	for _, bad := range []string{"no frontmatter\n", "---\nstatus: planned\n", "---\n{status: planned}\n---\n"} {
		if _, err := PlanHashBasis([]byte(bad)); err == nil {
			t.Errorf("PlanHashBasis(%q) succeeded; want an error", bad)
		}
	}
}

func TestPlanHashBasisSkipsProgressAndLog_AC43(t *testing.T) {
	withSections := plan + "\n## Steps\n\n1. Write it.\n\n## Progress\n\n- [ ] T1\n\n## Log\n\n## Notes\n\nKept.\n"
	basis, err := PlanHashBasis([]byte(withSections))
	if err != nil {
		t.Fatal(err)
	}
	// Each section runs to the next "## " heading or the end of the file.
	want := strings.NewReplacer("status: planned\n", "", "gate: spec # current gate\n", "", "approvals: []\n", "", "files: [src/orders/export.ts]\n", "",
		"## Progress\n\n- [ ] T1\n\n## Log\n\n", "").Replace(withSections)
	if string(basis) != want {
		t.Errorf("basis:\n%s\nwant:\n%s", basis, want)
	}

	same := map[string]string{
		"progress ticked": strings.Replace(withSections, "- [ ] T1", "- [x] T1", 1),
		"log appended":    strings.Replace(withSections, "## Log\n", "## Log\n\n- T1 done\n", 1),
	}
	for name, content := range same {
		got, err := PlanHashBasis([]byte(content))
		if err != nil || string(got) != want {
			t.Errorf("%s: basis = %q, %v; want unchanged", name, got, err)
		}
	}
	changed := map[string]string{
		"steps":         strings.Replace(withSections, "1. Write it.", "1. Write it twice.", 1),
		"other section": strings.Replace(withSections, "Kept.", "Changed.", 1),
		"acs":           strings.Replace(withSections, "acs: [AC1]", "acs: [AC2]", 1),
	}
	for name, content := range changed {
		got, err := PlanHashBasis([]byte(content))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if string(got) == want {
			t.Errorf("%s: basis unchanged; want the edit to change it", name)
		}
	}
}

const intentPlan = `---
id: aaaaaa-tidy
type: refactor
title: Tidy
branch: refactor/aaaaaa-tidy
status: planned
gate: intent
approvals: []
goal: performance
benchmark: sh ./bench.sh
---

## What and why

Make add faster.

## Steps
`

func TestIntentHashBasis_AC130(t *testing.T) {
	basis, err := IntentHashBasis([]byte(intentPlan))
	if err != nil {
		t.Fatal(err)
	}
	want := "goal:11\nperformance\nbenchmark:13\nsh ./bench.sh\nwhat-and-why:16\nMake add faster.\n"
	if string(basis) != want {
		t.Errorf("basis = %q; want %q", basis, want)
	}
	// An absent goal, benchmark and section are empty.
	empty, err := IntentHashBasis([]byte("---\nid: aaaaaa-tidy\nstatus: planned\n---\n"))
	if want := "goal:0\n\nbenchmark:0\n\nwhat-and-why:0\n\n"; err != nil || string(empty) != want {
		t.Errorf("empty basis = %q, %v; want %q", empty, err, want)
	}

	// Steps, public_surface:, other keys, CLI-owned keys and blank lines around the section keep it.
	same := map[string]string{
		"steps":          strings.Replace(intentPlan, "## Steps\n", "## Steps\n\n1. Inline the loop.\n\n## Progress\n\n## Log\n", 1),
		"public_surface": strings.Replace(intentPlan, "approvals: []\n", "approvals: []\npublic_surface: [add]\n", 1),
		"areas":          strings.Replace(intentPlan, "approvals: []\n", "approvals: []\nareas: [orders]\n", 1),
		"gate":           strings.Replace(intentPlan, "gate: intent", "gate: plan", 1),
		"blank lines":    strings.Replace(intentPlan, "Make add faster.\n", "\nMake add faster.\n\n\n", 1),
		"quoting":        strings.Replace(intentPlan, "goal: performance", `goal: "performance"`, 1),
	}
	for name, content := range same {
		got, err := IntentHashBasis([]byte(content))
		if err != nil || string(got) != want {
			t.Errorf("%s: basis = %q, %v; want unchanged", name, got, err)
		}
	}
	// The intent itself changes it.
	changed := map[string]string{
		"what and why":   strings.Replace(intentPlan, "Make add faster.", "Make add and sub faster.", 1),
		"goal":           strings.Replace(intentPlan, "goal: performance", "goal: readability", 1),
		"benchmark":      strings.Replace(intentPlan, "benchmark: sh ./bench.sh", "benchmark: sh ./bench2.sh", 1),
		"goal removed":   strings.Replace(intentPlan, "goal: performance\n", "", 1),
		"second section": intentPlan + "\n## What and why\n\nAlso sub.\n",
	}
	for name, content := range changed {
		got, err := IntentHashBasis([]byte(content))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if string(got) == want {
			t.Errorf("%s: basis unchanged; want the edit to change it", name)
		}
	}

	for _, bad := range []string{"no frontmatter\n", "---\nstatus: planned\n", "---\ngoal: [a]\nstatus: planned\n---\n"} {
		if _, err := IntentHashBasis([]byte(bad)); err == nil {
			t.Errorf("IntentHashBasis(%q) succeeded; want an error", bad)
		}
	}
}

func TestHashesIntentOnIntentBasis_AC130(t *testing.T) {
	root := writeFiles(t, map[string]string{planPath: intentPlan})
	basis, err := IntentHashBasis([]byte(intentPlan))
	if err != nil {
		t.Fatal(err)
	}
	got, err := Hashes(root, planPath, gate.Intent, []string{planPath})
	if err != nil || got[planPath] != sha(string(basis)) {
		t.Errorf("Hashes(intent) = %v, %v; want %s", got, err, sha(string(basis)))
	}
	// Every other gate hashes the whole plan.
	got, err = Hashes(root, planPath, gate.Plan, []string{planPath})
	if err != nil || got[planPath] == sha(string(basis)) {
		t.Errorf("Hashes(plan) = %v, %v; want the PlanHashBasis hash", got, err)
	}
}
