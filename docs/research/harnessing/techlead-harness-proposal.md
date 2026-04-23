# Tech Lead Review: Cerebro as Harness Manager

**Date:** 2026-04-22
**Author:** Tech Lead review (independent rewrite)
**Status:** Final review -- ready for decision
**Input documents:** `approach.md` v1.2, `evidence-catalog.md`, `settings-matrix.md`, `harness-management-research.md`
**Codebase version:** v1.7.0 (tag `v1.7.0`, commit `693d2a6`)

---

## 0. Purpose of This Document

The original `harness-management-research.md` was written before the evidence catalog, settings matrix, and approach document existed. It proposed extending Cerebro from a memory system into a "Brain + Harness Manager" with template management infrastructure (manifest.json, checksum-based skip, three-way diff), new CLI commands (`cerebro harness sync/status/diff`), new skills (`/implement`, `/preflight`), new agents (evaluator, PM, architect, tech-lead), domain packs, and a Stop hook system.

This document is a fresh evaluation of every proposal against three criteria:

1. **Evidence**: Is there proof this solves a real, measured problem?
2. **Implementation cost**: What does it take to build, test, and maintain?
3. **YAGNI**: Is it needed now, or is it speculative engineering?

Every claim below cites a specific source document, GitHub issue, or codebase file.

---

## 1. What the Evidence Actually Shows

### 1.1 The confirmed degradation mechanisms

The evidence catalog documents six root cause vectors (approach.md Section 4). The ones relevant to harness design are:

- **Vector 1 (Thinking Depth)**: Boris Cherny confirmed zero-thinking-tokens turns even at effort=high (evidence-catalog.md, HN item 47668520). The workaround (`CLAUDE_CODE_DISABLE_ADAPTIVE_THINKING=1`) was deprecated on Opus 4.7. **This means settings alone cannot prevent hallucination on current models.**

- **Vector 5 (Behavioral Drift)**: Each model version changes instruction-following strictness, tool-use patterns, and convention adherence (approach.md Section 4, Vector 5). CLAUDE.md rules that worked on 4.6 may be ignored on 4.7.

- **Vector 6 (Lever Removal)**: User control settings are deprecated with each release (approach.md Section 4, Vector 6). This is the strongest argument for hooks and external enforcement over settings.

### 1.2 What has proven effective

Two mitigations have quantitative evidence of working:

