package guard

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// names lists every guard, in the order testdata/switches prints them.
var names = []string{"selftest", "approvetty", "approveenv", "approvedirty", "approveowner", "approveconfirm",
	"checkintegrity", "checkfields", "checkgate", "checknounit", "checkchore",
	"evidence", "reprofreeze", "baselinefreeze", "reprocode", "reopen", "hook", "harnessprompt"}

// TestSwitches builds testdata/switches under each tag set and checks that a
// guard is off only with both lyceumtest and its lyceumtest_noguard_<name> tag:
// without lyceumtest, as in a release build, the noguard tag keeps it on.
func TestSwitches(t *testing.T) {
	type tc struct {
		tags string
		off  string
	}
	cases := []tc{{"", ""}, {"lyceumtest", ""}}
	for _, name := range names {
		cases = append(cases,
			tc{"lyceumtest_noguard_" + name, ""},
			tc{"lyceumtest,lyceumtest_noguard_" + name, name})
	}
	for _, c := range cases {
		t.Run("tags="+c.tags, func(t *testing.T) {
			t.Parallel()
			bin := filepath.Join(t.TempDir(), "switches")
			args := []string{"build", "-o", bin}
			if c.tags != "" {
				args = append(args, "-tags", c.tags)
			}
			args = append(args, "./testdata/switches")
			// The test binary runs where it was built, so GOROOT is the go running this test.
			build := exec.Command(filepath.Join(runtime.GOROOT(), "bin", "go"), args...) //nolint:staticcheck // SA1019, see above
			build.Env = append(os.Environ(), "CGO_ENABLED=0")
			if out, err := build.CombinedOutput(); err != nil {
				t.Fatalf("go %v: %v\n%s", args, err, out)
			}
			out, err := exec.Command(bin).Output()
			if err != nil {
				t.Fatalf("%s: %v", bin, err)
			}
			var want strings.Builder
			for _, name := range names {
				fmt.Fprintf(&want, "%s=%t\n", name, name != c.off)
			}
			if got := string(out); got != want.String() {
				t.Errorf("switches printed\n%s\nwant\n%s", got, want.String())
			}
		})
	}
}
