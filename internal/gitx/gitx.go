// Package gitx is the only place Lyceum runs git. It shells out to the user's
// git binary; every call runs under a Timeout.
package gitx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/oritromax/lyceum/internal/out"
)

// Timeout bounds every git call.
const Timeout = 30 * time.Second

// Minimum git version Lyceum supports.
const minMajor, minMinor = 2, 30

// exitError is git exiting non-zero: its code and trimmed stderr.
type exitError struct {
	args   []string
	code   int
	stderr string
}

func (e *exitError) Error() string {
	return fmt.Sprintf("git %s: exit %d: %s", strings.Join(e.args, " "), e.code, e.stderr)
}

// run runs git with args in dir and returns its stdout with surrounding whitespace trimmed.
func run(dir string, args ...string) (string, error) {
	b, err := runRaw(dir, args...)
	return strings.TrimSpace(string(b)), err
}

// runRaw runs git with args in dir and returns its stdout as is.
func runRaw(dir string, args ...string) ([]byte, error) {
	return runInput(dir, nil, args...)
}

// runInput is runRaw with input, if not nil, on git's stdin.
func runInput(dir string, input []byte, args ...string) ([]byte, error) {
	return runEnv(dir, nil, input, args...)
}

// runEnv is runInput with env added to git's environment.
func runEnv(dir string, env []string, input []byte, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	if env != nil {
		cmd.Env = append(os.Environ(), env...)
	}
	if input != nil {
		cmd.Stdin = bytes.NewReader(input)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if ctx.Err() != nil {
		return nil, fmt.Errorf("git %s: timed out after %s", strings.Join(args, " "), Timeout)
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return nil, &exitError{args: args, code: ee.ExitCode(), stderr: strings.TrimSpace(stderr.String())}
	}
	if err != nil {
		return nil, fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return stdout.Bytes(), nil
}

// exitCode reports whether err is git exiting with code.
func exitCode(err error, code int) bool {
	var ee *exitError
	return errors.As(err, &ee) && ee.code == code
}

// CheckVersion refuses when git is missing or older than 2.30.
func CheckVersion() error {
	v, err := run("", "version")
	if errors.Is(err, exec.ErrNotFound) {
		return out.Refuse("run git", "git was not found on PATH", "install git 2.30 or newer, then run the command again")
	}
	if err != nil {
		return err
	}
	return checkVersion(v)
}

// checkVersion checks the output of `git version`, e.g. "git version 2.39.3 (Apple Git-146)"
// or "git version 2.45.1.windows.1".
func checkVersion(v string) error {
	f := strings.Fields(v)
	if len(f) < 3 || f[0] != "git" || f[1] != "version" {
		return fmt.Errorf("unrecognised git version output %q", v)
	}
	parts := strings.SplitN(f[2], ".", 3)
	if len(parts) < 2 {
		return fmt.Errorf("unrecognised git version output %q", v)
	}
	major, err1 := strconv.Atoi(parts[0])
	minor, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return fmt.Errorf("unrecognised git version output %q", v)
	}
	if major < minMajor || major == minMajor && minor < minMinor {
		return out.Refuse("run git", fmt.Sprintf("git %s is older than %d.%d", f[2], minMajor, minMinor),
			fmt.Sprintf("upgrade git to %d.%d or newer, then run the command again", minMajor, minMinor))
	}
	return nil
}

// IsRepo reports whether dir is inside a git work tree.
func IsRepo(dir string) bool {
	v, err := run(dir, "rev-parse", "--is-inside-work-tree")
	return err == nil && v == "true"
}

// Root returns the top-level directory of the work tree holding dir.
func Root(dir string) (string, error) {
	return run(dir, "rev-parse", "--show-toplevel")
}

// Branch returns the current branch, or "" when HEAD is detached. It works in a
// repo with no commits.
func Branch(dir string) (string, error) {
	return run(dir, "branch", "--show-current")
}

// DefaultBranch returns the branch origin's HEAD points at, or the current branch
// when origin has no HEAD.
func DefaultBranch(dir string) (string, error) {
	v, err := run(dir, "symbolic-ref", "-q", "--short", "refs/remotes/origin/HEAD")
	if exitCode(err, 1) {
		return Branch(dir)
	}
	return strings.TrimPrefix(v, "origin/"), err
}

// Init runs `git init` in dir.
func Init(dir string) error {
	_, err := run(dir, "init", "-q")
	return err
}

// GitPath returns where git keeps path of its own, such as "hooks/pre-commit",
// as `git rev-parse --git-path` gives it: relative to dir unless absolute. It
// follows core.hooksPath and linked work trees.
func GitPath(dir, path string) (string, error) {
	return run(dir, "rev-parse", "--git-path", path)
}

// HeadSHA returns HEAD's short SHA, or "" in a repo with no commits.
func HeadSHA(dir string) (string, error) {
	v, err := run(dir, "rev-parse", "-q", "--verify", "--short", "HEAD")
	if exitCode(err, 1) {
		return "", nil
	}
	return v, err
}

// UserEmail returns `git config user.email`, or "" when it is unset.
func UserEmail(dir string) (string, error) {
	v, err := run(dir, "config", "user.email")
	if exitCode(err, 1) {
		return "", nil
	}
	return v, err
}

// Status returns the paths `git status --porcelain` reports as changed, staged or
// untracked, relative to the work tree root. Untracked directories are listed file
// by file; a rename or copy lists its new path.
func Status(dir string) ([]string, error) {
	b, err := runRaw(dir, "status", "--porcelain", "-z", "--untracked-files=all")
	if err != nil {
		return nil, err
	}
	var paths []string
	entries := strings.Split(string(b), "\x00")
	for i := 0; i < len(entries); i++ {
		e := entries[i]
		if len(e) < 4 {
			continue // the empty string after the final NUL
		}
		paths = append(paths, e[3:])
		if strings.ContainsAny(e[:2], "RC") {
			i++ // the next entry is the original path
		}
	}
	return paths, nil
}

// Untracked returns those of paths git does not track and does not ignore.
func Untracked(dir string, paths []string) ([]string, error) {
	b, err := runRaw(dir, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, err
	}
	var kept []string
	for _, p := range strings.Split(string(b), "\x00") {
		if p != "" && slices.Contains(paths, p) {
			kept = append(kept, p)
		}
	}
	return kept, nil
}

// BranchExists reports whether the local branch name exists.
func BranchExists(dir, name string) (bool, error) {
	_, err := run(dir, "rev-parse", "-q", "--verify", "refs/heads/"+name)
	if exitCode(err, 1) {
		return false, nil
	}
	return err == nil, err
}

// BaseRef returns the ref to diff against for base branch base: the local
// branch, else the remote-tracking origin/<base>, which is often all CI has,
// or "" when neither exists.
func BaseRef(dir, base string) (string, error) {
	for _, ref := range []string{"refs/heads/" + base, "refs/remotes/origin/" + base} {
		_, err := run(dir, "rev-parse", "-q", "--verify", ref)
		if err == nil {
			return ref, nil
		}
		if !exitCode(err, 1) {
			return "", err
		}
	}
	return "", nil
}

// RemoteURL returns the URL of remote name, or "" when there is no such remote.
func RemoteURL(dir, name string) (string, error) {
	v, err := run(dir, "config", "--get", "remote."+name+".url")
	if exitCode(err, 1) {
		return "", nil
	}
	return v, err
}

// Upstream returns the upstream of local branch name, such as "origin/main",
// or "" when it has none.
func Upstream(dir, name string) (string, error) {
	return run(dir, "for-each-ref", "--format=%(upstream:short)", "refs/heads/"+name)
}

// Branches returns the names of the local branches.
func Branches(dir string) ([]string, error) {
	v, err := run(dir, "for-each-ref", "--format=%(refname:short)", "refs/heads/")
	if err != nil || v == "" {
		return nil, err
	}
	return strings.Split(v, "\n"), nil
}

// TreeFiles returns the paths of the files under path in the tree of ref,
// relative to the work tree root.
func TreeFiles(dir, ref, path string) ([]string, error) {
	v, err := run(dir, "ls-tree", "-r", "--name-only", ref, "--", path)
	if err != nil || v == "" {
		return nil, err
	}
	return strings.Split(v, "\n"), nil
}

// Files returns the paths of the files in the work tree that git tracks or
// would track, those it ignores left out, relative to the work tree root.
func Files(dir string) ([]string, error) {
	b, err := runRaw(dir, "ls-files", "-z", "--cached", "--others", "--exclude-standard", "--full-name", ":/")
	if err != nil {
		return nil, err
	}
	return strings.FieldsFunc(string(b), func(r rune) bool { return r == 0 }), nil
}

// Refs returns the full names of the local and remote-tracking branches, in
// git's order, leaving out symbolic refs such as refs/remotes/origin/HEAD.
func Refs(dir string) ([]string, error) {
	v, err := run(dir, "for-each-ref", "--format=%(refname) %(symref)", "refs/heads/", "refs/remotes/")
	if err != nil || v == "" {
		return nil, err
	}
	var refs []string
	for _, line := range strings.Split(v, "\n") {
		if name, symref, _ := strings.Cut(line, " "); symref == "" {
			refs = append(refs, name)
		}
	}
	return refs, nil
}

// TreeEntry is a file in a git tree: its path, relative to the work tree root,
// and its blob ID.
type TreeEntry struct {
	Path, Blob string
}

// TreeBlobs returns the files directly in directory path of ref's tree, or none
// when ref has no such directory.
func TreeBlobs(dir, ref, path string) ([]TreeEntry, error) {
	b, err := runRaw(dir, "ls-tree", "-z", ref, "--", strings.TrimSuffix(path, "/")+"/")
	if err != nil {
		return nil, err
	}
	var entries []TreeEntry
	for _, e := range strings.Split(string(b), "\x00") {
		// "<mode> <type> <object>\t<path>"
		meta, p, ok := strings.Cut(e, "\t")
		if f := strings.Fields(meta); ok && len(f) == 3 && f[1] == "blob" {
			entries = append(entries, TreeEntry{Path: p, Blob: f[2]})
		}
	}
	return entries, nil
}

// Blobs returns the contents of each blob in ids, keyed by ID, read with one
// `git cat-file --batch`.
func Blobs(dir string, ids []string) (map[string][]byte, error) {
	blobs := make(map[string][]byte, len(ids))
	if len(ids) == 0 {
		return blobs, nil
	}
	b, err := runInput(dir, []byte(strings.Join(ids, "\n")+"\n"), "cat-file", "--batch")
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		// Each blob is "<id> blob <size>\n<contents>\n"; a missing one is "<id> missing\n".
		header, rest, ok := bytes.Cut(b, []byte("\n"))
		f := strings.Fields(string(header))
		if !ok || len(f) != 3 || f[1] != "blob" {
			return nil, fmt.Errorf("git cat-file --batch: object %s: unexpected header %q", id, header)
		}
		size, err := strconv.Atoi(f[2])
		if err != nil || size+1 > len(rest) {
			return nil, fmt.Errorf("git cat-file --batch: object %s: bad size in header %q", id, header)
		}
		blobs[id] = rest[:size]
		b = rest[size+1:]
	}
	return blobs, nil
}

