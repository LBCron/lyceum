package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/oritromax/lyceum/internal/approval"
	"github.com/oritromax/lyceum/internal/check"
	"github.com/oritromax/lyceum/internal/config"
	"github.com/oritromax/lyceum/internal/events"
	"github.com/oritromax/lyceum/internal/gitx"
	"github.com/oritromax/lyceum/internal/out"
	"github.com/oritromax/lyceum/internal/records"
	"github.com/oritromax/lyceum/internal/testhooks"
	"github.com/oritromax/lyceum/internal/unit"
	"github.com/spf13/cobra"
)

// runRecord is the internal `lyceum record <file>` (commands.md, "Internal";
// records.md, "CLI and agent split"). The file is a record the agent
// wrote directly in lyceum/understand/, lyceum/trace/ or lyceum/audit/, with
// cites: paths only. It validates the record, refusing naming the field, then
// fills in what is missing of id and created and every cite's blob or tree,
// renames a record given its id here to <id>.md, and commits that file, and
// only it, as "lyceum: record <id>". It refuses while any other file is
// staged. It needs no unit.
func runRecord(cmd *cobra.Command, args []string) error {
	what := "record"
	if len(args) != 1 {
		return out.Refuse(what, "record takes one argument, the record file",
			`run "lyceum record lyceum/<understand|trace|audit>/<file>.md"`)
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
	if _, err := config.Read(root); errors.Is(err, fs.ErrNotExist) {
		return out.Refuse(what, "Lyceum is not set up in this repo: "+config.Path+" is missing", `run "lyceum init"`)
	} else if err != nil {
		return err
	}

	abs := args[0]
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(dir, abs)
	}
	file, err := filepath.EvalSymlinks(abs)
	if errors.Is(err, fs.ErrNotExist) {
		return out.Refuse(what, args[0]+" does not exist", "write the record first, then run \"lyceum record <file>\"")
	} else if err != nil {
		return err
	}
	relOS, err := filepath.Rel(root, file)
	if err != nil {
		return err
	}
	rel := filepath.ToSlash(relOS)
	kind := ""
	for _, k := range records.Kinds {
		if path.Dir(rel) == records.Dir(k) && strings.HasSuffix(rel, ".md") {
			kind = k
		}
	}
	if kind == "" && path.Dir(rel) == records.SpecDir && strings.HasSuffix(rel, ".md") {
		return recordArea(cmd, root, file, rel)
	}
	if kind == "" {
		return out.Refuse(what, rel+" is not a .md file directly in lyceum/understand/, lyceum/trace/ or lyceum/audit/, or an area file in lyceum/spec/",
			"move the record into the folder of its kind, then run \"lyceum record <file>\" again")
	}

	target, id, committed, err := recordFile(root, file, rel, kind)
	if err != nil {
		return err
	}
	res := out.Result{Next: fmt.Sprintf("ask the user whether to export it to docs/: lyceum export %s", target),
		Data: struct {
			Record string `json:"record"`
			ID     string `json:"id"`
		}{target, id}}
	asJSON, _ := cmd.Flags().GetBool("json")
	if !asJSON && committed {
		_, _ = io.WriteString(cmd.OutOrStdout(), "Recorded "+target+"\n")
	} else if !asJSON {
		_, _ = io.WriteString(cmd.OutOrStdout(), target+" is already recorded: nothing changed\n")
	}
	out.Writer{Stdout: cmd.OutOrStdout(), Stderr: cmd.ErrOrStderr(), JSON: asJSON}.Result(res)
	return nil
}

