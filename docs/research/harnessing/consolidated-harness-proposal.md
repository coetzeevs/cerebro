# Consolidated Harness Proposal: Extending Cerebro for Claude Code Quality Enforcement

**Date:** 2026-04-22
**Version:** 1.0
**Status:** Final proposal for decision
**Inputs:** approach.md (v1.2), evidence-catalog.md, settings-matrix.md, session-summary.md, architect-harness-proposal.md, techlead-harness-proposal.md, cross-reviews, fact-check-results.md, codebase audit

---

## 0. Why This Document Exists

The original `harness-management-research.md` was written before the evidence base was assembled. It proposed a "brain + harness manager" architecture based on intuition about Claude Code's quality degradation and portability gaps. Four research documents and two independent reviews later, several of its foundational assumptions are wrong, several proposals are over-engineered, and the prioritization is backwards.

This document replaces the original with an evidence-grounded proposal. Every claim cites a source. Where evidence is absent, that gap is stated explicitly. Where predictions are made, they are labelled as such with supporting reasoning.

---

## 1. What the Evidence Establishes

### 1.1 The degradation is real, multi-vector, and ecosystem-level

Six root cause vectors are documented (approach.md, Section 4):

| Vector | Confirmed mechanism | Primary source |
|--------|---------------------|----------------|
| V1: Thinking depth | Zero-thinking-tokens turns even at effort=high | Boris Cherny HN item 47668520 |
| V2: Prompt caching bugs | 10-20x token inflation | #40524, ArkNill repo |
| V3: Context window degradation | Quality degrades at 20% of 1M window | Community reports, settings-matrix.md Section 2 |
| V4: Load-sensitive allocation | Time-of-day variance in thinking depth | #42796 (correlational, not causal) |
| V5: Behavioral drift | Same patterns recur across 4.5, 4.6, 4.7 | evidence-catalog.md Patterns 1, 3 |
| V6: Lever removal | budget_tokens, DISABLE_ADAPTIVE_THINKING deprecated | evidence-catalog.md Pattern 2 |

### 1.2 What a harness can and cannot address

A harness (hooks, skills, settings, CLAUDE.md) operates on the user's machine. It cannot influence server-side model behavior. This limits its scope:

| Vector | Harness contribution | Why |
|--------|---------------------|-----|
| V1: Thinking depth | **Low-medium** | Settings are necessary but not sufficient (Cherny confirmed zero-thinking even at effort=high). Skills with `effort: high` frontmatter provide the most targeted intervention but cannot prevent zero-thinking turns. |
| V2: Caching bugs | **None** | Upstream runtime bug. Fix is in Claude Code version management. |
| V3: Context degradation | **Medium** | Settings (compaction thresholds), structured workflows (shorter focused phases), compaction hooks. |
| V4: Load allocation | **None** | Server-side. Unverifiable from user's machine. |
| V5: Behavioral drift | **High** | Stop hooks catch premature stopping. PreToolUse hooks enforce read-before-edit. Skills enforce workflows. This is the primary justification for a harness. |
| V6: Lever removal | **Medium-high** | Hooks are the most durable enforcement mechanism because they operate outside the model's control plane (approach.md, Section 4, Vector 6). |

**Bottom line:** A harness meaningfully addresses 3 of 6 vectors (V3, V5, V6), partially addresses 1 (V1), and cannot address 2 (V2, V4). The original document did not perform this analysis, which led it to overstate what a harness can deliver.

### 1.3 What has quantitative evidence of working

Only two interventions have machine-readable evidence from #42796:

1. **Stop-phrase-guard**: 0 violations in entire history before March 8, then 173 violations in 17 days. This is a proven quality canary and the single most validated mitigation in the evidence base (approach.md Section 3.1, evidence-catalog.md timeline).

2. **Read:edit ratio monitoring**: Ratio dropped from 6.6 to 2.0 during degradation, measuring the "edit-without-reading" behavior. This was measured post-hoc from JSONL session logs, not enforced in real-time. No real-time enforcement implementation exists anywhere in the community.

