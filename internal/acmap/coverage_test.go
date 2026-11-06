package acmap

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// TestACCoverage is the mechanical form of TESTING.md principle 2: every AC of an
// implemented or closed plan, or with LYCEUM_RELEASE_COVERAGE=1 every AC in the spec,
// is mapped to a test, or for a live-only AC to the latest release check.
func TestACCoverage(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Fatal(err)
	}
	missing, err := unmapped(root, os.Getenv("LYCEUM_RELEASE_COVERAGE") == "1")
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) > 0 {
		t.Errorf("%d required ACs have no mapped test: %s", len(missing), strings.Join(missing, ", "))
	}
}

func TestACCoverageFixture(t *testing.T) {
	root := filepath.Join("testdata", "coverage")
	for _, c := range []struct {
		release bool
		want    []string
	}{
		{false, []string{"AC9002"}},
		// AC9004 is mapped, but only an implemented plan's acs_partial claims it (AC150).
		{true, []string{"AC" + "91", "AC9002", "AC9003", "AC9004"}},
	} {
		got, err := unmapped(root, c.release)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("release=%v: unmapped = %v, want %v", c.release, got, c.want)
		}
	}
}

// liveTwin names a live-only AC by concatenation so this file does not itself map it.
const liveTwin = "AC" + "4"

// platformTwin names a live-only packaging AC the same way.
const platformTwin = "AC" + "83"

func TestReleaseChecked(t *testing.T) {
	dir := filepath.Join("testdata", "releasecheck")
	for _, c := range []struct {
		dir  string
		want map[string]bool
	}{
		// Five PASS lines map a live-only AC; five for a non-live-only AC, or two, do not.
		{filepath.Join(dir, "pass"), map[string]bool{liveTwin: true}},
		{filepath.Join(dir, "missing"), map[string]bool{}},
		{filepath.Join(dir, "fail"), map[string]bool{}},
		// The latest file is the greatest date, then the greatest name; an older pass does not count.
		{filepath.Join(dir, "latest"), map[string]bool{}},
		{filepath.Join(dir, "absent"), map[string]bool{}},
		// A packaging AC needs a PASS line for each of the four v1 platforms and no FAIL.
		{filepath.Join(dir, "platform-pass"), map[string]bool{platformTwin: true}},
		{filepath.Join(dir, "platform-missing"), map[string]bool{}},
		{filepath.Join(dir, "platform-fail"), map[string]bool{}},
		// Harness lines do not count for a packaging AC, nor platform lines for a harness AC.
		{filepath.Join(dir, "platform-harness"), map[string]bool{}},
		{t.TempDir(), map[string]bool{}},
	} {
		got, err := ReleaseChecked(c.dir)
		if err != nil {
			t.Fatalf("%s: %v", c.dir, err)
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: ReleaseChecked = %v, want %v", c.dir, got, c.want)
		}
	}
}

func TestReleaseCheckedMalformed(t *testing.T) {
	for _, body := range []string{
		"# no results\n",
		"## Results\n\n## Next\n",
		"## Results\n\n```\nRESULT|pi|" + liveTwin + "|PASS\n",
		"## Results\n\n```\nRESULT|pi|" + liveTwin + "|SKIP\n```\n",
		"## Results\n\n```\nRESULT|pi|" + liveTwin + "\n```\n",
	} {
		dir := t.TempDir()
		write(t, filepath.Join(dir, "m3-2026-10-25.md"), body)
		if _, err := ReleaseChecked(dir); err == nil {
			t.Errorf("ReleaseChecked(%q): want error", body)
		}
	}
}

// TestLiveOnlyMatchesTesting fails when liveOnly drifts from the list in TESTING.md §6.
func TestLiveOnlyMatchesTesting(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "TESTING.md"))
	if err != nil {
		t.Fatal(err)
	}
	_, sec, ok := strings.Cut(string(data), "\n## 6. Coverage\n")
	if !ok {
		t.Fatal("TESTING.md has no ## 6. Coverage section")
	}
	sec, _, _ = strings.Cut(sec, "\n## ")
	got := wholeWordIDs([]byte(sec))
	sortIDs(got)
	if !reflect.DeepEqual(got, liveOnly) {
		t.Errorf("TESTING.md §6 lists %v, liveOnly is %v", got, liveOnly)
	}
}

// unmapped returns the required ACs under root with no mapped test, sorted numerically.
func unmapped(root string, release bool) ([]string, error) {
	var required []string
	if release {
		// A withdrawn or deferred row is not required (AC153).
		ids, err := OpenSpecACs(filepath.Join(root, "lyceum", "spec"))
		if err != nil {
			return nil, err
		}
		required = ids
	} else {
		ids, err := planACs(filepath.Join(root, "lyceum", "plan"))
		if err != nil {
			return nil, err
		}
		required = ids
	}
	globs, err := TestingGlobs(filepath.Join(root, "TESTING.md"))
	if err != nil {
		return nil, err
	}
	mapped, err := Mapped(root, globs)
	if err != nil {
		return nil, err
	}
	checked, err := ReleaseChecked(filepath.Join(root, "release-checks"))
	if err != nil {
		return nil, err
	}
	partial := map[string]bool{}
	if release {
		if partial, err = partialACs(filepath.Join(root, "lyceum", "plan")); err != nil {
			return nil, err
		}
	}
	seen := map[string]bool{}
	var missing []string
	for _, id := range required {
		if (len(mapped[id]) == 0 && !checked[id] || partial[id]) && !seen[id] {
			seen[id] = true
			missing = append(missing, id)
		}
	}
	sortIDs(missing)
	return missing, nil
}

// partialACs returns the ACs an implemented or closed plan in planDir claims
// in part, in acs_partial, and none claims whole, in acs.
func partialACs(planDir string) (map[string]bool, error) {
	files, err := filepath.Glob(filepath.Join(planDir, "*.md"))
	if err != nil {
		return nil, err
	}
	open := map[string]bool{}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		fm, err := frontmatter(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		var plan struct {
			Status  string   `yaml:"status"`
			Partial []string `yaml:"acs_partial"`
		}
		if err := yaml.Unmarshal(fm, &plan); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		if plan.Status == "implemented" || plan.Status == "closed" {
			for _, id := range plan.Partial {
				open[id] = true
			}
		}
	}
	whole, err := planACs(planDir)
	if err != nil {
		return nil, err
	}
	for _, id := range whole {
		delete(open, id)
	}
	return open, nil
}

// planACs returns the acs of every plan in planDir whose status is implemented or closed.
func planACs(planDir string) ([]string, error) {
	files, err := filepath.Glob(filepath.Join(planDir, "*.md"))
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		fm, err := frontmatter(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		var plan struct {
			Status string   `yaml:"status"`
			ACs    []string `yaml:"acs"`
		}
		if err := yaml.Unmarshal(fm, &plan); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		if plan.Status == "implemented" || plan.Status == "closed" {
			ids = append(ids, plan.ACs...)
		}
	}
	return ids, nil
}

// repoRoot walks up from the working directory to the directory holding go.mod.
func repoRoot() (string, error) {
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
