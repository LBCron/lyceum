package cli

import (
	"path"
	"slices"
	"strings"
)

// shellCommands is shell command line with the text that is data blanked: the
// body of a here-document fed to cat or tee, and the message of a git commit,
// tag or merge. The hook looks for `lyceum approve` in what is left.
//
// Missing a real approval is far worse than a false alarm, so this only ever
// blanks what it is sure of. Anything it can't read, or any sign the data might
// be run, returns line whole: a shell, eval or xargs anywhere, a command that
// is itself an expansion, a pipe out of the command, a here-document that
// expands, or a later command naming the file the here-document wrote.
func shellCommands(line string) string {
	s := shScan{src: line}
	cmds, end, ok := s.list(0, false)
	if !ok || end != len(line) {
		return line
	}
	for _, c := range cmds {
		if len(c.words) == 0 {
			continue
		}
		w := c.words[0].raw
		if strings.HasPrefix(w, "$") || strings.HasPrefix(w, `"$`) || strings.HasPrefix(w, "`") {
			return line
		}
		if slices.Contains(shKeywords, w) && len(c.words) > 1 && strings.Contains(c.words[1].raw, "$") {
			return line
		}
	}
	var cuts [][2]int
	for _, c := range cmds {
		cuts = append(cuts, c.dataSpans(cmds)...)
	}
	if len(cuts) == 0 {
		return line
	}
	slices.SortFunc(cuts, func(a, b [2]int) int { return a[0] - b[0] })
	var b strings.Builder
	at := 0
	for _, c := range cuts {
		b.WriteString(line[at:c[0]])
		b.WriteByte(' ')
		at = c[1]
	}
	b.WriteString(line[at:])
	kept := b.String()
	for _, w := range strings.FieldsFunc(kept, shellSeparator) {
		if slices.Contains(shRunners, path.Base(w)) {
			return line
		}
	}
	return kept
}

// shellSeparator reports the runes that separate words for the hook's matching.
func shellSeparator(r rune) bool {
	return strings.ContainsRune(" \t\r\n;&|()<>'\"`!", r)
}

var (
	// shRunners run text they are given, so nothing beside them is data.
	shRunners = []string{"sh", "bash", "zsh", "dash", "ksh", "ash", "fish", "eval", "source", ".", "xargs", "parallel"}
	// shKeywords can stand before a command word.
	shKeywords = []string{"do", "then", "else", "elif", "if", "while", "until", "{", "!", "time"}
	// shReaders don't run a file named on their command line.
	shReaders = []string{"git", "cat", "ls", "wc", "head", "tail", "rm", "grep", "chmod"}
)

// shWord is one word of a simple command, spanning src[start:end].
type shWord struct {
	start, end int
	raw        string
	// expands: the word holds a command substitution.
	expands bool
	// subs are the command lists of its $( ) substitutions, with their spans.
	subs []shSub
}

type shSub struct {
	start, end int // from "$(" to just past ")"
	cmds       []*shCmd
}

type shHeredoc struct {
	delim      string
	quoted     bool
	strip      bool
	start, end int // the body
	body       string
}

// shCmd is one simple command.
type shCmd struct {
	words    []shWord
	targets  []string // what > and >> write to
	heredocs []*shHeredoc
	piped    bool // its output goes on to another command
}

// dataSpans are the spans of c that are data, for the commands of its list.
func (c *shCmd) dataSpans(all []*shCmd) [][2]int {
	if c.piped || len(c.words) == 0 {
		return nil
	}
	var spans [][2]int
	if c.sink() && !c.fileRun(all) || c.gitMessageFromStdin() {
		for _, h := range c.heredocs {
			if h.isData() {
				spans = append(spans, [2]int{h.start, h.end})
			}
		}
	}
	return append(spans, c.messageSpans()...)
}

// sink reports whether c is exactly cat or tee, which copy their input.
func (c *shCmd) sink() bool {
	return c.words[0].raw == "cat" || c.words[0].raw == "tee"
}

// fileRun reports whether another command of all names a file c writes, other
// than one that only reads or stores it: it may run what was written.
func (c *shCmd) fileRun(all []*shCmd) bool {
	files := slices.Clone(c.targets)
	if c.words[0].raw == "tee" {
		for _, w := range c.words[1:] {
			if !strings.HasPrefix(w.raw, "-") {
				files = append(files, strings.Trim(w.raw, `'"`))
			}
		}
	}
	for _, f := range files {
		base := path.Base(f)
		for _, o := range all {
			if o == c || len(o.words) == 0 || slices.Contains(shReaders, o.words[0].raw) {
				continue
			}
			for _, w := range o.words {
				if strings.Contains(w.raw, base) {
					return true
				}
			}
		}
	}
	return false
}

// isData reports whether the here-document's body can't run anything as the
// shell reads it: with an unquoted delimiter a $( ), backquote or backslash
// (a line continuation can hide the delimiter) is still live.
func (h *shHeredoc) isData() bool {
	return h.quoted || !strings.Contains(h.body, "$(") && !strings.ContainsAny(h.body, "`\\")
}

