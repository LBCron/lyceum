package templates

import (
	"io/fs"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// The AGENTS.md section is under 80 lines, delimited by the markers, and carries
// the rules distribution.md lists.
func TestAgentsSection(t *testing.T) {
	lines := strings.Split(strings.TrimSuffix(AgentsSection, "\n"), "\n")
	if len(lines) >= 80 {
		t.Errorf("section is %d lines; want under 80", len(lines))
	}
	if lines[0] != SectionStart || lines[len(lines)-1] != SectionEnd {
		t.Errorf("section is not delimited by %q and %q", SectionStart, SectionEnd)
	}
	for _, want := range []string{
		"Run `lyceum status` first",
		"Always: AGENTS.md, RULES.md, TESTING.md",
		"Never run `lyceum approve` on your own initiative",
		"`approvals.harness_prompt: allow`",
		"only when the owner asks for it in this conversation",
		"Never fake a terminal",
		"Never edit CLI-owned fields",
		"### Ownership",
	} {
		if !strings.Contains(AgentsSection, want) {
			t.Errorf("section lacks %q", want)
		}
	}
}

// The CLAUDE.md pointer is the two sentences of distribution.md between the
// markers: it only points to AGENTS.md and restates no rule, approve included.
func TestClaudePointer(t *testing.T) {
	want := SectionStart + "\nRead AGENTS.md; this project uses Lyceum. Run `lyceum status` first.\n" + SectionEnd + "\n"
	if ClaudePointer != want {
		t.Errorf("pointer = %q; want %q", ClaudePointer, want)
	}
	if strings.Contains(ClaudePointer, "approve") {
		t.Errorf("pointer mentions approve; it restates no rule")
	}
}

// skillLines is each skill's line budget; skills not listed have 120.
var skillLines = map[string]int{"status": 40, "doc": 60, "test": 60}

// skillApprovals are the judgment gates each skill stops at, each named as
// the owner's `lyceum approve <gate>` (units.md, "Gate lists").
var skillApprovals = map[string][]string{
	"feat":     {"spec"},
	"bug":      {"cause"},
	"refactor": {"intent", "tests", "plan"},
	"chore":    {"plan"},
	"init":     {"project"},
}

// skillSteps are what each inspect skill must say (records.md): the
// commands it runs, the record format it writes, and where findings go.
var skillSteps = map[string][]string{
	"understand": {"ask what they want to understand before reading any code", "`lyceum understand \"<hint>\"`",
		"file names are fine, file contents are not",
		"Prefer a spec area", "warn_threshold", "`cites:`", "Never cite line numbers", "`lyceum record lyceum/understand/",
		"don't ask whether to", "`lyceum export <record>`"},
	"trace": {"`lyceum trace \"<question>\"`", "code-only", "`lyceum trace --run <env> --trigger \"<command>\" \"<question>\"`",
		"without `--yes`", "ask in chat", "Never add `--yes` on your own initiative", "`[read]`", "`[observed]`",
		"`sequenceDiagram`", "`lyceum record lyceum/trace/", "`lyceum add bug \"<title>\"`", "`lyceum add feat \"<title>\"`",
		"never fixed", "with a concrete title", "\"a bug unit\" alone is not the offer"},
	"audit": {"Run `lyceum audit`", "`## Summary`", "`lyceum record lyceum/audit/", "`lyceum chore \"Upgrade",
		"`lyceum add bug \"<title>\"`", "never changes packages"},
}

// oneLine joins s's words with single spaces, so wrapped text compares equal.
func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// Each embedded skill is a valid Agent Skills file: frontmatter with name
// lyceum-<name> and a description saying when to use it, and a body that
// sends the agent to lyceum status first, within its line budget. A
// judgment-gate skill names each gate's `lyceum approve <gate>` and carries
// the AGENTS.md approve rule and its harness-prompt exception word for word;
// every line naming `lyceum approve` names the owner. An inspect
// skill says what skillSteps lists (AC120). skills/ holds exactly SkillNames.
func TestSkills(t *testing.T) {
	_, exception, ok := strings.Cut(AgentsSection, "- The one exception: ")
	if !ok {
		t.Fatal("the AGENTS.md section has no harness-prompt exception")
	}
	exception, _, _ = strings.Cut(exception, "\n- ")
	exception = oneLine(exception)

	dirs, err := fs.ReadDir(skills, "skills")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, d := range dirs {
		names = append(names, d.Name())
	}
	if want := slices.Sorted(slices.Values(SkillNames)); !slices.Equal(names, want) {
		t.Errorf("skills/ holds %v; want %v", names, want)
	}
	for _, name := range SkillNames {
		text := Skill(name)
		front, body, ok := strings.Cut(strings.TrimPrefix(text, "---\n"), "\n---\n")
		if !ok || !strings.HasPrefix(text, "---\n") {
			t.Errorf("%s: no frontmatter", name)
			continue
		}
		var meta map[string]string
		if err := yaml.Unmarshal([]byte(front), &meta); err != nil {
			t.Errorf("%s: frontmatter: %v", name, err)
			continue
		}
		if meta["name"] != "lyceum-"+name {
			t.Errorf("%s: name is %q; want %q", name, meta["name"], "lyceum-"+name)
		}
		if d := meta["description"]; d == "" || strings.Contains(d, "\n") {
			t.Errorf("%s: description %q is not one line", name, d)
		}
		if len(meta) != 2 {
			t.Errorf("%s: frontmatter has %d keys; want name and description", name, len(meta))
		}
		if !strings.Contains(body, "Run `lyceum status` first") {
			t.Errorf("%s: body lacks \"Run `lyceum status` first\"", name)
		}
		if !strings.HasPrefix(meta["description"], "Use when ") {
			t.Errorf("%s: description %q does not start with \"Use when\"", name, meta["description"])
		}
		budget := skillLines[name]
		if budget == 0 {
			budget = 120
		}
		if n := strings.Count(text, "\n"); n >= budget {
			t.Errorf("%s: %d lines; want under %d", name, n, budget)
		}
		for _, want := range skillSteps[name] {
			if !strings.Contains(oneLine(body), want) {
				t.Errorf("%s: body lacks %q", name, want)
			}
		}
		if strings.Contains(strings.ToLower(body), "not built") {
			t.Errorf("%s: body says a command is not built; every command a skill names is built", name)
		}
		for _, g := range skillApprovals[name] {
			if !strings.Contains(body, "`lyceum approve "+g+"`") {
				t.Errorf("%s: body does not name `lyceum approve %s`", name, g)
			}
		}
		if _, ok := skillApprovals[name]; ok {
			if !strings.Contains(body, "Never run `lyceum approve` on your own initiative.") {
				t.Errorf("%s: body lacks the rule that the agent never runs approve on its own initiative", name)
			}
			if !strings.Contains(oneLine(body), exception) {
				t.Errorf("%s: body lacks the AGENTS.md harness-prompt exception %q", name, exception)
			}
		}
		for i, line := range strings.Split(body, "\n") {
			if strings.Contains(line, "lyceum approve") && !strings.Contains(line, "owner") {
				t.Errorf("%s: body line %d names lyceum approve without the owner: %q", name, i+1, line)
			}
		}
		if strings.Contains(body, "lyceum approve") && !strings.Contains(body, "on your own initiative") {
			t.Errorf("%s: body names lyceum approve but not that the agent never runs it on its own initiative", name)
		}
		// close, amend and plan are built: no section that names one says a command is not built.
		for _, section := range strings.Split(body, "\n## ") {
			if !strings.Contains(strings.ToLower(section), "not built") {
				continue
			}
			heading, _, _ := strings.Cut(section, "\n")
			for _, c := range []string{"lyceum close", "lyceum amend", "lyceum plan"} {
				if strings.Contains(section, c) {
					t.Errorf("%s: section %q names `%s` and says a command is not built", name, heading, c)
				}
			}
		}
	}
}

// TestSkillRules_AC164: the first interview question is open, every unit skill
// makes a proceed confirmation a structured yes/no, and every skill naming
// lyceum approve says one approval per command.
// An agent-only stop: the CLI half is in the AC's script, the rule is here.
func TestSkillStops_AC30_AC100(t *testing.T) {
	for name, want := range map[string][]string{
		"test":     {"fails on the current code: STOP", "Don't change the test or the code", "offer to keep it as a bug"},
		"refactor": {"A behaviour change turns up", "STOP", "don't\n   change any test assertion or spec behaviour text", "`lyceum add bug \"<title>\"`", "`lyceum add feat \"<title>\"`"},
	} {
		for _, w := range want {
			if !strings.Contains(Skill(name), w) {
				t.Errorf("%s skill lacks %q", name, w)
			}
		}
	}
}

func TestSkillRules_AC164(t *testing.T) {
	if !strings.Contains(oneLine(Skill("init")), "The first question is open, with no options") {
		t.Error("init: the first interview question is not open")
	}
	for _, name := range []string{"feat", "bug", "refactor", "chore", "doc", "test"} {
		if !strings.Contains(oneLine(Skill(name)), "any confirmation to proceed is a structured yes/no") {
			t.Errorf("%s: lacks the structured yes/no handoff", name)
		}
	}
	for _, name := range SkillNames {
		if strings.Contains(Skill(name), "lyceum approve") && !strings.Contains(oneLine(Skill(name)), "One approval per command") {
			t.Errorf("%s: names lyceum approve but not one approval per command", name)
		}
	}
}
