package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
)

var (
	noguardMu   sync.Mutex
	noguardBins = map[string]string{}
)

// noguardBinary returns a test build of lyceum with the named protection turned
// off (-tags lyceumtest,lyceumtest_noguard_<name>), built once per name per test
// run. The binary is named lyceum, alone in its directory, so a guard script can
// put that directory first on PATH.
func noguardBinary(t *testing.T, name string) string {
	t.Helper()
	noguardMu.Lock()
	defer noguardMu.Unlock()
	if bin, ok := noguardBins[name]; ok {
		return bin
	}
	bin := filepath.Join(binDir, "noguard_"+name, "lyceum")
	if err := goBuild("build", "-tags", "lyceumtest,lyceumtest_noguard_"+name, "-o", bin, "./cmd/lyceum"); err != nil {
		t.Fatal(err)
	}
	noguardBins[name] = bin
	return bin
}

func TestNoguardBinary(t *testing.T) {
	bin := noguardBinary(t, "selftest")
	if again := noguardBinary(t, "selftest"); again != bin {
		t.Errorf("second call returned %s; want cached %s", again, bin)
	}
	out, err := exec.Command(bin).Output()
	if err != nil {
		t.Fatalf("%s: %v", bin, err)
	}
	if !strings.Contains(string(out), "Lifecycle:") {
		t.Errorf("%s printed %q; want the menu", bin, out)
	}
}

// runGuard runs testdata/guard/<name>.txtar, the attack on protection name
// that its AC script shows refused, under the binary with that protection off.
// The script asserts the attack succeeds, so the AC script is refused by the
// protection it names and nothing else (TESTING.md §4).
func runGuard(t *testing.T, name string) {
	t.Helper()
	bin := noguardBinary(t, name)
	testscript.Run(t, testscript.Params{
		Files: []string{filepath.Join("testdata", "guard", name+".txtar")},
		Cmds:  map[string]func(*testscript.TestScript, bool, []string){"payload": payload, "ttyrun": ttyRun},
		Setup: func(env *testscript.Env) error {
			if err := setupScript(env); err != nil {
				return err
			}
			env.Setenv("PATH", filepath.Dir(bin)+string(os.PathListSeparator)+env.Getenv("PATH"))
			return nil
		},
	})
}

func TestGuard_Approvetty(t *testing.T)     { runGuard(t, "approvetty") }
func TestGuard_Approveenv(t *testing.T)     { runGuard(t, "approveenv") }
func TestGuard_Approvedirty(t *testing.T)   { runGuard(t, "approvedirty") }
func TestGuard_Approveowner(t *testing.T)   { runGuard(t, "approveowner") }
func TestGuard_Approveconfirm(t *testing.T) { runGuard(t, "approveconfirm") }
func TestGuard_Checkintegrity(t *testing.T) { runGuard(t, "checkintegrity") }
func TestGuard_Checkfields(t *testing.T)    { runGuard(t, "checkfields") }
func TestGuard_Checkgate(t *testing.T)      { runGuard(t, "checkgate") }
func TestGuard_Checknounit(t *testing.T)    { runGuard(t, "checknounit") }
func TestGuard_Checkchore(t *testing.T)     { runGuard(t, "checkchore") }
func TestGuard_Evidence(t *testing.T)       { runGuard(t, "evidence") }
func TestGuard_Reprofreeze(t *testing.T)    { runGuard(t, "reprofreeze") }
func TestGuard_Baselinefreeze(t *testing.T) { runGuard(t, "baselinefreeze") }
func TestGuard_Reprocode(t *testing.T)      { runGuard(t, "reprocode") }
func TestGuard_Reopen(t *testing.T)         { runGuard(t, "reopen") }
func TestGuard_Hook(t *testing.T)           { runGuard(t, "hook") }
func TestGuard_HarnessPrompt(t *testing.T)  { runGuard(t, "harnessprompt") }