// gitMessageWords returns the words after the subcommand of a git command
// that takes -m, or nil. An assignment before git is not looked through.
func (c *shCmd) gitMessageWords() []shWord {
	if c.words[0].raw != "git" {
		return nil
	}
	i := 1
	for i < len(c.words) && strings.HasPrefix(c.words[i].raw, "-") {
		if c.words[i].raw == "-C" || c.words[i].raw == "-c" {
			i++
		}
		i++
	}
	if i >= len(c.words) || !slices.Contains([]string{"commit", "tag", "merge"}, c.words[i].raw) {
		return nil
	}
	return c.words[i+1:]
}

// gitMessageFromStdin reports git commit -F - or -F /dev/stdin.
func (c *shCmd) gitMessageFromStdin() bool {
	words := c.gitMessageWords()
	for i, w := range words {
		if (w.raw == "-F" || w.raw == "--file") && i+1 < len(words) && (words[i+1].raw == "-" || words[i+1].raw == "/dev/stdin") {
			return true
		}
	}
	return false
}

// messageSpans are the spans of the -m values of a git command.
func (c *shCmd) messageSpans() [][2]int {
	var spans [][2]int
	words := c.gitMessageWords()
	for i, w := range words {
		var v shWord
		switch {
		case w.raw == "--message" || isShortMessageFlag(w.raw):
			if i+1 >= len(words) {
				continue
			}
			v = words[i+1]
		case strings.HasPrefix(w.raw, "--message=") || strings.HasPrefix(w.raw, "-m") && len(w.raw) > 2:
			v = w
		default:
			continue
		}
		if v.isData() {
			spans = append(spans, [2]int{v.start, v.end})
		}
	}
	return spans
}

// isShortMessageFlag reports -m, or a cluster ending in it, as -am.
func isShortMessageFlag(w string) bool {
	if len(w) < 2 || w[0] != '-' || w[1] == '-' || w[len(w)-1] != 'm' {
		return false
	}
	return strings.Trim(w[1:], "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ") == ""
}

// isData reports whether a message word can't run anything: no substitution,
// or exactly one that is all of it and only cats a here-document that is data.
func (w shWord) isData() bool {
	if !w.expands {
		return true
	}
	if len(w.subs) != 1 {
		return false
	}
	sub := w.subs[0]
	whole := sub.start == w.start && sub.end == w.end ||
		sub.start == w.start+1 && sub.end == w.end-1 && w.raw[0] == '"' && w.raw[len(w.raw)-1] == '"'
	if !whole || strings.Count(w.raw, "`") > 0 {
		return false
	}
	var real []*shCmd
	for _, c := range sub.cmds {
		if len(c.words) > 0 {
			real = append(real, c)
		}
	}
	if len(real) != 1 || len(real[0].words) != 1 || real[0].words[0].raw != "cat" || real[0].piped || len(real[0].heredocs) != 1 {
		return false
	}
	return real[0].heredocs[0].isData()
}

// shScan reads shell syntax just far enough to find words, quotes,
// substitutions and here-documents.
type shScan struct{ src string }