// recordFile is the record path of `lyceum record` for file, a record of
// kind at rel: it validates the record, refusing naming the field, fills in
// what is missing of id and created and every cite's blob or tree, renames a
// record given its id here to <id>.md, and commits that file, and only it.
// It returns where the record is now, its id, and whether it committed: an
// unchanged record commits nothing.
func recordFile(root, file, rel, kind string) (target, id string, committed bool, err error) {
	what := "record"
	data, err := os.ReadFile(file)
	if err != nil {
		return "", "", false, err
	}
	again := fmt.Sprintf(`fix it in %s, then run "lyceum record %s" again`, rel, rel)
	r, err := records.Parse(data)
	if err == nil {
		err = r.Validate()
	}
	if err == nil && r.Kind != kind {
		err = &records.FieldError{Field: "kind", Reason: fmt.Sprintf("%q, but the record is in %s/", r.Kind, records.Dir(kind))}
	}
	if err != nil {
		return "", "", false, out.Refuse(what, rel+" is not a valid record: "+err.Error(), again)
	}

	if err := onlyStaged(root, rel); err != nil {
		return "", "", false, err
	}
	cur, err := citeHashes(root, rel, r.Cites, "record")
	if err != nil {
		return "", "", false, err
	}

	id, target = r.ID, rel
	if id == "" {
		slug, err := unit.Slug(r.Title)
		if err != nil {
			return "", "", false, out.Refuse(what, rel+" is not a valid record: field title: "+err.Error(), again)
		}
		taken, err := recordHashes(root)
		if err != nil {
			return "", "", false, err
		}
		hash := unit.NewHash()
		for taken[hash] {
			hash = unit.NewHash()
		}
		id = unit.ID(hash, slug)
		target = path.Join(records.Dir(kind), id+".md")
	}
	created := r.Created
	if created == "" {
		created = testhooks.Now().Format(time.RFC3339)
	}
	if err := r.Fill(id, created, cur); err != nil {
		return "", "", false, err
	}
	filled, err := r.Bytes()
	if err != nil {
		return "", "", false, err
	}

	tracked, err := gitx.Tracked(root, rel)
	if err != nil {
		return "", "", false, err
	}
	commit := []string{target}
	if target != rel {
		if err := os.Rename(file, filepath.Join(root, filepath.FromSlash(target))); err != nil {
			return "", "", false, err
		}
		if tracked {
			commit = append(commit, rel)
		}
	}
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(target)), filled, 0o644); err != nil {
		return "", "", false, err
	}

	if head, err := gitx.Show(root, "HEAD", rel); tracked && target == rel && err == nil && bytes.Equal(head, filled) {
		return target, id, false, nil
	}
	if err := gitx.Commit(root, "lyceum: record "+id, commit...); err != nil {
		return "", "", false, err
	}
	return target, id, true, nil
}

// onlyStaged refuses unless nothing but rel is staged: record commits only rel.
func onlyStaged(root, rel string) error {
	staged, err := gitx.Staged(root)
	if err != nil {
		return err
	}
	if others := slices.DeleteFunc(staged, func(p string) bool { return p == rel }); len(others) > 0 {
		return out.Refuse("record", "other files are staged, and record commits only the record:\n  "+strings.Join(others, "\n  "),
			"unstage them: git restore --staged -- "+strings.Join(others, " ")+`, then run "lyceum record `+rel+`" again`)
	}
	return nil
}

// citeHashes returns the current hash of each of cites' paths, refusing,
// naming the field, when one does not exist. noun is what rel is ("record",
// "area file").
func citeHashes(root, rel string, cites []records.Cite, noun string) (map[string]records.Current, error) {
	var paths []string
	for _, c := range cites {
		paths = append(paths, c.Path)
	}
	cur, err := records.Hash(root, paths)
	if err != nil {
		return nil, err
	}
	for i, c := range cites {
		if cur[strings.TrimSuffix(c.Path, "/")].Missing() {
			return nil, out.Refuse("record", fmt.Sprintf("%s is not a valid %s: field cites[%d].path: %s does not exist", rel, noun, i, c.Path),
				fmt.Sprintf(`fix it in %s, then run "lyceum record %s" again`, rel, rel))
		}
	}
	return cur, nil
}