// SwitchNew creates branch name at start and checks it out.
func SwitchNew(dir, name, start string) error {
	_, err := run(dir, "switch", "-q", "-c", name, start)
	return err
}

// OwnCommitEnv is set on the commits Lyceum makes itself, so the pre-commit
// hook they fire can tell them from someone's own commit.
const OwnCommitEnv = "LYCEUM_OWN_COMMIT"

// Commit stages paths and commits them, and only them, with message: anything
// else already staged stays staged and out of the commit.
func Commit(dir, message string, paths ...string) error {
	if _, err := run(dir, append([]string{"add", "--"}, paths...)...); err != nil {
		return err
	}
	_, err := runEnv(dir, []string{OwnCommitEnv + "=1"}, nil, append([]string{"commit", "-q", "-m", message, "--"}, paths...)...)
	return err
}

// CommitSigned is Commit with the commit signed by the user's key.
func CommitSigned(dir, message string, paths ...string) error {
	if _, err := run(dir, append([]string{"add", "--"}, paths...)...); err != nil {
		return err
	}
	_, err := runEnv(dir, []string{OwnCommitEnv + "=1"}, nil, append([]string{"commit", "-q", "-S", "-m", message, "--"}, paths...)...)
	return err
}

// CanSign reports, with git's error, whether git can sign a commit here. It
// signs a commit object no ref reaches.
func CanSign(dir string) error {
	_, err := run(dir, "commit-tree", "-S", "-m", "lyceum: signing probe", "HEAD^{tree}")
	return err
}