1. **Stop-phrase-guard** (stellaraccident, #42796 Appendix B): A command-type Stop hook matching 30+ phrases. Went from 0 violations to 173 in 17 days. This is a proven quality canary -- it doesn't prevent degradation, but it detects it with machine-readable precision.

2. **Read:edit ratio monitoring** (#42796 main analysis): The ratio dropped from 6.6 to 2.0 during degradation. This is a measurable proxy for "edit-without-reading" behavior. The monitoring itself has no hook implementation in the public evidence -- #42796 measured it from JSONL session logs post-hoc, not in real time.

### 1.3 What has NOT been proven

The following proposals in the original document lack evidence of effectiveness:

- **Prompt-type Stop hooks for self-evaluation**: Known broken since v2.0.37 (GitHub issues #11610, #11786). The evaluator receives only metadata, not the conversation transcript. The original document correctly rejected this.

- **Agent-type Stop hooks**: No public evidence of anyone deploying these at scale. The original document deferred this -- correctly.

- **PreToolUse:Write/Edit read-before-edit enforcer**: No known implementation exists. The concept is sound (addresses the read:edit ratio collapse) but requires cross-turn state tracking. No reference implementation to validate the approach.

- **Evaluator agent (Anthropic GAN pattern)**: Anthropic's engineering blog describes a three-agent system for long-running apps. The Anthropic article itself warns: "Out of the box, Claude is a poor QA agent... required several rounds of prompt tuning." No evidence anyone has successfully deployed a skeptical evaluator agent via Claude Code's `~/.claude/agents/` mechanism.

- **Behavioral rules in CLAUDE.md**: The approach document (Section 4, Vector 1) documents that "CLAUDE.md conventions are ignored when thinking depth is insufficient to apply them." If the model allocates zero thinking tokens on a turn, rules in CLAUDE.md are irrelevant -- there is no reasoning step to apply them.

---

## 2. What Cerebro Currently Does (Codebase Audit)

### 2.1 Scaffolding system (`scaffold.go`, `cmd_init.go`)

The current system manages exactly 5 artifacts:

| Artifact | Template file | Scaffold function |
|----------|--------------|-------------------|
| `.claude/settings.json` (hooks) | `templates/settings.json` | `scaffoldSettings()` |
| `.claude/skills/recall/SKILL.md` | `templates/skill_recall.md` | `scaffoldSkills()` |
| `.claude/skills/remember/SKILL.md` | `templates/skill_remember.md` | `scaffoldSkills()` |
| `.claude/skills/consolidate/SKILL.md` | `templates/skill_consolidate.md` | `scaffoldSkills()` |
| `CLAUDE.md` (cerebro section) | `templates/claudemd_section.md` | `scaffoldCLAUDEMD()` |

Source: `cmd/cerebro/scaffold.go` lines 14-28.

All templates are embedded via `//go:embed`. The scaffolding supports `--force` (replace existing) and idempotent non-force (skip if present). The CLAUDE.md scaffolder preserves trailing sections after the Cerebro marker. The settings.json scaffolder merges hooks (non-force) or replaces cerebro-specific hooks while preserving user hooks (force).

Test coverage: 14 tests in `scaffold_test.go` covering new file creation, merge, skip, force-replace, trailing section preservation, and no-HTML-escaping. All tests pass.

### 2.2 Configuration system (`config.go`)

Added in v1.7.0. Stores 5 keys in `schema_meta` table:

- `prime_limit` (int, default 20)
- `gc_threshold` (float, default 0.01)
- `search_limit` (int, default 10)
- `search_threshold` (float, default 0.7)
- `recall_threshold` (float, default 0.3)

Source: `cmd/cerebro/config.go` lines 20-51.

Precedence: CLI flag > brain config > compiled default. Config travels with the brain via export/import. This system is well-designed and self-contained.

### 2.3 Hook template (current)

The settings.json template (`templates/settings.json`) defines hooks for 6 events:

- `SessionStart` (startup, resume, compact, clear) -- memory priming
- `UserPromptSubmit` -- fallback priming with sentinel dedup
- `PreCompact` -- timestamp logging
- `PostCompact` -- sentinel clearing
- `SessionEnd` -- garbage collection

All hooks are `type: "command"`. None are quality-enforcement hooks (no Stop, no PreToolUse). The hooks exist solely to manage Cerebro's memory lifecycle.

### 2.4 Existing user harness (outside Cerebro)

The user already has a sophisticated harness in `/Users/q/projects/.claude/`:

- 6 global CLAUDE.md rules (behavioral, not technical)
- 12 global swarm agents in `~/.claude/agents/`
- 6 project-level skills including a 283-line `/implement`
- Domain-specific setups (e.g., Terraform deny-lists, 11 domain agents, 27 commands)

This harness was built organically. The question is: should Cerebro manage any of it?

---

## 3. Proposal-by-Proposal Evaluation

### 3.1 Behavioral rules in CLAUDE.md

**Original proposal:** Add 4 universal rules (assumptions, read-before-write, doc linking, simplicity) to the Cerebro-managed CLAUDE.md section.

**Evidence assessment:** Mixed. The approach document (Vector 1) documents that CLAUDE.md conventions are ignored when thinking depth is insufficient. However, the global CLAUDE.md at `~/.claude/CLAUDE.md` already contains 6 behavioral rules, and the user reports these produce better sessions in projects with the full harness. The mechanism is: rules work when the model thinks deeply enough to apply them, and fail when it doesn't.

**Implementation cost:** Trivial. Edit `templates/claudemd_section.md` to add the rules. Zero Go code changes. One template file change, one test update.

**Risk:** Adding rules to the project-level CLAUDE.md section duplicates what the user may already have globally. Claude Code loads global + project + repo CLAUDE.md files and merges them. Duplicate rules waste tokens. The official best practice (code.claude.com/docs/en/best-practices) says CLAUDE.md should be 50-100 lines max.

**Verdict: REJECT as a Cerebro template change.** The user's global CLAUDE.md is the correct place for universal behavioral rules. Adding them to every project's CLAUDE.md via Cerebro creates duplication. If the user wants behavioral rules, they can add them to `~/.claude/CLAUDE.md` directly -- they already have. No Cerebro code change needed.

### 3.2 Evaluator agent

**Original proposal:** Create `~/.claude/agents/evaluator.md` -- a skeptical reviewer using `model: sonnet`.

**Evidence assessment:** The Anthropic article on harness design (cited in the original document, Section 3.1) provides the theoretical basis. The article warns that self-evaluation is degenerate and separate evaluators need "several rounds of prompt tuning." No evidence that anyone has deployed a pure-markdown evaluator agent in `~/.claude/agents/` and measured its effectiveness.

**Implementation cost:** The agent definition is a markdown file. Zero Cerebro code changes if the user creates it manually. If Cerebro were to scaffold it, it requires: one new template file, one new `//go:embed` directive, a new `scaffoldAgents()` function, a new `--agents` flag on `cerebro init`, and tests. Estimated: ~100 lines of Go + template, 4-6 tests.

**Risk:** An evaluator agent adds per-invocation cost (Sonnet pricing) and latency. Without calibration (the Anthropic article warns about this), it may produce false confidence -- praising mediocre work. The user already has `swarm-tech-lead.md` and other review agents in `~/.claude/agents/`.

**Verdict: DEFER.** This is a user-space configuration file, not a Cerebro concern. The user can create `~/.claude/agents/evaluator.md` today without any Cerebro changes. Cerebro scaffolding agents into the global directory (`~/.claude/agents/`) goes beyond Cerebro's scope -- Cerebro manages per-project memory and per-project integration files, not global Claude Code configuration. If there is demand after the user tests a hand-crafted evaluator agent, revisit.

### 3.3 Lightweight `/implement` skill

**Original proposal:** Add a 60-80 line universal implementation workflow as a Cerebro template.

**Evidence assessment:** The user has a 283-line `/implement` skill at `/Users/q/projects/.claude/skills/implement/SKILL.md` that works. The original document argues this skill is not portable because skills are project-scoped. This is correct -- Claude Code has no `~/.claude/skills/` mechanism.

**Implementation cost:** One new template file, one new `//go:embed` directive, add to the `scaffoldSkills()` map, update tests. Estimated: ~80 lines of template, ~20 lines of Go changes, 2-3 new tests.

**Risk:** A lightweight `/implement` may conflict with the user's existing heavy `/implement`. Claude Code loads project-local skills, so the heavy version overrides the template -- but only if both exist. In a fresh project where only the lightweight template exists, the user gets a degraded experience if they're used to the heavy version. The skill also references tools and agents that may not exist in every project (context7, swarm agents, Jira integration).

**Verdict: CONDITIONALLY ACCEPT.** A lightweight `/implement` template is the single highest-ROI addition because it enforces the "research before action" pattern that the evidence shows is critical (read:edit ratio collapse, #42796). But it must be genuinely universal -- no references to Jira, swarm agents, or project-specific tooling. It should be: (1) read target files, (2) verify assumptions, (3) present plan and STOP for approval, (4) execute, (5) verify. Five phases, no external dependencies beyond basic Claude Code tools.

**Implementation detail:** Add `templates/skill_implement.md` (~60-80 lines). Add `//go:embed templates/skill_implement.md` and `var skillImplementTemplate []byte` to `scaffold.go`. Add `"implement": skillImplementTemplate` to the `skills` map in `scaffoldSkills()`. Add 2-3 tests to `scaffold_test.go`. Total diff: ~100 lines.

### 3.4 `/preflight` skill

**Original proposal:** Pre-commit verification checklist skill.

**Evidence assessment:** None cited. The original document lists it in Tier 1 but "never describes" it (noted as a gap in the architect review, Section 7.5). No reference implementation exists in the community.

**Implementation cost:** One template file + minor Go changes (same pattern as `/implement`).

**Risk:** Duplicates what CI should do. A pre-commit hook in git (not Claude Code) is a more reliable enforcement mechanism for lint/test/format checks.

**Verdict: REJECT.** No evidence this is needed. Pre-commit checks belong in git hooks or CI, not in a Claude Code skill that depends on the model choosing to invoke it. This is exactly the kind of advisory mechanism that fails when thinking depth is insufficient (approach.md, Vector 1).

### 3.5 Template management system (manifest.json, checksum-based skip, three-way diff)

**Original proposal:** `.cerebro-manifest.json` with SHA-256 checksums, `cerebro harness sync` command, `cerebro harness diff` command, `cerebro harness status` command, ownership markers.

**Evidence assessment:** None. No evidence that the current `cerebro init --force` mechanism is insufficient. The user has been using `cerebro init --force` since v1.5.2 to update templates.

**Implementation cost:** This is the most expensive proposal. Estimated:

- `manifest.go`: Manifest read/write, checksum calculation, comparison logic (~200 lines)
- `cmd_harness.go`: Three new cobra commands (status, sync, diff) (~300 lines)
- `.cerebro-manifest.json` format definition and serialization
- Three-way diff implementation (or dependency on an external diff library)
- Ownership marker parsing and validation
- Tests: 15-20 new tests for all the new paths
- Total: ~600-800 lines of new Go code, plus test code

**Risk:** This is a configuration management system. Cerebro is a memory system. The complexity is disproportionate to the problem: Cerebro currently has 5 template files, and the proposal adds perhaps 2-3 more (implement skill, maybe evaluator agent). Managing 7-8 files with a manifest/checksum/three-way-diff system is over-engineering.

The comparison to `create-react-app` eject is misleading. CRA managed hundreds of configuration files across webpack, babel, eslint, jest, and TypeScript. Cerebro manages a settings.json merge + a few markdown files. The existing `--force` flag already handles the update case, and the existing skip-if-present logic handles the no-force case.

**Verdict: REJECT.** The existing `cerebro init` and `cerebro init --force` pattern is sufficient. The user can see what changed by reading the CHANGELOG for each release. If the template set grows to 15+ files in the future, revisit. For 5-8 files, a manifest system is pure overhead.

### 3.6 Stop hook (command-type, stop-phrase-guard)

**Original proposal:** A command-type Stop hook matching phrases that indicate premature stopping, permission-seeking, or ownership-dodging. Based on stellaraccident's implementation from #42796.

**Evidence assessment:** Strong. The #42796 stop-phrase-guard went from 0 violations to 173 in 17 days (evidence-catalog.md, March 8-25 timeline). This is the most well-evidenced quality intervention in the entire research corpus.

**Implementation cost:** This is a shell script, not Go code. It can be added to the settings.json template as a new Stop hook entry. The hook reads stdin (Claude's proposed stop message), checks for phrases, exits 0 with `{"decision": "block", "reason": "..."}` to prevent stopping or `{"decision": "approve"}` to allow it.

However: the hook script itself needs to live somewhere accessible. Options:
1. **Inline in settings.json**: Possible but ugly. The current settings.json hooks are already long one-liners.
2. **Separate script file scaffolded by `cerebro init`**: Requires adding file scaffolding beyond `.claude/skills/` and `.claude/settings.json`.
3. **Shipped with the cerebro binary**: Could be `cerebro stop-guard` subcommand that reads stdin and exits with the right code.

**Known issues:**
- Stop hooks do NOT fire in VSCode extension (GitHub #40029, confirmed bug, open as of 2026-04-22). This means the hook only works in CLI mode.
- Prompt-type Stop hooks are broken (#11610, #11786). Command-type hooks work.
- Stop hooks fire on EVERY response completion, not just task completion (code.claude.com/docs/en/hooks). This means the phrase matcher runs frequently, which is fine for a fast shell script but matters for latency.

**Verdict: CONDITIONALLY ACCEPT as a template addition.** Add a simple command-type Stop hook to the settings.json template. Keep the phrase list small (10-15 high-confidence patterns, not 30+) to reduce false positives. Ship it as disabled-by-default with a comment explaining how to enable it. The user can enable it by uncommenting.

Alternatively, and this is the simpler path: document the stop-phrase-guard pattern in the CLAUDE.md template so the user can set it up manually. This requires zero code changes.

**The honest assessment:** The highest-value version is a `cerebro stop-guard` subcommand that reads stdin and outputs the JSON decision. This way the settings.json hook is just `"command": "cat | cerebro stop-guard"`. The Go binary ships the phrase list, making updates atomic with cerebro releases. Estimated: ~80 lines of Go (new cobra command), ~40 lines of tests, one line added to settings.json template. But this only works in CLI mode due to #40029.

### 3.7 PreToolUse read-before-edit enforcer

**Original proposal:** Track which files have been read, block/warn on Write/Edit to unread files.

**Evidence assessment:** Conceptually sound -- addresses the read:edit ratio collapse from 6.6 to 2.0 (#42796). But no reference implementation exists anywhere in the community.

**Implementation cost:** High. This requires:
- Cross-turn state tracking: which files were read in the current session
- A PostToolUse hook that logs Read tool calls (file paths) to a session-specific temp file
- A PreToolUse hook on Write/Edit that checks the temp file before allowing the edit
- Session ID extraction from stdin JSON
- Cleanup on SessionEnd

This is not a simple shell one-liner. It's a stateful hook system. Estimated: 150-200 lines of shell scripting, or a Go subcommand.

**Risk:** False positives. Claude may "read" a file via Grep or Glob rather than the Read tool. The hook would need to track multiple tool types. Also, the hook fires on every Write/Edit, adding latency.

**Verdict: DEFER.** The concept is valuable but the implementation is complex and untested. No reference implementation exists to validate the approach. Start with the Stop hook (proven) and read:edit ratio monitoring (post-hoc, from session logs). If those prove insufficient, build this.

### 3.8 Domain packs (`cerebro init --pack terraform/node/python`)

**Original proposal:** Domain-specific agents, deny-lists, validation commands.

**Evidence assessment:** None. The original document defers this with a trigger condition ("when 3+ projects are running with the core harness").

**Verdict: REJECT.** This is scope creep. Cerebro is a memory system. Domain-specific Claude Code configuration is the user's responsibility. The original document's own architect review (Section 7.2) flags this as where "scope creep risk is real."

### 3.9 `cerebro init --agents`

**Original proposal:** Scaffold evaluator, PM, architect, tech-lead agents to `~/.claude/agents/`.

**Evidence assessment:** None. The user already has 12 global agents created manually.

**Implementation cost:** New function `scaffoldAgents()`, new flag, new templates, tests. ~150 lines.

**Risk:** Cerebro writing to `~/.claude/agents/` crosses a scope boundary. Cerebro manages per-project state (brain, hooks, skills, CLAUDE.md section). Global Claude Code configuration is a different concern.

**Verdict: REJECT.** Same reasoning as 3.2. The user manages global agents themselves. Cerebro should not write to `~/.claude/`.

---

## 4. The "Phase 1 Delivers 60-70% of Value" Claim

The original document (Section 8.1) claims: "Phase 1 requires ZERO cerebro changes" and "delivers 60-70% of value." Phase 1 consists of:

1. Add 4 behavioral rules to `~/.claude/CLAUDE.md`
2. Create `~/.claude/agents/evaluator.md`
3. Measure across 5-10 real tasks

**Evaluation:** The claim is partially true but misleading.

**What's true:** Items 1 and 2 require zero Cerebro code changes. They are manual user configuration. The "zero cerebro changes" part is accurate.

**What's unsupported:** The "60-70%" value claim has no evidence behind it. It's a guess. The evidence shows:

- Behavioral rules in CLAUDE.md fail when the model allocates zero thinking tokens (approach.md, Vector 1). Effort=high does not prevent this (evidence-catalog.md, Cherny HN item 47668520).
- No evaluator agent has been tested in the `~/.claude/agents/` context.
- The 30% improvement estimate is not grounded in any measurement.

**What should actually be done first:** The evidence points to two highest-ROI actions, neither of which is in "Phase 1":

1. **Deploy the settings baseline from settings-matrix.md** (effort=high minimum, showThinkingSummaries=true, earlier compaction). This is Tier 1 from approach.md.
2. **Add a command-type Stop hook** based on the stellaraccident pattern. This is the only intervention with quantitative evidence of effectiveness.

Neither of these requires Cerebro code changes. Both are settings.json modifications.

---

## 5. Revised Proposal: What Cerebro Should Actually Do

Based on the evidence, here is what I recommend, ordered by ROI:

### Tier A: Template updates (no new Go code, just template content)

**A1. Add stop-phrase-guard hook to settings.json template**

Add a Stop hook entry to `templates/settings.json`. The hook is a command-type bash one-liner that reads stdin, checks for premature-stop phrases, and outputs a JSON decision.

Files to change:
- `cmd/cerebro/templates/settings.json` -- add Stop hook array
- `cmd/cerebro/scaffold_test.go` -- verify Stop hook appears in scaffolded output

Estimated diff: ~30 lines in template, ~10 lines in test.

Evidence: #42796 Appendix B -- 0 to 173 violations in 17 days. Proven quality canary.

Limitation: Does not fire in VSCode extension (#40029). CLI-only.

Implementation detail -- the hook:
```json
{
  "matcher": "",
  "hooks": [
    {
      "type": "command",
      "command": "INPUT=$(cat); STOP_MSG=$(echo \"$INPUT\" | python3 -c \"import sys,json; print(json.load(sys.stdin).get('stop_message',''))\" 2>/dev/null); if echo \"$STOP_MSG\" | grep -qiE '(shall I|would you like me to|I can stop here|let me know if|I\\'ll leave it|beyond the scope|for now|as a future|out of scope|that should be)'; then printf '{\"decision\": \"block\", \"reason\": \"Do not stop prematurely. Continue working.\"}'; else printf '{\"decision\": \"approve\"}'; fi"
    }
  ]
}
```

Note: This uses a conservative 10-phrase list. The stellaraccident implementation uses 30+. Start small, measure false positives, expand.

**A2. Update CLAUDE.md template with configuration guidance**

The v1.7.0 CLAUDE.md template already added a configuration section. No further changes needed unless the settings baseline from `settings-matrix.md` is deployed (in which case, add a note about the deployed settings).

Files to change: None (already done in v1.7.0).

### Tier B: New template (minimal Go code)

**B1. Add lightweight `/implement` skill template**

Add a universal implementation workflow skill. Must be dependency-free (no Jira, no swarm agents, no context7 references).

Files to change:
- `cmd/cerebro/templates/skill_implement.md` -- new file (~70 lines)
- `cmd/cerebro/scaffold.go` -- add `//go:embed` directive and template var (~3 lines)
- `cmd/cerebro/scaffold.go` -- add to `scaffoldSkills()` map (~1 line)
- `cmd/cerebro/scaffold_test.go` -- add tests for implement skill scaffolding (~30 lines)

Estimated diff: ~100 lines total.

Evidence: The read:edit ratio collapse from 6.6 to 2.0 (#42796) directly results from the model editing without reading. A skill that enforces "read, research, plan, STOP, then execute" addresses this. The user's 283-line `/implement` produces good results in projects where it exists; a lightweight version makes this portable.

Risk: The skill is advisory -- the model must choose to follow it. When thinking depth is insufficient, it may skip phases. However, skills are invoked explicitly by the user (`/implement`), which provides stronger enforcement than CLAUDE.md rules loaded passively.

### Tier C: New Go subcommand (moderate code)

**C1. `cerebro stop-guard` subcommand**

A Go command that reads stdin (hook input JSON), extracts the stop message, checks against a phrase list, and outputs the JSON decision. This replaces the inline bash in the settings.json template with a cleaner `"command": "cerebro stop-guard -p \"$CLAUDE_PROJECT_DIR\""`.

Files to change:
- `cmd/cerebro/cmd_stop_guard.go` -- new file (~80 lines)
- `cmd/cerebro/cmd_stop_guard_test.go` -- new file (~60 lines)
- `cmd/cerebro/templates/settings.json` -- update Stop hook to use `cerebro stop-guard`

Estimated diff: ~170 lines.

Advantages over inline bash:
- Phrase list ships atomically with the binary version
- Testable in Go (unit tests for each phrase category)
- Can be extended later (configurable phrase list via `cerebro config`, per-project overrides)
- Cross-platform (no dependency on python3 for JSON parsing)

This is a natural extension of the existing pattern: `cerebro gc` runs from the SessionEnd hook, `cerebro recall --prime` runs from the SessionStart hook. `cerebro stop-guard` would run from the Stop hook.

### Tier D: Deferred (no evidence of need yet)

| Proposal | Reason to defer |
|----------|----------------|
| PreToolUse read-before-edit enforcer | No reference implementation exists. Complex stateful hook. |
| Template management system (manifest/checksum/diff) | 5-8 files don't need a manifest system. `--force` is sufficient. |
| Agent scaffolding (`--agents`) | Global agents are the user's concern, not Cerebro's. |
| Domain packs (`--pack`) | Unbounded scope creep. |
| Evaluator agent | Untested concept. User can create manually. |
| `/preflight` skill | CI/git hooks are better enforcement. |
| `cerebro harness status/sync/diff` commands | Over-engineering for current template count. |
| Paired section markers in CLAUDE.md | Current single-marker + trailing-section preservation works (tested in `scaffold_test.go`). |
| Ownership markers (`<!-- managed-by: cerebro -->`) | Only useful with manifest system, which is rejected. |

---

## 6. Implementation Plan

### Phase 1: Template-only changes (1-2 hours)

1. Add Stop hook to `templates/settings.json`
2. Add `templates/skill_implement.md`
3. Update `scaffold.go` embed directives and `scaffoldSkills()` map
4. Add tests to `scaffold_test.go`
5. Update CHANGELOG.md

All changes stay within the existing scaffold pattern. No new subsystems. No new dependencies. No new CLI commands.

Files changed: 4 (templates/settings.json, templates/skill_implement.md [new], scaffold.go, scaffold_test.go)
Test coverage: Extend existing scaffold tests to cover new templates.

### Phase 2: Stop-guard subcommand (2-4 hours, after Phase 1 is deployed and measured)

1. Add `cmd_stop_guard.go` with cobra command
2. Read stdin JSON, extract stop message
3. Match against configurable phrase list
4. Output JSON decision
5. Update settings.json template to use `cerebro stop-guard`
6. Add comprehensive tests

Files changed: 3 new (cmd_stop_guard.go, cmd_stop_guard_test.go), 1 modified (templates/settings.json)

### Phase 3: Measurement (1-2 weeks of normal use)

Before building anything else, measure:
- How often does the Stop hook fire? (Add a counter to the hook output log)
- Does the `/implement` skill reduce the need for user interrupts?
- Are there new degradation patterns the current hooks don't catch?

Only proceed to Tier D items if measurement shows they're needed.

---

## 7. What the Original Document Got Right

Credit where due -- the original `harness-management-research.md` made several correct calls:

1. **Rejecting prompt-type Stop hooks** (Section 6.1): Correctly identified the regression (#11610, #11786) and the self-evaluation degeneracy problem. Well-researched.

2. **Deferring agent-type Stop hooks** (Section 6.2): Correct. The concern about infinite loops, VSCode incompatibility, and overkill for trivial interactions is valid.

3. **Deferring domain packs** (Section 4.2 Tier 3): The trigger condition ("when 3+ projects are running") is a good discipline.

4. **Tiered approach**: The idea of starting simple and escalating is sound. The specific tier contents need revision (per this document), but the principle is correct.

5. **Hook durability argument** (approach.md, Section 4, Vector 6): Hooks survive model transitions because they operate outside the model's control plane. This is the foundational insight that motivates everything in this proposal.

6. **Embedding templates in the binary** (Section 5.5): Correct decision. `//go:embed` is already in use, templates are small, no network dependency needed.

## 8. What the Original Document Got Wrong

1. **Attributed "Go-native engine" to gstack** (Section 3.2): gstack is a pure-Markdown skills toolkit. The Go-native guardrail engine is Chachamaru127's claude-code-harness. These are different projects.

2. **Overestimated Phase 1 value** (Section 8.1): The "60-70% of value with zero cerebro changes" claim has no measurement behind it. Behavioral rules fail when thinking depth is zero (the confirmed mechanism from evidence-catalog.md).

3. **Template management complexity** (Section 5): manifest.json, checksum-based skip, three-way diff, and ownership markers are enterprise-grade configuration management for 5-8 markdown files. Disproportionate to the problem.

4. **Scope creep into global config** (Section 4.2, 4.3): Cerebro managing `~/.claude/agents/` crosses a scope boundary. Cerebro manages per-project state (brain database, project hooks, project skills, project CLAUDE.md section). Global Claude Code configuration is a different domain.

5. **Missing the settings baseline**: The original document doesn't mention deploying the settings baseline from `settings-matrix.md` (effort=high, showThinkingSummaries=true, earlier compaction). This is the actual lowest-effort, highest-impact change -- and it requires zero code changes of any kind.

---

## 9. Corrected Factual Claims

| Claim in original | Correction | Source |
|-------------------|-----------|--------|
| "gstack ... Go-native engine: ~10ms per phase processing" (Section 3.2) | The Go-native engine is Chachamaru127's claude-code-harness, not gstack. gstack is pure-Markdown skills files. | Web search: gstack is 23 Markdown-based slash commands (garrytan/gstack README) |
| "Skills are project-scoped only ... no global mechanism" (Section 2.3) | Correct, but understated. This is a Claude Code design decision, not a bug. Global skills would conflict with project-local customization. | code.claude.com/docs/en/best-practices |
| "Phase 1 delivers 60-70% of value" (Section 8.1) | Unmeasured claim. Behavioral rules fail when thinking depth is zero (confirmed mechanism). Settings baseline is the actual lowest-effort change. | approach.md Section 4 Vector 1, evidence-catalog.md Cherny HN thread |
| "Stop hook: Exit code 1 blocks the stop" (various) | Stop hooks use JSON output (`{"decision": "block"}`) on exit 0, not exit codes. Exit code 2 with stderr JSON is an alternative pattern but exit 0 + stdout JSON is the documented approach. | code.claude.com/docs/en/hooks, Context7 claude-code docs |

---

## 10. Testing Strategy

### For template changes (Phase 1)

Extend existing `scaffold_test.go` patterns:

1. **TestScaffoldSettings_HasStopHook**: Verify Stop hook appears in freshly scaffolded settings.json
2. **TestScaffoldSettings_StopHookPreservedOnMerge**: Verify Stop hook is not duplicated when cerebro hooks already exist
3. **TestScaffoldSkills_ImplementCreated**: Verify implement skill is scaffolded
4. **TestScaffoldSkills_ImplementNotOverwritten**: Verify existing implement skill is not clobbered without `--force`

### For stop-guard subcommand (Phase 2)

1. **TestStopGuard_BlocksPrematureStopPhrases**: Feed known phrases, verify `{"decision": "block"}`
2. **TestStopGuard_ApprovesNormalStop**: Feed normal completion messages, verify `{"decision": "approve"}`
3. **TestStopGuard_HandlesEmptyInput**: Verify graceful handling of empty or malformed stdin
4. **TestStopGuard_HandlesNoStopMessage**: Verify graceful handling of JSON without stop_message field
5. **TestStopGuard_PhraseListCoverage**: Verify each phrase category has at least one test case

### For behavioral changes (ongoing)

These cannot be unit tested because they depend on model behavior. The approach document (Section 9) recommends:

- Run 5-10 regression prompts across non-EDP projects
- Compare stop-hook violation counts before/after
- Track user interrupt rate (manual, from session experience)

This is fundamentally a "deploy and observe" situation. There is no deterministic test for "does Claude stop prematurely less often with this hook." The stop-hook violation count is the closest proxy.

---

## 11. Maintenance Burden Assessment

| Change | Ongoing maintenance | Burden |
|--------|-------------------|--------|
| Stop hook in settings.json template | Phrase list may need updates as model behavior changes | Low -- bash one-liner, edit template file |
| `/implement` skill template | May need updates as Claude Code skill format evolves | Low -- markdown file |
| `cerebro stop-guard` subcommand | Phrase list management, stdin JSON format changes | Medium -- Go code, but contained in one file |
| Template management system (REJECTED) | Manifest format, checksum logic, diff algorithm, ownership markers | High -- ongoing maintenance of a configuration management subsystem |
| Agent scaffolding (REJECTED) | Agent format changes, model availability, global directory management | Medium -- but crosses scope boundary |

The recommended changes (A1, B1, C1) have low-to-medium maintenance burden and stay within the existing patterns. The rejected changes (manifest system, domain packs) would create significant ongoing maintenance.

---

## 12. Summary of Decisions

| Proposal | Decision | Rationale |
|----------|----------|-----------|
| Settings baseline deployment | DO FIRST (no code) | Highest ROI, zero implementation cost. Use settings-matrix.md baseline. |
| Stop hook in settings.json | ACCEPT (Tier A) | Proven effective (#42796). Template-only change. |
| `/implement` skill template | ACCEPT (Tier B) | Addresses read:edit ratio collapse. Minimal code change. |
| `cerebro stop-guard` subcommand | ACCEPT (Tier C) | Natural extension of existing pattern. Better than inline bash. |
| Behavioral rules in CLAUDE.md | REJECT | User manages global rules; per-project duplication wastes tokens. |
| Evaluator agent | DEFER | Untested concept. User can create manually. |
| `/preflight` skill | REJECT | CI/git hooks are better enforcement. |
| Template management system | REJECT | Over-engineering for 5-8 files. `--force` is sufficient. |
| `cerebro init --agents` | REJECT | Crosses scope boundary. Global config is user's domain. |
| Domain packs | REJECT | Unbounded scope creep. |
| PreToolUse read-before-edit | DEFER | No reference implementation. Complex state management. |
| Paired CLAUDE.md markers | REJECT | Current implementation works and is tested. |

---

## 13. Open Questions

1. **Stop hook stdin format**: What exactly does Claude Code pass to Stop hook stdin? The evidence shows `stop_message` field but the exact JSON schema is not fully documented. Need to test empirically before implementing.

2. **Stop hook false positive rate**: The stellaraccident implementation used 30+ phrases. How many of those fire on legitimate completions? Need measurement before expanding beyond a conservative 10-phrase list.

3. **VSCode Stop hook bug timeline**: GitHub #40029 is open. If this is fixed, the Stop hook becomes universally useful. If not, it's CLI-only, which limits its value.

4. **`cerebro config` for stop-guard phrases**: Should the phrase list be configurable via `cerebro config`? This would allow per-brain customization but adds complexity. Defer until Phase 2 is deployed and measured.

5. **Does the `/implement` skill actually get invoked?** Skills require the user to type `/implement`. If users forget to use it, the skill provides no value. The evidence from the user's existing 283-line `/implement` suggests this is not a problem for this specific user, but it may be for others.