Everything else -- behavioral rules, evaluator agents, structured skills -- is theoretically justified but empirically unproven in the Claude Code context.

---

## 2. Corrections to the Original Document

These are verified facts that the original `harness-management-research.md` got wrong. Both independent reviewers caught some; fact-checking confirmed all.

### 2.1 Personal skills exist and are global

**Original claim** (Section 1, portability table; Section 2.3; Section 4.3; Section 7.3): "Skills are project-scoped only -- no global skills mechanism exists in Claude Code."

**Fact:** Personal skills at `~/.claude/skills/<skill-name>/SKILL.md` are "available across all your projects" (fact-check-results.md, Claim 1; Claude Code skills docs). The portability gap that motivated the entire harness manager proposal is narrower than claimed.

### 2.2 Skill precedence is the opposite of assumed

**Original assumption:** Project-local skills override global templates.

**Fact:** Precedence is enterprise > personal > project (fact-check-results.md, Claim 1b). A personal skill at `~/.claude/skills/implement/` would override every project's custom version. This reverses the layering model the original document assumed.

### 2.3 Skills support effort, model, and hooks frontmatter

**Original document:** No mention of these capabilities.

**Fact:** All six queried frontmatter fields are confirmed (fact-check-results.md, Claim 2). Most significantly:
- `effort: high` on a skill forces deep reasoning when the skill is active -- the most targeted intervention against V1
- `hooks` in skill frontmatter enables skill-scoped enforcement (e.g., read-before-edit only during `/implement`)
- Skills survive compaction (5K tokens per skill, 25K total budget) -- more durable than CLAUDE.md rules

### 2.4 Stop hook input field name is wrong

**Original and both reviewers assumed:** `stop_message` field.

**Fact:** The field is `last_assistant_message` (fact-check-results.md, Claim 4). Additional Stop-specific field: `stop_hook_active` (boolean indicating whether this turn is already a continuation from a prior Stop hook block).

### 2.5 Stop hook blocking uses JSON decision protocol

**Both valid mechanisms** (fact-check-results.md, Claim 3):
- Exit 0 + JSON `{"decision": "block", "reason": "..."}` on stdout -- the Stop-hook-specific protocol
- Exit 2 + stderr message -- the generic blocking mechanism for all hooks

For Stop hooks, the documentation shows: omit the `decision` field entirely to allow stopping (not `"approve"` -- that appears only in plugin-dev teaching examples).

### 2.6 Agent hooks are not experimental

**Architect's claim:** Agent hooks are "experimental and may change."

**Fact:** Agent hooks are documented as one of four standard hook types (`command`, `http`, `prompt`, `agent`) with no experimental label (fact-check-results.md, Claim 8). They support up to 50 tool-use turns.

### 2.7 VSCode Stop hook bug is undocumented

**Multiple citations of GitHub #40029** across both proposals.

**Fact:** No VSCode-specific Stop hook issue appears in the documentation (fact-check-results.md, Claim 7). The GitHub issue may exist but is not reflected in official docs. The docs do state Stop hooks fire "when Claude finishes responding" and are not fired by user interrupts or `max_turns` limits.

### 2.8 The "60-70% value from Phase 1" claim is unmeasured

**Original document** (Section 8.1): "Phase 1 delivers 60-70% of value."

**Fact:** No measurement exists. Behavioral rules in CLAUDE.md fail when thinking depth is zero (the confirmed mechanism from V1). The tech lead correctly identifies this as an unfounded claim.

### 2.9 gstack attribution error

**Original document** (Section 3.2): Attributes "Go-native engine: ~10ms per phase processing" to gstack.

**Fact:** gstack is pure-Markdown skills files (garrytan/gstack). The Go-native engine is Chachamaru127's `claude-code-harness`.

---

## 3. Architecture Decision: What to Build