// IntroducedBy returns the oldest commit reachable from HEAD that added text
// to path, or "" when none did.
func IntroducedBy(dir, path, text string) (string, error) {
	v, err := run(dir, "log", "--format=%H", "-S"+text, "--", path)
	if err != nil || v == "" {
		return "", err
	}
	lines := strings.Split(v, "\n")
	return lines[len(lines)-1], nil
}

// Signature returns git's verdict on commit's signature (%G?: "G" is a good
// signature git trusts) and the signer it names (%GS).
func Signature(dir, commit string) (status, signer string, err error) {
	v, err := run(dir, "log", "-1", "--format=%G?%n%GS", commit)
	if err != nil {
		return "", "", err
	}
	status, signer, _ = strings.Cut(v, "\n")
	return status, signer, nil
}

// Diff returns `git diff <commit> -- <paths>`: the changes to paths from commit
// to the working tree. Untracked files are not in it.
func Diff(dir, commit string, paths ...string) (string, error) {
	b, err := runRaw(dir, append([]string{"diff", "--no-color", commit, "--"}, paths...)...)
	return string(b), err
}

// MergeBase returns the short SHA of the merge base of ref and HEAD.
func MergeBase(dir, ref string) (string, error) {
	v, err := run(dir, "merge-base", ref, "HEAD")
	if err != nil {
		return "", err
	}
	return run(dir, "rev-parse", "--short", v)
}