// recordArea is `lyceum record lyceum/spec/<area>.md` (records.md, "As-built
// spec sections"): for a registered area file with cites:, it fills in
// each cite's blob or tree and nothing else, and commits that file, and only
// it, as "lyceum: record <file>". Area files are approved by hash, so it says
// this is a spec edit and names the approval that covered the file.
func recordArea(cmd *cobra.Command, root, file, rel string) error {
	what := "record"
	again := fmt.Sprintf(`fix it in %s, then run "lyceum record %s" again`, rel, rel)
	areas, err := approval.RegisteredAreas(root)
	if err != nil {
		return err
	}
	if !slices.Contains(areas, rel) {
		name := strings.TrimSuffix(path.Base(rel), ".md")
		return out.Refuse(what, fmt.Sprintf("%s is not in the area registry of %s", rel, approval.SpecPath),
			"register it: lyceum plan --new-area "+name)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	if !records.HasFrontmatter(data) {
		return out.Refuse(what, rel+" has no frontmatter, so it has no cites:", "add cites: frontmatter listing the cited paths, then run \"lyceum record "+rel+"\" again")
	}
	r, err := records.Parse(data)
	if err == nil {
		err = records.ValidateCites(r.Cites)
	}
	if err != nil {
		return out.Refuse(what, rel+" is not a valid area file: "+err.Error(), again)
	}
	if len(r.Cites) == 0 {
		return out.Refuse(what, rel+" has no cites:", "add cites: frontmatter listing the cited paths, then run \"lyceum record "+rel+"\" again")
	}
	if err := onlyStaged(root, rel); err != nil {
		return err
	}
	cur, err := citeHashes(root, rel, r.Cites, "area file")
	if err != nil {
		return err
	}
	if err := r.FillCites(cur); err != nil {
		return err
	}
	filled, err := r.Bytes()
	if err != nil {
		return err
	}
	if err := os.WriteFile(file, filled, 0o644); err != nil {
		return err
	}

	evs, err := events.Read(root)
	if err != nil {
		return err
	}
	var covering *events.Event
	for _, e := range check.Counting(evs) {
		if hashes, _ := e.Fields["hashes"].(map[string]any); e.Type == "approve" && hashes[rel] != nil {
			covering = &e
		}
	}
	edit := "This is a spec edit: no approval covers " + rel + " yet\n"
	next, g, of := `run "lyceum status"`, "", ""
	if covering != nil {
		g, _ = covering.Fields["gate"].(string)
		if covering.Unit != "" {
			of = " of unit " + covering.Unit
		}
		edit = fmt.Sprintf("This is a spec edit: the %s approval%s covered %s, so check flags it until the owner re-approves it\n", g, of, rel)
		next = fmt.Sprintf(`the owner runs "lyceum approve %s" in a terminal`, g)
		if g == string(approval.Project) {
			branch, err := gitx.Branch(root)
			if err != nil {
				return err
			}
			p, err := openUnit(root, branch)
			if err != nil {
				return err
			}
			next = check.ReapproveStep(p, []string{rel}, rel)
		}
	}
	res := out.Result{Next: next, Data: struct {
		Area string `json:"area"`
		// Approval is the gate whose approval covered the area file, or "".
		Approval string `json:"approval"`
	}{rel, g}}

	asJSON, _ := cmd.Flags().GetBool("json")
	tracked, err := gitx.Tracked(root, rel)
	if err != nil {
		return err
	}
	if head, err := gitx.Show(root, "HEAD", rel); tracked && err == nil && bytes.Equal(head, filled) {
		if !asJSON {
			_, _ = io.WriteString(cmd.OutOrStdout(), rel+" is already recorded: nothing changed\n")
		}
		out.Writer{Stdout: cmd.OutOrStdout(), Stderr: cmd.ErrOrStderr(), JSON: asJSON}.Result(res)
		return nil
	}
	if err := gitx.Commit(root, "lyceum: record "+rel, rel); err != nil {
		return err
	}
	if !asJSON {
		_, _ = io.WriteString(cmd.OutOrStdout(), "Recorded "+rel+"\n"+edit)
	}
	out.Writer{Stdout: cmd.OutOrStdout(), Stderr: cmd.ErrOrStderr(), JSON: asJSON}.Result(res)
	return nil
}

// recordHashes returns the hashes of the record files' names in every record
// folder: the IDs a new record must not reuse.
func recordHashes(root string) (map[string]bool, error) {
	taken := map[string]bool{}
	for _, k := range records.Kinds {
		des, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(records.Dir(k))))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, de := range des {
			if h, _, ok := strings.Cut(de.Name(), "-"); ok {
				taken[h] = true
			}
		}
	}
	return taken, nil
}
