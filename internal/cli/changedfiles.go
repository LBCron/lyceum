package cli

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"strings"

	"github.com/oritromax/lyceum/internal/config"
	"github.com/oritromax/lyceum/internal/gitx"
	"github.com/oritromax/lyceum/internal/out"
	"github.com/spf13/cobra"
)

// runChangedFiles is the internal `lyceum changed-files`: the paths the
// branch changes against its base, resolved as check resolves it, plus
// the uncommitted ones, sorted and deduped. It prints one path per line and
// nothing else, so a TESTING.md subset can use it as `$(lyceum changed-files)`.
func runChangedFiles(cmd *cobra.Command, args []string) error {
	what := "list changed files"
	if len(args) > 0 {
		return out.Refuse(what, "changed-files takes no arguments", `run "lyceum changed-files"`)
	}
	if err := gitx.CheckVersion(); err != nil {
		return err
	}
	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	if !gitx.IsRepo(dir) {
		return out.Refuse(what, "this is not a git repository, so Lyceum is not set up here", `run "lyceum init"`)
	}
	root, err := gitx.Root(dir)
	if err != nil {
		return err
	}
	cfg, err := config.Read(root)
	if errors.Is(err, fs.ErrNotExist) {
		return out.Refuse(what, "Lyceum is not set up in this repo: "+config.Path+" is missing", `run "lyceum init"`)
	} else if err != nil {
		return err
	}
	base, err := baseRef(root, cfg.BaseBranch, what, false)
	if err != nil {
		return err
	}
	paths, err := gitx.BranchChanges(root, base)
	if err != nil {
		return err
	}
	if paths == nil {
		paths = []string{}
	}
	if asJSON, _ := cmd.Flags().GetBool("json"); asJSON {
		out.Writer{Stdout: cmd.OutOrStdout(), Stderr: cmd.ErrOrStderr(), JSON: true}.Result(out.Result{
			Next: `run "lyceum test"`,
			Data: struct {
				Files []string `json:"files"`
			}{paths},
		})
		return nil
	}
	if len(paths) > 0 {
		_, _ = io.WriteString(cmd.OutOrStdout(), strings.Join(paths, "\n")+"\n")
	}
	return nil
}