// list reads commands from i to the end of the text or, when nested, to the
// ")" closing a substitution. It returns them and where it stopped; ok is false
// on anything it can't read.
func (s *shScan) list(i int, nested bool) (cmds []*shCmd, end int, ok bool) {
	cur := &shCmd{}
	var pending []*shHeredoc
	depth := 0
	target := false
	finish := func() {
		cmds = append(cmds, cur)
		cur = &shCmd{}
		target = false
	}
	for i < len(s.src) {
		switch ch := s.src[i]; {
		case ch == ' ' || ch == '\t' || ch == '\r':
			i++
		case ch == '\n':
			finish()
			i++
			for _, h := range pending {
				if i, ok = s.body(h, i); !ok {
					return nil, 0, false
				}
			}
			pending = nil
		case ch == '#' && (i == 0 || strings.ContainsRune(" \t\n;&|()", rune(s.src[i-1]))):
			for i < len(s.src) && s.src[i] != '\n' {
				i++
			}
		case ch == ';':
			finish()
			i++
		case ch == '&':
			if strings.HasPrefix(s.src[i:], "&>") {
				i += 2
				target = true
				break
			}
			finish()
			i++
		case ch == '|':
			switch {
			case strings.HasPrefix(s.src[i:], "||"):
				i += 2
			case strings.HasPrefix(s.src[i:], "|&"):
				cur.piped = true
				i += 2
			default:
				cur.piped = true
				i++
			}
			finish()
		case ch == '(':
			depth++
			finish()
			i++
		case ch == ')':
			if nested && depth == 0 {
				finish()
				if len(pending) > 0 {
					return nil, 0, false
				}
				return cmds, i, true
			}
			depth = max(depth-1, 0)
			finish()
			i++
		case strings.HasPrefix(s.src[i:], "<<<"):
			i += 3
		case strings.HasPrefix(s.src[i:], "<<"):
			i += 2
			h := &shHeredoc{}
			if i < len(s.src) && s.src[i] == '-' {
				h.strip = true
				i++
			}
			for i < len(s.src) && (s.src[i] == ' ' || s.src[i] == '\t') {
				i++
			}
			w, next, ok := s.word(i)
			if !ok || w.raw == "" || w.expands {
				return nil, 0, false
			}
			h.quoted = strings.ContainsAny(w.raw, `'"\`)
			h.delim = strings.NewReplacer(`'`, "", `"`, "", `\`, "").Replace(w.raw)
			i = next
			pending = append(pending, h)
			cur.heredocs = append(cur.heredocs, h)
		case ch == '<':
			i++
			if i < len(s.src) && strings.ContainsRune("&>", rune(s.src[i])) {
				i++
			}
		case ch == '>':
			i++
			if i < len(s.src) && strings.ContainsRune(">&|", rune(s.src[i])) {
				i++
			}
			target = true
		default:
			w, next, ok := s.word(i)
			if !ok {
				return nil, 0, false
			}
			i = next
			if target {
				cur.targets = append(cur.targets, strings.Trim(w.raw, `'"`))
				target = false
				continue
			}
			cur.words = append(cur.words, w)
		}
	}
	if nested || len(pending) > 0 {
		return nil, 0, false
	}
	finish()
	return cmds, i, true
}

// body reads here-document h's body from line start i and returns where the
// line after its delimiter starts. A body with no delimiter is not read.
func (s *shScan) body(h *shHeredoc, i int) (int, bool) {
	h.start = i
	for i < len(s.src) {
		eol := strings.IndexByte(s.src[i:], '\n')
		line, next := s.src[i:], len(s.src)
		if eol >= 0 {
			line, next = s.src[i:i+eol], i+eol+1
		}
		cmp := line
		if h.strip {
			cmp = strings.TrimLeft(line, "\t")
		}
		if cmp == h.delim {
			h.end = i
			h.body = s.src[h.start:h.end]
			return next, true
		}
		i = next
	}
	return 0, false
}

// word reads one word from i, through its quotes and substitutions.
func (s *shScan) word(i int) (w shWord, end int, ok bool) {
	w.start = i
	for i < len(s.src) {
		ch := s.src[i]
		switch {
		case strings.IndexByte(" \t\r\n;&|()<>", ch) >= 0:
			w.end, w.raw = i, s.src[w.start:i]
			return w, i, true
		case ch == '\\':
			i += 2
		case ch == '\'':
			j := strings.IndexByte(s.src[i+1:], '\'')
			if j < 0 {
				return w, 0, false
			}
			i += j + 2
		case ch == '"':
			if i, ok = s.dquote(&w, i+1); !ok {
				return w, 0, false
			}
		case ch == '`':
			if i, ok = s.backquote(&w, i+1); !ok {
				return w, 0, false
			}
		case ch == '$':
			next := byte(0)
			if i+1 < len(s.src) {
				next = s.src[i+1]
			}
			switch next {
			case '\'', '"': // $'…' has its own escapes, so the quotes here can't be trusted
				return w, 0, false
			case '(':
				if i, ok = s.subst(&w, i); !ok {
					return w, 0, false
				}
			default:
				i++
			}
		default:
			i++
		}
	}
	if i > len(s.src) {
		return w, 0, false
	}
	w.end, w.raw = i, s.src[w.start:i]
	return w, i, true
}

// dquote reads a double-quoted string from just after its opening quote.
func (s *shScan) dquote(w *shWord, i int) (int, bool) {
	var ok bool
	for i < len(s.src) {
		switch {
		case s.src[i] == '"':
			return i + 1, true
		case s.src[i] == '\\':
			i += 2
		case s.src[i] == '`':
			if i, ok = s.backquote(w, i+1); !ok {
				return 0, false
			}
		case strings.HasPrefix(s.src[i:], "$("):
			if i, ok = s.subst(w, i); !ok {
				return 0, false
			}
		default:
			i++
		}
	}
	return 0, false
}

// backquote reads a backquoted substitution from just after its opening quote.
func (s *shScan) backquote(w *shWord, i int) (int, bool) {
	w.expands = true
	for i < len(s.src) {
		switch s.src[i] {
		case '\\':
			i += 2
		case '`':
			return i + 1, true
		default:
			i++
		}
	}
	return 0, false
}

// subst reads a $( ) substitution starting at i.
func (s *shScan) subst(w *shWord, i int) (int, bool) {
	w.expands = true
	cmds, end, ok := s.list(i+2, true)
	if !ok {
		return 0, false
	}
	w.subs = append(w.subs, shSub{start: i, end: end + 1, cmds: cmds})
	return end + 1, true
}
