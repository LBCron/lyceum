// Package out writes how a command ends: a Result when it succeeds, a Refusal
// when it declines to act, or an unexpected error.
//
// Every ending names the next action. In human mode a Result ends
// with "Unit: …" (when set), "Gate: …" (when set) and "Next: …" lines on stdout,
// and a Refusal prints "Can't <what>: <reason>" and one "Next: <step>" line per
// step on stderr. In JSON mode a Result or Refusal is exactly one JSON object on
// stdout, and nothing else is written there:
//
//	{"ok":true,"unit":…,"gate":…,"next":…,"data":…}
//	{"ok":false,"refusal":{"reason":…,"next":[…]}}
//
// unit, gate and data are null when unset. A command may add its own keys
// beside these (Result.Extra, Refusal.WithJSON). An unexpected error goes to
// stderr in both modes.
//
// Exit codes:
//
//	0  ExitOK       the command did what it was asked
//	1  ExitError    an unexpected error: a bug or a failing environment
//	2  ExitRefused  the command refused; the output says why and what to do next
package out

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"
)

// Exit codes of the lyceum process.
const (
	ExitOK      = 0
	ExitError   = 1
	ExitRefused = 2
)

// Result is a successful outcome. Unit and Gate are empty when no unit is
// involved. Next is the action to take now. Data is the command's payload,
// emitted in JSON mode only.
type Result struct {
	Unit, Gate, Next string
	Data             any
	// Extra holds keys written beside the others in JSON mode only.
	Extra map[string]any
}

// Refusal is a command declining to act. It is an error, so a command returns
// it from RunE. Its fields are unexported: Refuse is the only way to build one,
// and it requires a next step.
type Refusal struct {
	what, reason string
	next         []string
	extra        map[string]any
}

// Refuse returns a Refusal printed as "Can't <what>: <reason>". It panics when
// no next step is given or a step is blank: a refusal without a way forward is
// a bug (AC108).
func Refuse(what, reason string, next ...string) *Refusal {
	if len(next) == 0 {
		panic("out.Refuse: a refusal needs at least one next step")
	}
	for _, s := range next {
		if strings.TrimSpace(s) == "" {
			panic("out.Refuse: blank next step")
		}
	}
	return &Refusal{what: what, reason: reason, next: slices.Clone(next)}
}

func (r *Refusal) Error() string { return "can't " + r.what + ": " + r.reason }

// WithJSON adds extra's keys beside ok and refusal in JSON mode, and returns r.
func (r *Refusal) WithJSON(extra map[string]any) *Refusal {
	r.extra = extra
	return r
}

// Writer writes a command's ending to the process streams.
type Writer struct {
	Stdout, Stderr io.Writer
	JSON           bool
}

// Result writes r and returns ExitOK.
func (w Writer) Result(r Result) int {
	if w.JSON {
		return w.writeJSON(ExitOK, struct {
			OK   bool    `json:"ok"`
			Unit *string `json:"unit"`
			Gate *string `json:"gate"`
			Next string  `json:"next"`
			Data any     `json:"data"`
		}{true, nullable(r.Unit), nullable(r.Gate), r.Next, r.Data}, r.Extra)
	}
	var b strings.Builder
	if r.Unit != "" {
		b.WriteString("Unit: " + r.Unit + "\n")
	}
	if r.Gate != "" {
		b.WriteString("Gate: " + r.Gate + "\n")
	}
	b.WriteString("Next: " + r.Next + "\n")
	_, _ = io.WriteString(w.Stdout, b.String())
	return ExitOK
}

// Fail writes err and returns its exit code: ExitRefused for a Refusal,
// ExitError for anything else.
func (w Writer) Fail(err error) int {
	var r *Refusal
	if !errors.As(err, &r) {
		return w.unexpected(err)
	}
	if len(r.next) == 0 {
		// Only a zero Refusal, built without Refuse, gets here.
		return w.unexpected(errors.New("internal error: refusal without a next step"))
	}
	if w.JSON {
		type refusal struct {
			Reason string   `json:"reason"`
			Next   []string `json:"next"`
		}
		return w.writeJSON(ExitRefused, struct {
			OK      bool    `json:"ok"`
			Refusal refusal `json:"refusal"`
		}{false, refusal{r.reason, r.next}}, r.extra)
	}
	var b strings.Builder
	b.WriteString("Can't " + r.what + ": " + r.reason + "\n")
	for _, s := range r.next {
		b.WriteString("Next: " + s + "\n")
	}
	_, _ = io.WriteString(w.Stderr, b.String())
	return ExitRefused
}

func (w Writer) unexpected(err error) int {
	_, _ = io.WriteString(w.Stderr, err.Error()+"\n")
	return ExitError
}

// writeJSON writes v, an object, with extra's keys after its own, as one line
// on stdout and returns code, or reports an unexpected error and writes
// nothing to stdout when either can't be encoded.
func (w Writer) writeJSON(code int, v any, extra map[string]any) int {
	b, err := encode(v)
	if err != nil {
		return w.unexpected(err)
	}
	if len(extra) > 0 {
		rest, err := encode(extra)
		if err != nil {
			return w.unexpected(err)
		}
		// Splice {"ok":…} and {"k":…} into one object.
		b = append(append(b[:len(b)-1], ','), rest[1:]...)
	}
	_, _ = w.Stdout.Write(append(b, '\n'))
	return code
}

// encode returns v as compact JSON without HTML escaping or a final newline.
func encode(v any) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(b.Bytes(), []byte("\n")), nil
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