// FirstAfter returns the first commit after commit on the ancestry path to
// HEAD: the commit made on top of commit, or "" when HEAD is commit.
func FirstAfter(dir, commit string) (string, error) {
	v, err := run(dir, "rev-list", "--reverse", "--ancestry-path", commit+"..HEAD")
	if err != nil {
		return "", err
	}
	first, _, _ := strings.Cut(v, "\n")
	return first, nil
}

// Show returns the contents of path, relative to the work tree root, in rev's tree.
func Show(dir, rev, path string) ([]byte, error) {
	return runRaw(dir, "show", rev+":"+path)
}

// CommittedSince returns the paths any commit in commit..HEAD touches, relative
// to the work tree root, sorted and without duplicates.
func CommittedSince(dir, commit string) ([]string, error) {
	b, err := runRaw(dir, "log", "--format=", "--name-only", "-z", commit+"..HEAD")
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, p := range strings.Split(string(b), "\x00") {
		if p = strings.Trim(p, "\n"); p != "" {
			paths = append(paths, p)
		}
	}
	slices.Sort(paths)
	return slices.Compact(paths), nil
}

// BranchChanges returns the paths the branch changes against base: those
// `git diff --name-only <merge-base of base and HEAD>...HEAD` lists, plus the
// uncommitted ones Status lists, relative to the work tree root, sorted and
// without duplicates.
func BranchChanges(dir, base string) ([]string, error) {
	committed, err := BranchCommitted(dir, base)
	if err != nil {
		return nil, err
	}
	paths, err := Status(dir)
	if err != nil {
		return nil, err
	}
	paths = append(paths, committed...)
	slices.Sort(paths)
	return slices.Compact(paths), nil
}

