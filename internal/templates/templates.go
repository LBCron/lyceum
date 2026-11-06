// Package templates holds the files `lyceum init` writes (distribution.md,
// "lyceum init").
package templates

import (
	"embed"
	"fmt"
)

// Markers delimit the Lyceum section in AGENTS.md and CLAUDE.md.
const (
	SectionStart = "<!-- lyceum:start -->"
	SectionEnd   = "<!-- lyceum:end -->"
)

// AgentsSection is the Lyceum section of AGENTS.md, markers included.
//
//go:embed agents.md
var AgentsSection string

// ClaudePointer is the Lyceum section of CLAUDE.md, markers included: a short
// pointer to AGENTS.md, written when Claude Code is wired.
//
//go:embed claude.md
var ClaudePointer string

// Rules is a new project's RULES.md.
//
//go:embed rules.md
var Rules string

// Spec is a new project's lyceum/SPEC.md, with an empty area registry.
//
//go:embed spec.md
var Spec string

// PiExtension is Pi's pre-tool hook shim, .pi/extensions/lyceum.ts.
//
//go:embed pi-extension.ts
var PiExtension string

// OpenCodePlugin is OpenCode's pre-tool hook shim, a V2 plugin at
// .opencode/plugins/lyceum.js.
//
//go:embed opencode-plugin.js
var OpenCodePlugin string

// Markers delimit Lyceum's block in a pre-commit hook.
const (
	HookStart = "# lyceum:start"
	HookEnd   = "# lyceum:end"
)

// PreCommit is Lyceum's block of the pre-commit hook, markers included.
//
//go:embed pre-commit
var PreCommit string

// Workflow is the CI workflow, .github/workflows/lyceum.yml.
//
//go:embed workflow.yml
var Workflow string

// SkillNames are the Lyceum skills init writes, each as lyceum-<name>: the
// eight unit skills, then understand, trace and audit.
var SkillNames = []string{"feat", "bug", "doc", "test", "refactor", "chore", "init", "status", "understand", "trace", "audit"}

//go:embed skills
var skills embed.FS

// Skill returns the SKILL.md of skill lyceum-<name>, one of SkillNames.
func Skill(name string) string {
	data, err := skills.ReadFile("skills/" + name + "/SKILL.md")
	if err != nil {
		panic(fmt.Sprintf("templates: no skill %q", name)) // SkillNames and skills/ are kept in step
	}
	return string(data)
}
