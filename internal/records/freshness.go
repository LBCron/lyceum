package records

import (
	"bytes"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"syscall"

	"github.com/oritromax/lyceum/internal/gitx"
)

// State is a record's freshness (records.md, "Freshness").
type State string

// Freshness states. Invalid is a record or area file that can't be checked:
// unparsable, or not yet filled in by `lyceum record`.
const (
	Fresh   State = "fresh"
	Drifted State = "drifted"
	Broken  State = "broken"
	Invalid State = "invalid"
)

// Current is a cited path's current hash in the work tree: Blob for a file,
// Tree for a folder, neither when the path does not exist.
type Current struct {
	Blob, Tree string
}

// Missing reports whether the path does not exist.
func (c Current) Missing() bool { return c.Blob == "" && c.Tree == "" }

// Hash returns the current hash of each of paths, relative to root, keyed by
// the path without a trailing "/". It never reads a commit, so it gives the
// same answer for the same content on any branch or after any merge (AC73).
//
// A file's hash is the blob ID `git hash-object` gives its current content. A
// folder's hash is the tree ID git would give its current content: every file
// under it that git tracks or would track (`git ls-files --cached --others
// --exclude-standard`), as it is now in the work tree, deleted files left out,
// each a regular file (mode 100644) or a symlink (120000). Modes count for
// nothing else, so a chmod is not drift. For a clean folder with no executable
// files it equals `git rev-parse HEAD:<folder>`. It runs at most one git
// ls-files and one git hash-object for all the paths.
func Hash(root string, paths []string) (map[string]Current, error) {
	cur := map[string]Current{}
	seen, isFile := map[string]bool{}, map[string]bool{}
	var files, dirs []string
	for _, p := range paths {
		k := key(p)
		if seen[k] {
			continue
		}
		seen[k] = true
		fi, err := os.Lstat(filepath.Join(root, filepath.FromSlash(k)))
		switch {
		// ENOTDIR: a path below a file does not exist either.
		case errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR):
			cur[k] = Current{}
		case err != nil:
			return nil, err
		case fi.IsDir():
			dirs = append(dirs, k)
		default:
			files = append(files, k)
			isFile[k] = true
		}
	}

	// The files under each folder, as they are now.
	under, err := gitx.FilesUnder(root, dirs)
	if err != nil {
		return nil, err
	}
	modes := map[string]string{}
	toHash := slices.Clone(files)
	for _, f := range under {
		fi, err := os.Lstat(filepath.Join(root, filepath.FromSlash(f)))
		if errors.Is(err, fs.ErrNotExist) || err == nil && fi.IsDir() {
			continue // deleted from the work tree, or a submodule
		}
		if err != nil {
			return nil, err
		}
		modes[f] = "100644"
		if fi.Mode()&fs.ModeSymlink != 0 {
			modes[f] = "120000"
		}
		if !isFile[f] {
			toHash = append(toHash, f)
		}
	}
	ids, err := gitx.HashFiles(root, toHash)
	if err != nil {
		return nil, err
	}
	blobs := make(map[string]string, len(ids))
	for i, f := range toHash {
		blobs[f] = ids[i]
	}
	for _, f := range files {
		cur[f] = Current{Blob: blobs[f]}
	}
	if len(dirs) == 0 {
		return cur, nil
	}

	newHash := sha1.New
	if format, err := objectFormat(root, ids); err != nil {
		return nil, err
	} else if format == "sha256" {
		newHash = sha256.New
	}
	for _, d := range dirs {
		entries := map[string]treeFile{}
		for f, mode := range modes {
			if rel, ok := strings.CutPrefix(f, d+"/"); ok {
				id, err := hex.DecodeString(blobs[f])
				if err != nil {
					return nil, fmt.Errorf("git hash-object: bad ID %q for %s", blobs[f], f)
				}
				entries[rel] = treeFile{mode, id}
			}
		}
		cur[d] = Current{Tree: hex.EncodeToString(treeID(newHash, entries))}
	}
	return cur, nil
}

// objectFormat is the repo's object hash, read from the length of a blob ID
// in ids, or from git when there is none.
func objectFormat(root string, ids []string) (string, error) {
	if len(ids) > 0 {
		if len(ids[0]) == 64 {
			return "sha256", nil
		}
		return "sha1", nil
	}
	return gitx.ObjectFormat(root)
}

// treeFile is a file in a tree: its mode and blob ID.
type treeFile struct {
	mode string
	id   []byte
}