// BranchCommitted returns the paths `git diff --name-only <merge-base of base
// and HEAD>...HEAD` lists: what the branch changes against base, committed.
func BranchCommitted(dir, base string) ([]string, error) {
	b, err := runRaw(dir, "diff", "--name-only", "-z", base+"...HEAD")
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, p := range strings.Split(string(b), "\x00") {
		if p != "" {
			paths = append(paths, p)
		}
	}
	return paths, nil
}

// HashFiles returns the blob ID git gives the current content of each file in
// paths, relative to dir, in order, read with one `git hash-object
// --stdin-paths`. It writes nothing to the object store.
func HashFiles(dir string, paths []string) ([]string, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	b, err := runInput(dir, []byte(strings.Join(paths, "\n")+"\n"), "hash-object", "--stdin-paths")
	if err != nil {
		return nil, err
	}
	ids := strings.Fields(string(b))
	if len(ids) != len(paths) {
		return nil, fmt.Errorf("git hash-object --stdin-paths: %d IDs for %d paths", len(ids), len(paths))
	}
	return ids, nil
}

// FilesUnder returns the files under each of paths, relative to dir, that git
// tracks or would track, those it ignores left out, as Files lists them. A
// tracked file deleted from the work tree is still listed.
func FilesUnder(dir string, paths []string) ([]string, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	b, err := runRaw(dir, append([]string{"--literal-pathspecs", "ls-files", "-z", "--cached", "--others", "--exclude-standard", "--"}, paths...)...)
	if err != nil {
		return nil, err
	}
	files := strings.FieldsFunc(string(b), func(r rune) bool { return r == 0 })
	slices.Sort(files)
	return slices.Compact(files), nil // an unmerged file is listed once per stage
}

// ObjectFormat returns the repo's object hash: "sha1" or "sha256".
func ObjectFormat(dir string) (string, error) {
	return run(dir, "rev-parse", "--show-object-format")
}

// Staged returns the paths staged in the index, relative to the work tree root.
func Staged(dir string) ([]string, error) {
	b, err := runRaw(dir, "diff", "--cached", "--name-only", "-z")
	if err != nil {
		return nil, err
	}
	return strings.FieldsFunc(string(b), func(r rune) bool { return r == 0 }), nil
}

// Tracked reports whether git tracks path, relative to dir.
func Tracked(dir, path string) (bool, error) {
	v, err := run(dir, "--literal-pathspecs", "ls-files", "--", path)
	return v != "", err
}

// LogCommit is one commit as Log returns it.
type LogCommit struct {
	SHA, Short string
	Time       time.Time // committer date, UTC
	Author     string    // author email
	Subject    string
}

// Log returns the commits `git log <args>` lists, newest first, such as
// Log(dir, "--branches") for every local branch's.
func Log(dir string, args ...string) ([]LogCommit, error) {
	b, err := runRaw(dir, append([]string{"log", "--format=%H%x00%h%x00%cI%x00%ae%x00%s"}, args...)...)
	if err != nil {
		return nil, err
	}
	var commits []LogCommit
	for _, line := range strings.Split(strings.TrimSuffix(string(b), "\n"), "\n") {
		f := strings.SplitN(line, "\x00", 5)
		if len(f) != 5 {
			continue
		}
		t, err := time.Parse(time.RFC3339, f[2])
		if err != nil {
			return nil, fmt.Errorf("git log: commit %s: %w", f[1], err)
		}
		commits = append(commits, LogCommit{SHA: f[0], Short: f[1], Time: t.UTC(), Author: f[3], Subject: f[4]})
	}
	return commits, nil
}