### 3.1 The "brain + harness manager" framing

**Verdict: Partially justified, but the scope should be smaller than proposed.**

Cerebro already does harness work: `cerebro init` scaffolds hooks, skills, and CLAUDE.md. `cerebro init --force` updates them. The scaffolding code is 298 lines (scaffold.go), uses `//go:embed`, and has 15 tests. Extending this to scaffold better defaults is incremental work with low risk.

What is NOT justified: a template management subsystem (manifest, checksums, three-way diffs, `harness sync/status/diff` commands). The template set is 5-8 files totaling under 15KB. The `--force` mechanism is sufficient. Both reviewers agree on this independently.

### 3.2 What to build (accepted proposals)

| Item | Type | Evidence | Source |
|------|------|----------|--------|
| `cerebro stop-guard` subcommand | New Go command | #42796: 0→173 violations in 17 days | architect + tech lead agree |
| Stop hook in settings.json template | Template change | Same evidence; auto-added by `addMissingEvents()` on next `cerebro init` | architect + tech lead agree |
| `/implement` skill template | New template | Read:edit ratio collapse 6.6→2.0 (#42796); user's 283-line version proves the pattern | architect + tech lead agree |
| `effort: high` in `/implement` frontmatter | Template content | Skill frontmatter confirmed (fact-check); most targeted V1 intervention | architect found, tech lead accepted |

### 3.3 What NOT to build (rejected proposals)

| Item | Reason | Both reviewers agree? |
|------|--------|-----------------------|
| Template management system (manifest, checksums, diff) | Over-engineering for 5-8 files. `--force` is sufficient. | Yes |
| `cerebro harness sync/status/diff` commands | No evidence of demand. `cerebro init --force` already works. | Yes |
| `/preflight` skill | Never specified. CI/git hooks are the right enforcement layer. | Yes |
| `cerebro init --agents` | Crosses scope boundary. Cerebro manages per-project state; `~/.claude/agents/` is user-managed. | Yes |
| Domain packs (`--pack terraform/node/python`) | Unbounded scope creep. Correctly deferred in the original. | Yes |
| Ownership markers (`<!-- managed-by: cerebro -->`) | Only useful with the manifest system, which is rejected. | Yes |
| Paired CLAUDE.md section markers | Current single-marker + trailing-section preservation works and is tested (scaffold_test.go). No failure case demonstrated. | Tech lead rejects; architect wants. Resolution: defer until a real failure surfaces. |
| Behavioral rules in Cerebro's CLAUDE.md template | User manages global rules in `~/.claude/CLAUDE.md`. Per-project duplication wastes tokens. Architect acknowledged this logic then contradicted it. | Tech lead rejects; architect contradicts self. Resolution: don't template it -- user adds rules globally. |

### 3.4 What to defer with explicit triggers

| Item | Trigger condition | Evidence gap |
|------|-------------------|-------------|
| PreToolUse read-before-edit enforcer | Measurement shows read:edit ratio problem persists after stop-guard + /implement | No reference implementation exists anywhere. Complex stateful hook (cross-turn tracking). |
| Agent-type Stop hook (evaluator) | Measurement shows assumption-making persists after stop-guard + /implement | Agent hooks work but no deployment evidence in this context. Anthropic warns evaluator agents need "several rounds of prompt tuning." |
| Evaluator agent definition | User can create `~/.claude/agents/evaluator.md` manually for Phase 0 testing | No evidence of measured effectiveness |

---

## 4. Implementation Plan

### Phase 0: Deploy proven interventions (no Cerebro code changes)

**Timeline:** Immediate. Estimated effort: 1 hour.

These require editing user configuration files only. No Go code, no builds, no tests.

| # | Action | File | Evidence | Rationale |
|---|--------|------|----------|-----------|
| 0a | Deploy settings baseline | `~/.claude/settings.json` | settings-matrix.md Section 8 | Highest ROI, zero implementation cost. `effortLevel: "high"`, `showThinkingSummaries: true`. Both reviewers agree this was missing from the original document's Phase 1. |
| 0b | Deploy Stop hook (inline bash) | `~/.claude/settings.json` | #42796: 0→173 violations | Most quantitatively validated intervention. Deploy at user level for global coverage. |
| 0c | Add behavioral rules | `~/.claude/CLAUDE.md` | approach.md Section 2 problem statement | Addresses V5. Advisory, but low cost. The user's current 6 lines are well below the 50-100 line guidance. |
| 0d | Create `/implement` skill | `~/.claude/skills/implement/SKILL.md` (personal scope) | User's 283-line version proves the pattern | **Prediction:** Placing at personal scope provides global coverage. Risk: overrides any project-level `/implement`. Mitigated by the fact that higher-priority is what we want -- a project can opt out by naming its skill differently. |
| 0e | Measure baseline | 5-10 tasks across non-EDP projects | approach.md success criteria | Stop hook violation count, user interrupt frequency, subjective quality assessment. |

**Phase 0 Stop hook (inline version for immediate deployment):**

```json
{
  "hooks": {
    "Stop": [
      {
        "matcher": "",
        "hooks": [
          {
            "type": "command",
            "command": "INPUT=$(cat); MSG=$(echo \"$INPUT\" | grep -o '\"last_assistant_message\":\"[^\"]*\"' | head -1 | cut -d'\"' -f4); if echo \"$MSG\" | grep -qiE '(shall I|would you like me to|I can stop here|let me know if|I.ll leave it|beyond the scope|for now,|as a future|out of scope|that should be sufficient)'; then echo '{\"decision\": \"block\", \"reason\": \"Do not stop prematurely. Continue working on the task.\"}'; else echo '{}'; fi"
          }
        ]
      }
    ]
  }
}
```

**Design notes:**
- Reads `last_assistant_message` from stdin JSON (verified field name, fact-check Claim 4)
- Uses exit 0 + JSON decision protocol (verified, fact-check Claim 3)
- Omits `decision` field (outputs `{}`) to allow stopping when no phrase matches (verified: "omit to allow", fact-check Claim 3)
- 10-phrase conservative list. Expand after measuring false positives.
- Pure POSIX shell -- no python3 dependency, no jq dependency
- **Limitation:** Grep-based JSON extraction is fragile. This is intentionally a stopgap until `cerebro stop-guard` replaces it.

**Phase 0 `/implement` skill:**

```yaml
---
name: implement
description: "Structured implementation workflow: context, research, plan (approval gate), execute, verify."
argument-hint: "[task description]"
effort: high
allowed-tools: Read, Write, Edit, Bash, Grep, Glob, Agent
---
```

```markdown
# Implement

Structured workflow for $ARGUMENTS. Follow these phases in order.

## Phase 1: Context
- Read target files and project documentation before proposing changes
- Run /recall if Cerebro is initialized for this project
- Identify relevant tests, configs, and dependencies

## Phase 2: Research
- Verify assumptions via docs, grep, context7, or reading code
- No unverified claims about APIs, configs, or behaviors
- If unsure, research first -- do not guess

## Phase 3: Plan
Present your approach:
- Files to create or modify
- Key design decisions and rationale
- Risks or dependencies

**STOP. Wait for explicit user approval before writing any code.**

## Phase 4: Execute
- Implement the approved plan
- Never push to main/default branch
- Never push without confirming with the user
- Always read a file before editing it

## Phase 5: Verify
- Run tests and linters if applicable
- Review the diff against the approved plan
- Flag any deviations from the plan

## Rules
- Never act on unverified assumptions
- Always read files before editing them
- Never proceed past Phase 3 without explicit user approval
- Never push to main. Always branch.
```

**Key design decisions:**
- `effort: high` frontmatter forces deep reasoning when skill is active (addresses V1)
- `allowed-tools` is explicit but permissive -- Agent is included for subagent delegation
- No references to Jira, swarm agents, or project-specific tooling -- this is a universal skill
- 60 lines, well within the 5K compaction survival budget (fact-check Claim 5)
- Placed at `~/.claude/skills/implement/` for global availability
- **Precedence consideration:** Enterprise > personal > project means this personal skill will override any project-level `/implement` that shares the name. This is the desired behavior for universal enforcement. Projects that need a custom workflow should use a different skill name (e.g., `/build`, `/develop`) or override at enterprise level.

### Phase 1: Cerebro template improvements (code changes)

**Timeline:** After Phase 0 measurement shows value. Estimated effort: 1-2 sessions.

**Trigger:** Phase 0 stop hook fires at least once in 10 sessions, OR /implement skill is used and subjectively improves outcomes. If neither, the interventions aren't providing value and we should investigate why before embedding them in Cerebro.

| # | Action | Files changed | Test coverage |
|---|--------|--------------|---------------|
| 1a | Add `cerebro stop-guard` subcommand | New: `cmd/cerebro/cmd_stop_guard.go`, `cmd_stop_guard_test.go` | 5+ tests: blocks phrases, allows normal stops, handles empty input, handles missing field, phrase category coverage |
| 1b | Add Stop hook to settings.json template (using `cerebro stop-guard`) | Modified: `templates/settings.json` | Extend scaffold tests: verify Stop hook in output, verify merge with existing hooks |
| 1c | Add `/implement` skill template | New: `templates/skill_implement.md`. Modified: `scaffold.go` (+3 lines embed/map) | Extend scaffold tests: skill created, not overwritten without force, force overwrites |
| 1d | Update CHANGELOG.md, README.md | Modified: `CHANGELOG.md`, `README.md` | N/A |

**`cerebro stop-guard` specification:**

```
Usage: cerebro stop-guard [-p project-dir]

Reads hook input JSON from stdin. Extracts `last_assistant_message`.
Checks against a built-in phrase list. Outputs JSON decision to stdout.

Output on match:   {"decision": "block", "reason": "Continue working. Matched: '<phrase>'"}
Output on no match: {}

Exit code: always 0 (uses JSON decision protocol, not exit code blocking).

The phrase list is compiled into the binary and updated with cerebro releases.
Future: cerebro config could allow per-brain phrase customization.
```

**Why a Go subcommand, not inline bash:**
- Follows the existing pattern: `cerebro gc` from SessionEnd, `cerebro recall --prime` from SessionStart, `cerebro stop-guard` from Stop
- Phrase list ships atomically with the binary -- updates are automatic on `brew upgrade`
- Testable with Go unit tests (tech lead proposed 5 test cases, architect agreed)
- Cross-platform -- no grep/sed/python3 behavior differences
- JSON parsing is reliable (Go's `encoding/json`) vs fragile grep-based extraction
- **Estimated implementation:** ~80 lines of Go, ~60 lines of tests

**Settings.json template hook entry:**

```json
"Stop": [
  {
    "matcher": "",
    "hooks": [
      {
        "type": "command",
        "command": "cat | cerebro stop-guard -p \"$CLAUDE_PROJECT_DIR\" 2>/dev/null; true"
      }
    ]
  }
]
```

The `2>/dev/null; true` fallback ensures the hook is a no-op if cerebro is not installed, preventing breakage for users who remove cerebro but keep the settings.json.

**How template merge handles the new event (verified against codebase):**

- `scaffoldSettings(projectDir, false)` on an existing project: `addMissingEvents()` detects `Stop` is absent from existing hooks, adds it automatically. No code change needed.
- `scaffoldSettings(projectDir, true)`: `replaceCerebro()` strips all cerebro entries, then merges all template events including `Stop`. Works correctly.
- New project (`cerebro init`): Template written directly. `Stop` included.

### Phase 2: Read-before-edit monitoring (conditional)

**Trigger:** Phase 0/1 measurement shows edit-without-reading remains a problem (user still encounters files edited without being read first).

**Approach:** Start with monitoring, not enforcement.

| # | Action | Mechanism |
|---|--------|-----------|
| 2a | PostToolUse logging hook | Logs tool name + file path to a session-specific temp file. Enables offline read:edit ratio calculation. |
| 2b | Skill-scoped PreToolUse hook (in `/implement` frontmatter) | Read-before-edit enforcement scoped to the skill's lifecycle. Uses the `hooks` frontmatter field (confirmed, fact-check Claim 2). Only fires when `/implement` is active. |

**Why skill-scoped over global:** A global PreToolUse hook on Write/Edit fires on every edit including trivial ones (fixing a typo, updating a version number). This creates friction without value. Scoping it to `/implement` means enforcement only activates during structured work where the full research-before-action workflow is expected.

**Prediction:** Skill-scoped hooks are the cleanest architecture for this, but no one has published a working implementation of read-state-tracking via skill hooks. The concept is sound (the feature exists in the docs), but there may be practical limitations we'll discover during implementation.

### Phase 3: Evaluator integration (conditional)

**Trigger:** Phases 0-2 demonstrate that assumption-making persists despite stop-guard + /implement + read monitoring.

**Approach:** Agent-type Stop hook with a Sonnet subagent that has read-only tool access.

**Cost consideration:** On subscription plans (Pro/Max), agent hooks consume included usage allocation — there is no incremental dollar cost per invocation. The constraint is rate limits (session budget, token quotas), not per-call billing. On API plans, each invocation costs one Sonnet API call (~$0.02-0.10 per turn). Either way, the primary cost is **latency** (5-15 seconds per turn completion) and **rate limit consumption**, not dollars. Should be opt-in via a `cerebro config` setting: `stop_evaluator = true/false`.

**This phase is explicitly deferred. Not designed in detail here.**

---

## 5. What This Proposal Does NOT Include

These items from the original document are explicitly out of scope. Each has a stated reason and a trigger condition for reconsideration.

| Item | Reason | Reconsider when |
|------|--------|----------------|
| `cerebro harness sync/status/diff` | `cerebro init --force` is sufficient for 5-8 files | Template count exceeds 15 files |
| Template manifest (.cerebro-manifest.json) | Premature for current scale | Users report `--force` clobbering customizations they want to keep |
| Domain packs (--pack terraform/node/python) | Unbounded scope, no evidence of demand | 3+ projects running with core harness and organic domain patterns emerge |
| `cerebro init --agents` | Crosses scope boundary (global vs project) | User explicitly requests global scaffolding |
| `/preflight` skill | CI/git hooks are better enforcement | Pre-commit hooks prove insufficient |
| Paired CLAUDE.md markers | Current implementation works and is tested | A real failure case surfaces (not theoretical fragility) |
| Plugin architecture | Strategic question requiring separate analysis | Cerebro's distribution model is revisited |

---

## 6. Precedence and Placement Matrix

Verified against fact-check-results.md and Claude Code official docs.

### 6.1 Skill placement

| Skill | Location | Precedence tier | Rationale |
|-------|----------|----------------|-----------|
| recall, remember, consolidate | Project `.claude/skills/` | Project (lowest) | Invoke cerebro with `-p "$CLAUDE_PROJECT_DIR"` -- inherently project-specific |
| implement (lightweight, universal) | Personal `~/.claude/skills/` (Phase 0) then project `.claude/skills/` (Phase 1 template) | Personal overrides project | Phase 0: personal for global coverage. Phase 1: project template via `cerebro init` provides per-project baseline. Personal version wins if both exist. |

### 6.2 Hook placement

| Hook | Location | Scope |
|------|----------|-------|
| Cerebro memory lifecycle (SessionStart, UserPromptSubmit, etc.) | Project `.claude/settings.json` | Per-project (different brain per project) |
| Stop hook (stop-guard) | Phase 0: user `~/.claude/settings.json`. Phase 1: project `.claude/settings.json` template | Phase 0: global. Phase 1: per-project (added to all cerebro-initialized projects). Both merge. |

### 6.3 CLAUDE.md placement

| Content | Location | Manager |
|---------|----------|---------|
| Universal behavioral rules | `~/.claude/CLAUDE.md` | User-managed |
| Cerebro Memory System section | Project `CLAUDE.md` | Cerebro-managed (via `cerebro init`) |
| Project-specific instructions | Project `CLAUDE.md` (above or below cerebro section) | User-managed |

---

## 7. Testing Strategy

### 7.1 Unit tests (deterministic)

**For `cerebro stop-guard` (Phase 1a):**

| Test | Input | Expected output |
|------|-------|----------------|
| Blocks "shall I" | `{"last_assistant_message": "Shall I continue?"}` | `{"decision": "block", "reason": "..."}` |
| Blocks "for now" | `{"last_assistant_message": "That should work for now."}` | `{"decision": "block", "reason": "..."}` |
| Allows normal completion | `{"last_assistant_message": "Done. All tests pass."}` | `{}` |
| Handles empty stdin | (empty) | `{}` |
| Handles missing field | `{"session_id": "abc"}` | `{}` |
| Handles `stop_hook_active: true` | `{"stop_hook_active": true, "last_assistant_message": "..."}` | Normal processing (recursive guard if needed) |

**For scaffold changes (Phase 1b-c):**

| Test | Verification |
|------|-------------|
| Stop hook appears in new scaffold | Parse output JSON, verify `Stop` key exists with cerebro command |
| Stop hook added via addMissingEvents | Scaffold over existing cerebro project (no force), verify Stop added |
| Stop hook replaced via force | Force-scaffold, verify Stop hook uses latest command |
| Implement skill created | Verify `.claude/skills/implement/SKILL.md` exists with correct content |
| Implement skill not overwritten | Create custom implement, scaffold without force, verify custom preserved |
| Implement skill force-overwritten | Create custom implement, scaffold with force, verify template wins |

### 7.2 Behavioral measurement (non-deterministic)

These cannot be unit tested. They require deployment + observation.

| Metric | How to measure | Baseline |
|--------|---------------|----------|
| Stop hook violation count | Count `{"decision": "block"}` outputs per session (add logging to the hook) | 0 before deployment |
| User interrupt frequency | Subjective tracking: how often do you correct Claude or restart? | Current experience |
| `/implement` adoption | How often is `/implement` used vs bare prompts? | 0 (not deployed) |
| Read:edit ratio | Post-hoc from JSONL session files (same method as #42796) | Not yet measured for this user |

---

## 8. Risk Assessment

| Risk | Impact | Mitigation |
|------|--------|------------|
| Stop hook false positives block legitimate completions | Medium -- forces Claude to continue when it should stop | Start with conservative 10-phrase list. Log all blocks. Review and tune. Add `stop_hook_active` check to avoid infinite blocking loops. |
| `/implement` skill overhead on trivial tasks | Low -- user must explicitly invoke `/implement` | Skill is opt-in. Use bare prompts for trivial tasks. |
| `effort: high` in skill frontmatter increases cost | Low-medium -- deeper thinking = more tokens | This is the desired behavior. The evidence (V1) shows insufficient thinking is the problem, not excessive thinking. |
| Personal `/implement` overrides project-specific versions | Medium -- enterprise > personal > project means personal always wins | Projects that need custom workflows should use a different skill name. Document this. |
| `cerebro stop-guard` fails silently if cerebro not installed | Low -- hook command includes `2>/dev/null; true` fallback | Graceful degradation to no enforcement. |
| Stop hook doesn't fire in some environments | Unknown -- VSCode issue unconfirmed in docs | Monitor. If confirmed, document CLI-only limitation. |

---

## 9. Open Questions

### 9.1 Answerable with measurement (Phase 0 will resolve)

1. **Does the stop-phrase-guard reduce premature stopping for this user?** The #42796 evidence is from a different user's workflow. Our phrase list may need different tuning.
2. **Does `/implement` with `effort: high` produce better outcomes than bare prompts?** Predicted yes based on V1 evidence, but unproven for this user.
3. **What is the false-positive rate of the 10-phrase list?** Unknown until deployed.

### 9.2 Answerable with investigation

4. **Should Cerebro become a Claude Code plugin?** The plugin system packages skills, hooks, subagents, and MCP servers into installable units -- structurally what `cerebro init` does. If Cerebro shipped as a plugin, distribution, versioning, and discovery come for free. This deserves a separate analysis. (Architect raised this; Tech Lead called it scope creep. Both are right -- it's important but doesn't belong in THIS proposal.)
5. **Can skill-scoped hooks track read-state across tool calls?** The `hooks` frontmatter is confirmed, but no one has published a working implementation of cross-turn state tracking via skill hooks. This is the Phase 2 uncertainty.

### 9.3 Unanswerable (depends on Anthropic)

6. **Will Anthropic change the hooks API?** Hooks are the foundation of this entire proposal. If Anthropic restricts or removes hooks, all mitigations collapse.
7. **Will future models fix the zero-thinking-tokens bug?** If they do, the `/implement` skill's `effort: high` becomes less critical. The stop-guard remains valuable for behavioral drift regardless.
8. **Will `~/.claude/skills/` precedence change?** The current enterprise > personal > project order may change. Our placement strategy would need revision.

---

## 10. Relationship to the Existing Research Documents

| Document | Status | Role going forward |
|----------|--------|-------------------|
| `approach.md` | Unchanged | The investigation's source of truth. Root cause taxonomy, evidence analysis, success criteria. |
| `evidence-catalog.md` | Unchanged | Timestamped evidence. Update as new data emerges (Marginlab tracker, new issues). |
| `settings-matrix.md` | Unchanged | Settings baseline for Phase 0a deployment. Reference for future settings changes. |
| `session-summary.md` | Unchanged | Research methodology and agent handoff context. |
| `harness-management-research.md` | **Superseded by this document.** | Retain as historical artifact. Do not update. |
| `architect-harness-proposal.md` | Input to this document | Independent review. Key contributions: root cause coverage analysis, skill frontmatter discovery, corrected skill portability claim. |
| `techlead-harness-proposal.md` | Input to this document | Independent review. Key contributions: `cerebro stop-guard` subcommand design, settings baseline gap, maintenance burden analysis, YAGNI discipline. |
| `architect-reviews-techlead.md` | Input to this document | Cross-review. |
| `techlead-reviews-architect.md` | Input to this document | Cross-review. |
| `fact-check-results.md` | Input to this document | Verified facts against primary documentation. |

---

## 11. Decision Summary

| Phase | What | Cerebro code? | Trigger |
|-------|------|--------------|---------|
| **0** | Settings baseline + Stop hook (inline) + behavioral rules + /implement skill | No | **Now** |
| **1** | `cerebro stop-guard` subcommand + Stop hook in template + /implement template + scaffold tests | Yes | Phase 0 shows value |
| **2** | Read-before-edit monitoring/enforcement (skill-scoped hooks) | Yes (templates) | Edit-without-reading persists |
| **3** | Agent-type evaluator Stop hook (opt-in) | Yes (templates + config) | Assumption-making persists |

**Total new CLI commands:** 1 (`cerebro stop-guard`)
**Total new Go files:** 2 (`cmd_stop_guard.go`, `cmd_stop_guard_test.go`)
**Total new templates:** 1 (`skill_implement.md`)
**Total modified templates:** 1 (`settings.json` -- add Stop hook)
**Total modified Go files:** 1 (`scaffold.go` -- add embed + map entry)

This is approximately 60% less scope than the original document's 5-phase proposal with harness management commands, while addressing the same root cause vectors with evidence-backed interventions.
