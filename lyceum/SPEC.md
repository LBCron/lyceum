# Lyceum: specification index

**Status: v1**, the first public version. Decisions from v1 on are logged in `decisions.md`.

Lyceum is a spec-driven software development lifecycle for agentic coding. It ships as one CLI
(`lyceum`) plus skills that any agent harness can load. The CLI keeps state and proves evidence. The agent
interviews, drafts and builds. Only the human approves.

## How to read this spec

- This file is the index. Load an area file only when the work needs it.
- Acceptance criteria are numbered globally (`AC1`, `AC2`, …). Numbers are never reused, and withdrawn ACs
  stay in place, marked withdrawn. Tests cite AC IDs literally.
- Markers:

| Marker | Meaning |
|---|---|
| `[NEEDS CLARIFICATION: …]` | A decision the owner still owes. Nothing that depends on it is built |
| `[DERIVED: Dn]` | Inferred from the cited decisions rather than stated in them. Confirm at the gate |
| `[AGENT-PROPOSED: …]` | Added by the drafter with no decision behind it. Accept or strike at the gate |
| `[TBD until <x>]` | Deliberately deferred to a known later step |

## Area registry

In projects that use Lyceum the agent writes this table and `check` validates it. Here it lists Lyceum's own spec areas.

| Area | File | Covers | ACs |
|---|---|---|---|
| overview | `spec/overview.md` | Purpose, users, goals, invariant, scope, non-functional, glossary | AC1–AC4 |
| architecture | `spec/architecture.md` | CLI/agent split, file layout, config, IDs, branches, logs | AC5–AC14, AC109–AC110, AC114–AC115 |
| units | `spec/units.md` | Units of work, plan file, gate lists per type, close, amend, concurrency, interviews | AC15–AC38, AC100–AC102, AC106–AC107, AC124–AC132, AC154–AC156, AC168 |
| enforcement | `spec/enforcement.md` | `approve`, `check`, CI, hooks, signed approvals, bypass detection | AC39–AC54, AC108, AC118, AC121–AC123, AC139–AC149, AC166–AC167, AC169–AC175, AC178, AC181 |
| test-runner | `spec/test-runner.md` | `lyceum test`, the TESTING.md schema, run records, mechanical gates, AC mapping | AC55–AC66, AC103–AC105, AC112–AC113, AC116, AC150–AC153, AC179–AC180 |
| records | `spec/records.md` | `understand`, `trace`, `audit`, citations, freshness, lookup, export | AC67–AC82, AC133–AC136, AC138 |
| distribution | `spec/distribution.md` | `init` install, harness support, packaging, platforms | AC83–AC94, AC111, AC119–AC120, AC137, AC163–AC165, AC176, AC182–AC183 |
| commands | `spec/commands.md` | Reference: every command, its syntax, where it is defined, `status`, `log` | AC95–AC99, AC117, AC157–AC162, AC177 |

## Open items

None owed by the owner.