// treeID returns the ID of the git tree holding files, keyed by slash-separated
// path, as `git write-tree` would write it.
func treeID(newHash func() hash.Hash, files map[string]treeFile) []byte {
	type entry struct {
		name, mode string
		id         []byte
	}
	var entries []entry
	subs := map[string]map[string]treeFile{}
	for p, f := range files {
		dir, rest, ok := strings.Cut(p, "/")
		if !ok {
			entries = append(entries, entry{p, f.mode, f.id})
			continue
		}
		if subs[dir] == nil {
			subs[dir] = map[string]treeFile{}
		}
		subs[dir][rest] = f
	}
	for dir, sub := range subs {
		entries = append(entries, entry{dir, "40000", treeID(newHash, sub)})
	}
	// git sorts a tree's entries by name, a subtree's name as if it ended in "/".
	sortName := func(e entry) string {
		if e.mode == "40000" {
			return e.name + "/"
		}
		return e.name
	}
	slices.SortFunc(entries, func(a, b entry) int { return strings.Compare(sortName(a), sortName(b)) })
	var body bytes.Buffer
	for _, e := range entries {
		body.WriteString(e.mode + " " + e.name + "\x00")
		body.Write(e.id)
	}
	h := newHash()
	_, _ = fmt.Fprintf(h, "tree %d\x00", body.Len())
	_, _ = h.Write(body.Bytes())
	return h.Sum(nil)
}

// Freshness is a record's or area file's freshness.
type Freshness struct {
	State State `json:"state"`
	// Changed are the cited paths whose hash no longer matches, as cited.
	Changed []string `json:"changed,omitempty"`
	// Missing are the cited paths that no longer exist, as cited.
	Missing []string `json:"missing,omitempty"`
}

// Check computes the freshness of cites against cur, the current hashes Hash
// returned for their paths: broken when a cited path no longer exists,
// drifted when a hash no longer matches, else fresh. A path cited more than
// once is named once, as first named.
func Check(cites []Cite, cur map[string]Current) Freshness {
	var f Freshness
	named := map[string]bool{}
	for _, c := range cites {
		k := key(c.Path)
		h := cur[k]
		switch {
		case named[k]:
			continue
		case h.Missing():
			f.Missing = append(f.Missing, c.Path)
		case h.Blob != c.Blob || h.Tree != c.Tree:
			f.Changed = append(f.Changed, c.Path)
		default:
			continue
		}
		named[k] = true
	}
	switch {
	case len(f.Missing) > 0:
		f.State = Broken
	case len(f.Changed) > 0:
		f.State = Drifted
	default:
		f.State = Fresh
	}
	return f
}

// Entry is one record or area file with cites: and its freshness.
type Entry struct {
	// Path is the file, relative to the repo root.
	Path string `json:"path"`
	// Kind is the record kind, or "area" for an area file.
	Kind string `json:"kind"`
	ID   string `json:"id,omitempty"`
	Freshness
	// Reason says why an Invalid entry can't be checked.
	Reason string `json:"reason,omitempty"`
}

// All returns every record under lyceum/{understand,trace,audit}/ and every
// area file in lyceum/spec/ with cites:, area files first, then records by
// kind, each by file name, with their freshness. It hashes every cited path
// with one call to Hash.
func All(root string) ([]Entry, error) {
	type found struct {
		entry Entry
		cites []Cite
	}
	var all []found
	for _, kind := range append([]string{"area"}, Kinds...) {
		dir := Dir(kind)
		if kind == "area" {
			dir = SpecDir
		}
		names, err := mdFiles(filepath.Join(root, filepath.FromSlash(dir)))
		if err != nil {
			return nil, err
		}
		for _, name := range names {
			e := Entry{Path: path.Join(dir, name), Kind: kind}
			data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(e.Path)))
			if err != nil {
				return nil, err
			}
			if kind == "area" && !HasFrontmatter(data) {
				continue
			}
			r, err := Parse(data)
			switch {
			case err != nil:
			case kind == "area" && !r.HasCites:
				continue
			case kind == "area":
				if err = ValidateCites(r.Cites); err == nil {
					err = CitesComplete(r.Cites)
				}
			default:
				e.ID = r.ID
				if err = r.Validate(); err == nil {
					err = r.Complete()
				}
				if err == nil && r.Kind != kind {
					err = &FieldError{"kind", fmt.Sprintf("%q, but the record is in %s/", r.Kind, dir)}
				}
			}
			if err != nil {
				e.State, e.Reason = Invalid, err.Error()
				all = append(all, found{entry: e})
				continue
			}
			all = append(all, found{e, r.Cites})
		}
	}

	var paths []string
	for _, f := range all {
		for _, c := range f.cites {
			paths = append(paths, c.Path)
		}
	}
	cur, err := Hash(root, paths)
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, 0, len(all))
	for _, f := range all {
		if f.entry.State != Invalid {
			f.entry.Freshness = Check(f.cites, cur)
		}
		entries = append(entries, f.entry)
	}
	return entries, nil
}

// mdFiles returns the names of the .md files directly in dir, sorted, or none
// when dir does not exist.
func mdFiles(dir string) ([]string, error) {
	des, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, de := range des {
		if !de.IsDir() && strings.HasSuffix(de.Name(), ".md") {
			names = append(names, de.Name())
		}
	}
	return names, nil
}

// Detail names what makes e not fresh: its missing and changed paths, or why
// it is invalid; "" when it is fresh.
func (e Entry) Detail() string {
	if e.State == Invalid {
		return e.Reason
	}
	var parts []string
	if len(e.Missing) > 0 {
		parts = append(parts, "missing "+strings.Join(e.Missing, ", "))
	}
	if len(e.Changed) > 0 {
		parts = append(parts, "changed "+strings.Join(e.Changed, ", "))
	}
	return strings.Join(parts, "; ")
}
