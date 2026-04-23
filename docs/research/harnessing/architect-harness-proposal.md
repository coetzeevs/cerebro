# Architect's Harness Proposal: Extending Cerebro into a Claude Code Harness Manager

**Date:** 2026-04-22
**Status:** Proposal (independent rewrite of `harness-management-research.md`)
**Author role:** Principal Architect (independent review)

---

## 0. Purpose and Method

This document is a ground-up rewrite of `harness-management-research.md`, which was drafted **before** the evidence catalog, settings matrix, approach document, and session summary existed. That original document made reasonable proposals, but many were based on intuition rather than evidence. This rewrite challenges every assumption against the actual evidence base, the current Cerebro codebase, and verified Claude Code documentation.

**Evidence sources used:**
- `approach.md` v1.2 — root cause taxonomy (6 vectors), mitigation tiers, workstream status
- `evidence-catalog.md` — 25+ timeline entries, 10 GitHub issues, Boris Cherny HN confirmation
- `settings-matrix.md` — 30+ quality-relevant settings, deployable baseline
- `harness-management-research.md` — the original proposal being reviewed
- Cerebro codebase: `cmd/cerebro/scaffold.go`, `cmd/cerebro/cmd_init.go`, `cmd/cerebro/templates/`
- Claude Code official documentation: [hooks reference](https://code.claude.com/docs/en/hooks), [skills](https://code.claude.com/docs/en/skills), [settings](https://code.claude.com/docs/en/settings), [best practices](https://code.claude.com/docs/en/best-practices)
- Community tools: [gstack](https://github.com/garrytan/gstack) (66K+ stars), [claude-code-harness](https://github.com/Chachamaru127/claude-code-harness), [everything-claude-code](https://github.com/affaan-m/everything-claude-code)
- Anthropic engineering: [Harness Design for Long-Running Applications](https://www.anthropic.com/engineering/harness-design-long-running-apps)
- User's live harness: `~/.claude/`, `/Users/q/projects/.claude/`, agents, skills, settings

**Method:** Every claim and proposal cites its evidence. Where evidence conflicts, both sides are presented. Where evidence is absent, that gap is stated explicitly.

---

## 1. Challenge: Does "Brain + Harness Manager" Hold Up?

The original document proposes extending Cerebro from a memory system into a "two-faceted tool: brain (memory) + harness (Claude Code configuration management)." Before accepting this framing, it must be tested against the evidence.

### 1.1 Evidence supporting the framing

1. **Cerebro already does harness work.** The `cerebro init` command scaffolds `.claude/settings.json` (hooks), `.claude/skills/` (recall, remember, consolidate), and `CLAUDE.md` (behavioral instructions). This is not memory management -- it is configuration scaffolding. The code is in `scaffold.go` (298 lines) and `cmd_init.go` (165 lines), using `//go:embed` templates.

2. **`cerebro init --force` is already a sync mechanism.** It replaces cerebro hooks in settings.json while preserving user hooks (`replaceCerebro()` in scaffold.go lines 106-145), updates skills, and replaces the CLAUDE.md cerebro section while preserving trailing sections. This is rudimentary template management already shipping.

3. **Skills invoke cerebro commands.** The recall, remember, and consolidate skills call `cerebro recall`, `cerebro add`, `cerebro search`, etc. Shipping skills from the same binary guarantees version compatibility between the skill's expected CLI interface and the binary's actual interface.

4. **The harness work is small.** The entire template set is 5 files totaling under 15KB. Even with proposed additions, it would remain under 100KB -- trivially embeddable via `//go:embed`.

### 1.2 Evidence against the framing (scope creep risk)

1. **The degradation problem is upstream, not downstream.** The 6 root cause vectors in `approach.md` Section 4 are: thinking depth changes, prompt caching bugs, context window degradation, load-sensitive allocation, behavioral drift across versions, and systematic lever removal. None of these are problems that a harness *manager* solves. They are problems that a *harness* (hooks, skills, settings, CLAUDE.md) partially mitigates, and a harness manager merely distributes the harness.

2. **The "portability" problem may be overstated.** The original document claims quality "does not transfer" to other projects. But the official Claude Code docs now confirm personal skills live in `~/.claude/skills/` and are available across all projects ([skills docs](https://code.claude.com/docs/en/skills), "Where skills live" table: Personal = `~/.claude/skills/<skill-name>/SKILL.md`, applies to "All your projects"). **This directly contradicts** the original document's claim that "Skills are project-scoped only -- no global mechanism exists" (Section 1, portability table). The original document's entire portability argument was built on a false premise.

3. **The real portability gap is narrower than claimed.** With the corrected understanding:

   | Layer | Portable today? | Original claim |
   |-------|----------------|----------------|
   | Rules (CLAUDE.md) | Yes -- `~/.claude/CLAUDE.md` is global | "Partially" -- Correct |
   | Hooks | Yes -- `~/.claude/settings.json` hooks merge with project hooks ([settings docs](https://code.claude.com/docs/en/settings)) | "Yes" -- Correct |
   | Skills | **Yes -- `~/.claude/skills/` is personal/global** | **"No" -- Incorrect** |
   | Agents | Yes -- `~/.claude/agents/` is global | "Yes" -- Correct |
   | Memory | Yes -- Cerebro per-project | "Yes" -- Correct |
   | Permissions | Partially -- user-level `~/.claude/settings.json` permissions merge with project | **"No" -- Partially incorrect** |

   The only truly non-portable layer is domain-specific configuration (Terraform deny-lists, project-specific CLAUDE.md content). This is inherently non-portable because it is domain-specific.

### 1.3 Verdict

The "brain + harness manager" framing is **partially justified** for Tiers 1-2 (core scaffolding and template updates) because Cerebro already does this work and the marginal cost of improving it is low. However, the original document's justification was based on an incorrect claim about skill portability. The actual value proposition is narrower:

- **Justified:** Improving `cerebro init` to scaffold better defaults (behavioral rules, an implement skill, an evaluator agent) and keeping them updated via `cerebro init --force`.
- **Not justified (yet):** A full template management system with manifests, checksums, three-way diffs, and `harness sync/status/diff` commands. The portability problem that motivated this system is smaller than claimed.

---

## 2. Root Cause Coverage Analysis

The approach document identifies 6 root cause vectors. For each, I assess whether a harness can address it, and if so, how.

### Vector 1: Thinking Depth / Effort Level Changes

**Can a harness address this?** Partially.

- **Settings:** `effortLevel: "high"` in `~/.claude/settings.json` persists across sessions. Already deployed in the user's global settings. Evidence from `settings-matrix.md` Section 1: "effort=high is necessary but not sufficient" because Boris Cherny confirmed zero-thinking turns occur even at effort=high (evidence-catalog.md, Boris Cherny HN thread, item 47668520).
- **Hooks cannot fix this.** No hook can force the model to think more deeply. A Stop hook can catch the *consequences* (premature stopping) but not the cause (zero reasoning tokens). A PreToolUse hook can catch edit-without-reading but cannot force the model to reason before deciding to edit.
- **CLAUDE.md cannot fix this.** Advisory instructions are only followed if the model allocates enough reasoning tokens to process them. On zero-thinking turns, CLAUDE.md is irrelevant.
- **What a harness CAN do:** Deploy optimal settings (`effortLevel`, `showThinkingSummaries`), deploy hooks that catch downstream symptoms, and deploy monitoring that detects when upstream changes alter thinking depth.

**Harness contribution: Low-medium.** Settings are necessary but not sufficient. Hooks catch symptoms, not causes.

### Vector 2: Prompt Caching Bugs

**Can a harness address this?** No.

This is a Claude Code runtime bug (issues #40524, #38335). The fix is in Claude Code version management (v2.1.90+ has partial fixes). A harness cannot influence prompt caching behavior.

**Harness contribution: None.**

### Vector 3: Context Window Degradation

**Can a harness address this?** Partially.

- **Settings:** `CLAUDE_AUTOCOMPACT_PCT_OVERRIDE` and `CLAUDE_CODE_AUTO_COMPACT_WINDOW` can be deployed via the `env` key in settings.json. These are evidence-backed (settings-matrix.md Section 2; community reports document degradation at 20% of 1M window).
- **Hooks:** Pre/PostCompact hooks can log compaction events (Cerebro already does this in `settings.json` template). A Stop hook with exit 2 can block premature stopping that occurs when the model gets "context anxiety" (Anthropic's own term from the harness design article: "Models wrap up work prematurely as context fills").
- **Skills:** The `/implement` skill's phased approach (context loading, then research, then plan, then execute) structures work into shorter focused segments, reducing context accumulation.
- **CLAUDE.md:** Post-compaction recovery instructions (already in Cerebro's template) help Claude restore critical context after compaction.

**Harness contribution: Medium.** Settings + structured workflows + compaction hooks are meaningful mitigations.

### Vector 4: Load-Sensitive Resource Allocation

**Can a harness address this?** No.

Evidence is correlational rather than causal (approach.md Section 4, Vector 4). Anthropic denies time-of-day/load-based quality reduction (September 2025 postmortem). A harness operates on the user's machine and cannot influence server-side resource allocation.

**Harness contribution: None.**

### Vector 5: Behavioral Drift Across Model Updates

**Can a harness address this?** Yes -- this is where hooks are most valuable.

- **Stop hook (exit 2):** Blocks premature stopping regardless of model version. The #42796 stop-phrase-guard went from 0 violations to 173 in 17 days (evidence-catalog.md timeline, Mar 8-25). This is the most proven hook-based mitigation in the evidence base.
- **PreToolUse hooks:** Block destructive commands, enforce read-before-edit patterns. These are behavioral constraints that survive model transitions because they operate outside the model's control plane (approach.md Section 4, Vector 6: "Hooks operate outside the model's control plane and survive model transitions").
- **Skills as workflow enforcement:** A structured `/implement` skill with an explicit approval gate (Phase 3: "STOP. Wait for explicit user approval") prevents the "simplest fix mentality" documented in the problem statement (approach.md Section 2) regardless of model version.
- **Agent-based review:** The evaluator agent pattern (from Anthropic's [harness design article](https://www.anthropic.com/engineering/harness-design-long-running-apps): "When asked to evaluate work they've produced, agents tend to respond by confidently praising the work") provides generator/evaluator separation that catches behavioral drift.

**Harness contribution: High.** This is the primary justification for building a harness.

### Vector 6: Systematic Removal of User Control Levers

**Can a harness address this?** Indirectly.

Hooks and skills are the most durable mitigation precisely because each model release deprecates settings-based levers (`budget_tokens` deprecated on 4.6+, `CLAUDE_CODE_DISABLE_ADAPTIVE_THINKING` unsupported on 4.7 -- evidence-catalog.md, Patterns 2 and 5). External enforcement via hooks survives lever removal. However, hooks themselves depend on Claude Code's hook system, which could theoretically be changed or restricted.

**Harness contribution: Medium-high.** Hooks are the current best hedge against lever removal.

### Summary

| Vector | Harness contribution | Primary mechanism |
|--------|---------------------|-------------------|
| V1: Thinking depth | Low-medium | Settings (necessary, not sufficient) |
| V2: Caching bugs | None | N/A |
| V3: Context degradation | Medium | Settings + compaction hooks + structured workflows |
| V4: Load allocation | None | N/A |
| V5: Behavioral drift | **High** | Stop hooks + PreToolUse hooks + skills + evaluator agent |
| V6: Lever removal | Medium-high | Hooks as durable external enforcement |

**A harness meaningfully addresses 3 of 6 vectors (V3, V5, V6), partially addresses 1 (V1), and cannot address 2 (V2, V4).** The original document did not perform this analysis, which led it to overstate what a harness can deliver.

---

## 3. What Cerebro Already Provides vs What Needs Building

### 3.1 Currently shipping (verified against codebase)

| Capability | Implementation | Files |
|-----------|---------------|-------|
| Brain init + schema management | `brain.Init()`, auto-migration | `brain/brain.go`, `internal/store/schema.go` |
| Memory CRUD | add, get, update, list, search, supersede, archive | `cmd/cerebro/cmd_*.go`, `brain/brain.go` |
| Vector search + composite scoring | sqlite-vec, MMR diversification | `internal/store/search.go`, `cmd/cerebro/cmd_prime_mmr.go` |
| Knowledge graph | edges, relations, graph expansion | `internal/store/edges.go` |
| Lifecycle management | GC, recall --prime, consolidate, promote | `cmd/cerebro/cmd_gc.go`, `cmd_recall.go` |
| Export/import | json, sql, sqlite formats | `internal/store/export.go`, `cmd/cerebro/cmd_export.go` |
| **Scaffold: settings.json** | Hook merge, cerebro-aware replace, addMissingEvents | `cmd/cerebro/scaffold.go` lines 45-197 |
| **Scaffold: skills** | recall, remember, consolidate (project `.claude/skills/`) | `scaffold.go` lines 199-230, `templates/skill_*.md` |
| **Scaffold: CLAUDE.md** | Cerebro Memory System section with marker-based replace | `scaffold.go` lines 232-297, `templates/claudemd_section.md` |
| **`--force` flag** | Replaces cerebro hooks, skills, and CLAUDE.md section | `cmd_init.go` |
| Config management | Per-brain config (embed provider, model, dims) | `cmd/cerebro/config.go` |

### 3.2 What the original document proposes to build

| Proposed feature | Original phase | Assessment |
|-----------------|---------------|------------|
| Behavioral rules in CLAUDE.md | Phase 1 | **Build.** Low cost, evidence-backed. |
| Evaluator agent | Phase 1 | **Build.** Evidence-backed (Anthropic harness article). |
| Lightweight `/implement` skill | Phase 2 | **Build.** The user's existing 280-line `/implement` proves the pattern works. A lightweight version is justified. |
| `/preflight` skill | Phase 2 | **Defer.** Never specified. The `/implement` skill's Phase 5 (verify) covers this. |
| `--agents` flag for `cerebro init` | Phase 2 | **Reconsider.** Agents go in `~/.claude/agents/` (global). Cerebro scaffolds project-level files. Global agent scaffolding is a different concern. |
| Ownership markers (`<!-- managed-by: cerebro -->`) | Phase 2 | **Defer.** Premature complexity. The existing `"cerebro"` string-match in `scaffoldSettings()` and `## Cerebro Memory System` marker in `scaffoldCLAUDEMD()` already serve this purpose. |
| Version metadata in files | Phase 2 | **Defer.** Adds maintenance burden without evidence of need. |
| `.cerebro-manifest.json` (checksums) | Phase 2 | **Defer.** The portability problem that motivated this is smaller than claimed (Section 1.2 above). |
| `cerebro harness status` | Phase 3 | **Defer.** No evidence of demand. |
| `cerebro harness sync` | Phase 3 | **Defer.** `cerebro init --force` already does this. |
| `cerebro harness diff` | Phase 3 | **Defer.** No evidence of demand. Three-way diff is engineering for engineering's sake without a demonstrated user need. |
| Domain packs (`--pack terraform`) | Phase 4 | **Correctly deferred** in original. Agree. |
| Stop hook (agent-type) | Phase 5 | **Reprioritize.** The evidence strongly supports a command-type Stop hook NOW, not deferred. See Section 4. |

### 3.3 What the original document misses

| Gap | Evidence | Recommendation |
|-----|----------|---------------|
| Personal skills (`~/.claude/skills/`) now exist | [Skills docs](https://code.claude.com/docs/en/skills): "Personal = `~/.claude/skills/<skill-name>/SKILL.md`" | This changes the scaffolding strategy. Universal skills (implement, preflight) should scaffold to `~/.claude/skills/`, not project `.claude/skills/`. |
| Skills have `effort` and `model` frontmatter | [Skills docs](https://code.claude.com/docs/en/skills), frontmatter reference | Skills can override effort level per-skill. An `/implement` skill can set `effort: high` to ensure deep reasoning during implementation. |
| Skills have `hooks` frontmatter | [Skills docs](https://code.claude.com/docs/en/skills): "Hooks scoped to this skill's lifecycle" | Skills can carry their own hooks. This means a skill can enforce read-before-edit only while it is active, without polluting global hooks. |
| Skills have `context: fork` for subagent execution | [Skills docs](https://code.claude.com/docs/en/skills): "Set to `fork` to run in a forked subagent context" | The evaluator pattern can be implemented as a skill with `context: fork` rather than requiring a global agent. |
| 24 hook event types (not 6) | [Hooks reference](https://code.claude.com/docs/en/hooks) | The original document only discusses SessionStart, Stop, PreToolUse, PostToolUse, UserPromptSubmit, and SessionEnd. Claude Code now supports 24 events including SubagentStart/Stop, TaskCreated/Completed, FileChanged, InstructionsLoaded, and others. |
| Exit code 2 (not 1) blocks actions | [Hooks reference](https://code.claude.com/docs/en/hooks): "Exit 2 - Blocking Error" | The original document references exit code 1 for blocking. This is incorrect. Exit 1 is a non-blocking error. Exit 2 is required to block. This is critical for Stop hooks. |
| Skills survive compaction (first 5K tokens, 25K budget) | [Skills docs](https://code.claude.com/docs/en/skills): "re-attaches the most recent invocation of each skill after the summary, keeping the first 5,000 tokens" | Skills are more durable across compaction than CLAUDE.md content. This makes skills a better vehicle for behavioral rules than CLAUDE.md for long sessions. |
| Prompt-type hooks limitation is documented differently than claimed | [Hooks reference](https://code.claude.com/docs/en/hooks) shows prompt hooks receive a prompt string, not conversation content | The original document cites issues #11610 and #11786 for prompt Stop hook regressions. The official docs don't list these as known issues. The rejection of prompt-type Stop hooks is still correct (self-evaluation degeneracy from Anthropic's article), but the specific regression claim needs verification. |

---

## 4. Revised Proposal: Minimum Viable Harness

Based on the evidence analysis, here is a revised, evidence-grounded proposal. It is smaller than the original because the portability problem is smaller than claimed, and it reprioritizes based on which root cause vectors a harness can actually address.

### 4.1 Phase 0: Immediate improvements (zero Cerebro changes)

**Justification:** The approach document's own principle: "Start with Tier 1 and Tier 2. Only build a full harness if simpler interventions prove insufficient" (approach.md Section 7, Risk: Over-engineering the harness).

| Action | Location | Evidence basis | Effort |
|--------|----------|---------------|--------|
| Add behavioral rules to CLAUDE.md | `~/.claude/CLAUDE.md` | User's current 6-line file is below the best-practices guidance. Rules address V5 (behavioral drift). | 10 min |
| Create evaluator agent | `~/.claude/agents/evaluator.md` | Anthropic harness article: "Separating generator from evaluator is the key lever." | 15 min |
| Create lightweight `/implement` skill | `~/.claude/skills/implement/SKILL.md` | User's 280-line version proves the pattern. Lightweight version at `~/.claude/skills/` is globally available. | 30 min |
| Deploy Stop hook (command-type) | `~/.claude/settings.json` | #42796: 0 violations to 173 in 17 days. Most proven mitigation in the evidence base. Uses exit 2 per official docs. | 30 min |
| Measure baseline | Run 5-10 tasks across non-EDP projects | approach.md success criteria: "measurable improvement in autonomous task completion rate" | 2-4 hours |

**Why the Stop hook is Phase 0, not deferred:** The original document defers the Stop hook to Phase 5, citing "Phase 1+2 demonstrably fail to prevent assumption-making." This contradicts the evidence. The stop-phrase-guard is the single most quantitatively validated mitigation in the entire evidence base (approach.md Section 3.1: "0 violations (entire history before March 8) to 173 violations in 17 days"). It is also the simplest to implement (a command hook with pattern matching, exit 2 to block). Deferring the most proven mitigation while building unproven template management systems is backwards.

**Why command-type, not agent-type:** Agent-type Stop hooks add 5-15 seconds latency per turn and are marked "experimental" in the official docs. A command-type hook with phrase matching is deterministic, sub-10ms (per Chachamaru127's Go engine benchmarks), and zero-cost. The evidence from #42796 is based on a command-type approach (`stop-phrase-guard.sh`). Start with what is proven.

**Stop hook specification:**

```json
{
  "hooks": {
    "Stop": [
      {
        "matcher": "",
        "hooks": [
          {
            "type": "command",
            "command": "INPUT=$(cat); MSG=$(echo \"$INPUT\" | grep -oiE '(shall I|should I|want me to|would you like|I can stop|let me know if|feel free to|I\\'ll leave|as an exercise|beyond the scope|out of scope|for now|good enough|that covers|wrapping up|I\\'ll stop here|that should be|future enhancement|TODO for later|left as)' | head -1); if [ -n \"$MSG\" ]; then echo \"Stop hook: blocked premature stopping ('$MSG'). Continue working.\" >&2; exit 2; fi; exit 0"
          }
        ]
      }
    ]
  }
}
```

**Note on phrase selection:** The phrases above are a starting set derived from the #42796 behavioral catalog (approach.md Section 3.1, Appendix A reference) and the `claudefa.st` [stop hook patterns](https://claudefa.st/blog/tools/hooks/stop-hook-task-enforcement). They should be tuned based on the user's actual false-positive rate. The stellaraccident implementation matched 30+ phrases across 5 categories (permission-seeking, ownership-dodging, premature stopping, scope reduction, future-work deferral).

### 4.2 Phase 1: Cerebro template improvements

**Justification:** Cerebro already scaffolds harness files. Improving the templates is incremental work that ships better defaults to every `cerebro init` user.

| Action | Detail | Evidence basis |
|--------|--------|---------------|
| Add behavioral rules section to CLAUDE.md template | 4-6 rules addressing V5 behavioral drift. Use paired markers for section management. | approach.md Section 2 problem statement (edit-without-reading, assumption-making, convention drift) |
| Add `/implement` skill template | Lightweight 60-80 line universal workflow. Scaffold to project `.claude/skills/`. | User's heavy version proves the pattern. Skills at project level override personal skills of the same name (skills docs precedence). |
| Add `effort: high` to `/implement` skill frontmatter | Uses the `effort` frontmatter field | Skills docs: "Effort level when this skill is active. Overrides the session effort level." Addresses V1. |
| Extend settings.json template with Stop hook | Command-type stop-phrase-guard | Phase 0 validates the pattern; Phase 1 embeds it in the template for new projects. |
| Update `scaffoldCLAUDEMD` to use paired markers | `<!-- cerebro:memory-system:start -->` / `<!-- cerebro:memory-system:end -->` | Original document Section 5.4. Agree this is a good improvement. The current approach (find marker, assume everything after is cerebro) is fragile. |
| Test all templates | `scaffold_test.go` already has 19KB of tests | Follow existing TDD pattern. |

**What this phase explicitly does NOT build:**
- No `--agents` flag (agents are global, not per-project; Cerebro scaffolds per-project)
- No `.cerebro-manifest.json` (no evidence of need)
- No ownership markers beyond what already exists
- No version metadata in files

### 4.3 Phase 2: Read-before-edit hook (conditional)

**Trigger condition:** Phase 0 measurement shows the read:edit ratio problem persists despite the Stop hook and /implement skill.

**Justification:** The read:edit ratio collapse (6.6 to 2.0 in #42796 data) is the second most quantified degradation signal. However, implementing a read-before-edit enforcer requires tracking state across tool calls within a session, which is significantly more complex than a stateless phrase-matching hook.

**Implementation options (in order of preference):**

1. **PreToolUse hook with file tracking.** A command hook on `Write|Edit` that reads stdin for the `tool_input.file_path`, checks whether that file was recently read (via a log file written by a PostToolUse hook on `Read`), and exits 2 if not. This requires two hooks working in concert and a temp file for state.

2. **Skill-scoped hooks.** The `/implement` skill can carry its own `hooks` frontmatter that enforces read-before-edit only while the skill is active. This is cleaner than a global hook that fires on every edit, including trivial ones.

3. **PostToolUse logging only.** A PostToolUse hook that logs all tool calls to a file, enabling offline read:edit ratio analysis without runtime blocking. Lower risk, still provides the detection signal.

**Recommended approach:** Option 3 first (monitoring), then Option 2 (enforcement within /implement) if monitoring shows the problem persists.

### 4.4 Phase 3: Evaluator agent integration (conditional)

**Trigger condition:** Phase 0 measurement shows assumption-making persists despite behavioral rules, Stop hook, and /implement skill.

**Justification:** Anthropic's harness design article provides the strongest evidence for generator/evaluator separation. However, the article also warns: "Every component in a harness encodes an assumption about what the model can't do on its own, and those assumptions are worth stress testing. Opus 4.6 eliminated the need for sprint decomposition that Sonnet 4.5 required."

**Implementation:** An agent-type Stop hook that spawns a Sonnet subagent with read-only tool access (Read, Grep, Glob) to check modified files for unverified assumptions. Only triggers on significant changes (not trivial edits).

**Cost consideration:** Each invocation costs one Sonnet API call. At current pricing, this adds approximately $0.02-0.10 per turn depending on context size. For a typical 50-turn session, that is $1-5 additional cost. This must be opt-in.

### 4.5 What should NOT be built

The following items from the original document are explicitly rejected or indefinitely deferred based on evidence analysis:

| Item | Reason for rejection | Evidence |
|------|---------------------|----------|
| `cerebro harness sync` command | `cerebro init --force` already serves this purpose. Adding a separate `sync` command with different semantics adds cognitive load without demonstrated benefit. | Codebase: `scaffold.go` already implements force-replace semantics. |
| `cerebro harness status` command | No evidence of user demand. The user can check template versions by reading the files. | No evidence found in any research document or community tool. |
| `cerebro harness diff` (three-way) | Engineering for engineering's sake. The `create-react-app` eject analogy does not apply because CRA's templates are hundreds of files; Cerebro's are 5-8. | Template count: 5 files totaling <15KB (codebase). |
| `.cerebro-manifest.json` checksums | Premature optimization for a problem that does not yet exist at scale. The existing string-match detection (`strings.Contains(string(existingData), "cerebro")` in scaffold.go line 79) is sufficient for 5-8 files. | Codebase: scaffold.go already handles conflict detection. |
| Domain packs (`--pack terraform/node/python`) | Correctly deferred in original. Agree. The number of domain packs is unbounded, and each encodes domain assumptions that require ongoing maintenance. | Original document Section 7.2: "Tier 3 is where scope creep risk is real." |
| `/preflight` skill | Never specified in the original document beyond a name. The `/implement` skill's Phase 5 (verify) covers this use case. A standalone preflight adds a skill to remember without adding capability. | Original document Section 7.5: "Listed in Tier 1 but never described." |

---

## 5. Compatibility Matrix

Verified against official Claude Code documentation as of April 2026.

### 5.1 Hook compatibility

| Hook event | Cerebro uses today | Proposal adds | Blocks? | Verified source |
|-----------|-------------------|---------------|---------|-----------------|
| SessionStart (startup, resume, compact, clear) | Yes -- memory priming | No change | No (observational) | [Hooks reference](https://code.claude.com/docs/en/hooks) |
| UserPromptSubmit | Yes -- fallback priming | No change | Yes (exit 2) | Hooks reference |
| PreCompact | Yes -- logging | No change | Yes (exit 2) | Hooks reference |
| PostCompact | Yes -- sentinel clear | No change | No (post-event) | Hooks reference |
| SessionEnd | Yes -- GC | No change | No (observational) | Hooks reference |
| **Stop** | **No** | **Yes -- stop-phrase-guard** | **Yes (exit 2)** | Hooks reference: "Stop: Prevents Claude stopping" |
| **PreToolUse (Write\|Edit)** | **No** | **Phase 2: read-before-edit** | **Yes (exit 2)** | Hooks reference: "PreToolUse: Blocks tool call" |
| **PostToolUse** | **No** | **Phase 2: logging** | **No (post-event)** | Hooks reference |

### 5.2 Settings merge behavior

| Scope | Merge behavior | Cerebro impact |
|-------|---------------|----------------|
| User (`~/.claude/settings.json`) | Lowest priority for scalars; arrays merge | Stop hook here applies globally |
| Project (`.claude/settings.json`) | Mid priority; arrays merge | Cerebro hooks here; project Stop hook would merge with user Stop hook |
| Local (`.claude/settings.local.json`) | Higher than project; arrays merge | Personal overrides; **known bug**: can replace (not merge) permission arrays |

**Risk:** The `settings.local.json` permission replacement bug (documented in original document Section 7.3) means project-level deny-lists could be clobbered by a local settings file. This is a Claude Code bug, not a Cerebro problem, but Cerebro's documentation should warn about it.

### 5.3 Skill placement

| Skill | Recommended location | Rationale |
|-------|---------------------|-----------|
| recall, remember, consolidate | Project `.claude/skills/` | **Keep as-is.** These invoke `cerebro` with `-p "$CLAUDE_PROJECT_DIR"` and are project-specific (different brain per project). |
| implement (lightweight) | Project `.claude/skills/` (via `cerebro init`) | Project-level so it can be overridden by heavy versions (user's 280-line version). Personal `~/.claude/skills/implement/` serves as fallback if no project skill exists. |
| implement (user's heavy version) | Project `.claude/skills/` (user-managed, not cerebro-managed) | Cerebro should never touch this. The user's version overrides the template. |

**Precedence order** (from skills docs): Enterprise > Personal > Project. Wait -- this means personal skills **override** project skills, which is the opposite of what we want. If the user has a lightweight `/implement` in `~/.claude/skills/` and a heavy one in `.claude/skills/`, the personal one wins.

**CORRECTION:** Re-reading the skills docs: "When skills share the same name across levels, higher-priority locations win: enterprise > personal > project." This means a personal skill takes precedence over a project skill. This is the opposite of the layering model assumed by the original document (which assumed project-local overrides global templates).

**Impact:** This means the lightweight `/implement` should be scaffolded at the **project** level by `cerebro init`, and the user's heavy version should be at the **project** level too (replacing the lightweight one). A personal `/implement` in `~/.claude/skills/` would be a bad idea because it would override every project's custom version.

**Revised placement strategy:**
- **Cerebro-managed skills (recall, remember, consolidate, implement):** Scaffold to project `.claude/skills/` via `cerebro init`. This is what Cerebro already does.
- **Global behavioral tools (evaluator agent):** Place in `~/.claude/agents/`. This is what the original document recommends and is correct.
- **Personal skills for non-project-specific workflows:** User manages `~/.claude/skills/` directly. Cerebro does not touch this directory.

### 5.4 CLAUDE.md layering

| Level | Cerebro role | Content |
|-------|-------------|---------|
| `~/.claude/CLAUDE.md` | User-managed (Cerebro does NOT scaffold here) | Terse global rules (currently 6 lines) |
| `~/projects/CLAUDE.md` | Cerebro-managed (Memory System section) | Memory system instructions |
| `~/projects/agentic/CLAUDE.md` | Cerebro-managed (Memory System section) | Memory system instructions (duplicated at this level) |
| `~/projects/agentic/cerebro/CLAUDE.md` | Cerebro-managed (Memory System section) + user-managed (project docs) | Memory system + project-specific instructions |

The behavioral rules from Phase 0 go in `~/.claude/CLAUDE.md` (user-managed, global). Cerebro should NOT scaffold behavioral rules into the CLAUDE.md template because:
1. They are universal, not project-specific
2. `~/.claude/CLAUDE.md` is user-managed (per CLAUDE.md instructions: "user's private global instructions")
3. Adding them to every project's CLAUDE.md would duplicate them across every cerebro-initialized project

However, Cerebro CAN add behavioral rules to the CLAUDE.md **template** for projects that lack a global CLAUDE.md. This is a reasonable default.

---

## 6. Implementation Specification

### 6.1 Phase 0 deliverables (manual, no code changes)

**File: `~/.claude/CLAUDE.md`** (append to existing 6 lines):

```markdown
## Behavioral Rules
- Never act on unverified assumptions. Research via context7, grep, or docs before acting.
- Always read files before editing them. Never propose changes to code you haven't read.
- Never stop early, ask permission to continue, or defer work to "future enhancements."
- Prefer the simplest approach that works. Don't add abstractions beyond what the task requires.
```

**File: `~/.claude/agents/evaluator.md`:**

```markdown
---
name: evaluator
description: Skeptical code reviewer that finds problems, not confirms correctness
model: sonnet
tools: Read, Grep, Glob
---
You are a skeptical reviewer. Your job is to find what is WRONG, not confirm what is right.

Check for:
1. Unverified assumptions -- were APIs, configs, or behaviors confirmed against docs or the actual codebase, or just assumed?
2. Files edited without being read first -- check git diff against recent read operations
3. Edge cases -- error paths, empty inputs, concurrent access, not just the happy path
4. Missing verification -- claims about "how X works" that were not confirmed via context7, grep, or reading code

Be specific. Point to exact files and lines. Explain what could go wrong.
Do NOT praise the work. Focus exclusively on problems and risks.
```

**File: `~/.claude/skills/implement/SKILL.md`** (personal global fallback):

```yaml
---
name: implement
description: "Structured implementation workflow: load context, research, plan (with approval gate), execute, verify. Use for any non-trivial code change."
argument-hint: "[task description or ticket key]"
effort: high
disable-model-invocation: true
---
```

```markdown
# Implement

Structured workflow for $ARGUMENTS. Follow these phases in order.

## Phase 1: Context
- Read target files and project documentation before proposing changes
- Run /recall if Cerebro is initialized
- Check for relevant tests, configs, and dependencies

## Phase 2: Research
- Verify assumptions via docs, grep, context7, or reading code
- No unverified claims about APIs, configs, or behaviors
- If unsure about something, research it -- do not guess

## Phase 3: Plan
Present your approach:
- What files will be created or modified
- Key design decisions and their rationale
- Any risks or dependencies

**STOP. Wait for explicit user approval before writing any code.**

## Phase 4: Execute
- Implement with atomic commits
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

**File: `~/.claude/settings.json`** (add Stop hook to existing hooks object, or merge if no hooks exist):

The Stop hook specified in Section 4.1 above.

### 6.2 Phase 1 deliverables (Cerebro code changes)

1. **New template:** `cmd/cerebro/templates/skill_implement.md` -- lightweight /implement skill (as above, adapted for project-level scaffolding with `cerebro` CLI references).

2. **New template:** `cmd/cerebro/templates/claudemd_behavioral.md` -- behavioral rules section for CLAUDE.md.

3. **Updated template:** `cmd/cerebro/templates/settings.json` -- add Stop hook to the hooks object.

4. **Updated template:** `cmd/cerebro/templates/claudemd_section.md` -- use paired markers (`<!-- cerebro:memory-system:start -->` / `<!-- cerebro:memory-system:end -->`).

5. **Updated scaffold.go:**
   - `scaffoldSkills()` adds `implement` to the skills map
   - `scaffoldCLAUDEMD()` updated for paired markers and optional behavioral rules section
   - `scaffoldSettings()` includes Stop hook in template

6. **Updated scaffold_test.go:** Tests for all new templates and updated scaffolding behavior.

7. **No new CLI commands.** No `harness sync`, `harness status`, or `harness diff`. The existing `cerebro init` and `cerebro init --force` are sufficient.

### 6.3 Phase 2 deliverables (conditional -- see trigger in Section 4.3)

1. **PostToolUse logging hook** in settings.json template -- logs tool calls to a session file for offline read:edit ratio analysis.

2. **Skill-scoped PreToolUse hook** in the /implement skill's frontmatter -- enforces read-before-edit only during structured implementation.

### 6.4 Phase 3 deliverables (conditional -- see trigger in Section 4.4)

1. **Agent-type Stop hook** as an opt-in addition. Not embedded in the default template. Documented as a power-user option.

---

## 7. What the Original Document Gets Right

Credit where due -- several aspects of the original document are well-reasoned and survive scrutiny:

1. **The tiered approach (core, agents, domain packs)** mirrors Claude Code's own layering and is sound.
2. **Deferring domain packs** is correct. The trigger condition ("when 3+ projects are running with the core harness") is reasonable.
3. **Rejecting prompt-type Stop hooks** is correct. Self-evaluation degeneracy is well-documented (Anthropic harness article).
4. **The evaluator agent design** (model: sonnet, skeptical focus, no praise) is well-reasoned.
5. **The lightweight /implement skill** (60-80 lines with 5 phases) is well-scoped. The heavy version's existence proves the pattern.
6. **The embedding vs external registry decision** is correct. `//go:embed` is already in use and appropriate for <100KB of templates.
7. **The external research section** (Anthropic harness article, gstack, 12 patterns, everything-claude-code) is thorough and well-cited.

---

## 8. What the Original Document Gets Wrong

1. **"Skills are project-scoped only -- no global mechanism exists."** False. Personal skills in `~/.claude/skills/` are available across all projects. This was the central portability argument and it is incorrect.

2. **"Exit code 1 blocks the stop and forces continuation."** False. Exit code 1 is a non-blocking error. Exit code 2 is required to block. This is critical for the Stop hook and the read-before-edit hook. Getting this wrong would make the hooks silently ineffective.

3. **The Stop hook is deferred to Phase 5.** The stop-phrase-guard is the single most quantitatively validated mitigation in the evidence base. Deferring it while building template management infrastructure is misaligned with the evidence.

4. **The template management system (manifests, checksums, three-way diffs) is over-engineered.** The portability problem is smaller than claimed (see Section 1.2). The existing `cerebro init --force` mechanism is sufficient for 5-8 template files.

5. **Skill precedence is backwards.** The document assumes project-local skills override global templates. The official docs state the opposite: enterprise > personal > project. This affects the entire scaffolding strategy.

6. **"Claude Code comparison matrix changed, support articles changed."** The Pro plan removal situation (evidence-catalog.md, April 22 entry) is presented as evidence of a pattern but is flagged as "developing" and "contradictory across Anthropic's own pages." The original document doesn't reference this at all since it was written first, but the broader point stands: the harness proposal should not be motivated by commercial changes to Anthropic's pricing.

---

## 9. Open Questions and Evidence Gaps

1. **Does the lightweight /implement skill actually improve outcomes in non-EDP projects?** No evidence exists yet. Phase 0 measurement is required before investing in Phase 1 Cerebro changes.

2. **What is the false-positive rate of a stop-phrase-guard?** The #42796 data shows 173 violations in 17 days, but does not report false positives. A phrase like "for now" may match legitimate usage. The hook needs tuning against real sessions.

3. **Do behavioral rules in CLAUDE.md survive compaction?** The best practices docs state CLAUDE.md is loaded every session, but compaction may lose nuance. Skills survive compaction (first 5K tokens per skill, 25K total budget). If CLAUDE.md rules are frequently lost, they should migrate to a skill.

4. **Is the skill precedence (enterprise > personal > project) stable?** This is a recent documentation addition. If Anthropic changes this (e.g., to allow project skills to override personal), the scaffolding strategy changes.

5. **How does gstack (66K+ stars) distribute its skills?** The original document mentions gstack but does not analyze its distribution mechanism. If gstack solves the portability problem in a way Cerebro could adopt, that changes the build-vs-adopt decision. gstack is distributed as a [Claude Code plugin](https://gstacks.org/), which is a distribution mechanism Cerebro does not use.

6. **Should Cerebro become a plugin?** The Claude Code plugin system ([plugins docs](https://code.claude.com/docs/en/plugins-reference)) packages skills, hooks, subagents, and MCP servers into installable units. This is precisely what the "harness manager" proposal describes. If Cerebro shipped as a plugin rather than a standalone CLI, it would gain distribution, versioning, and discovery for free. This is a strategic question that deserves its own analysis.

---

## 10. Summary: What to Build, in What Order

| Phase | Scope | Cerebro code changes? | Trigger |
|-------|-------|----------------------|---------|
| **0** | Deploy behavioral rules + evaluator + /implement + Stop hook manually | No | Immediate |
| **1** | Improve Cerebro templates (implement skill, behavioral rules, Stop hook, paired markers) | Yes (templates + scaffold.go) | After Phase 0 measurement shows value |
| **2** | Add read-before-edit monitoring/enforcement | Yes (templates) | After Phase 0 shows read:edit ratio problem persists |
| **3** | Agent-type Stop hook (opt-in) | Yes (templates) | After Phases 0-2 show assumption-making persists |

**Total new CLI commands: Zero.** The existing `cerebro init` and `cerebro init --force` are sufficient.

**Total new Go files: Zero.** Changes are limited to templates and scaffold.go/scaffold_test.go.

**Estimated effort:**
- Phase 0: 1 session (manual file creation + baseline measurement)
- Phase 1: 1-2 sessions (template creation + scaffold updates + tests)
- Phase 2: 1 session (hook development + testing)
- Phase 3: 1 session (agent hook development + opt-in mechanism)

This is approximately 60% less scope than the original document's 5-phase proposal, while addressing the same root cause vectors and being grounded in evidence rather than assumption.

---

## Appendix A: Evidence Citation Index

| Claim | Source |
|-------|--------|
| Stop-phrase-guard: 0 to 173 violations in 17 days | evidence-catalog.md, timeline Mar 8-25; approach.md Section 3.1 |
| Read:edit ratio collapsed 6.6 to 2.0 | approach.md Section 3.1; evidence-catalog.md timeline |
| Zero-thinking-tokens confirmed by Boris Cherny | evidence-catalog.md, Boris Cherny HN thread (item 47668520) |
| Effort=high is necessary but not sufficient | settings-matrix.md Section 1; approach.md Section 4 Vector 1 |
| Self-evaluation degeneracy | Anthropic harness article: "confidently praising the work" |
| Context anxiety / premature wrapping | Anthropic harness article: "Models wrap up work prematurely as context fills" |
| Exit code 2 blocks, exit code 1 does not | [Hooks reference](https://code.claude.com/docs/en/hooks), exit code behavior table |
| Personal skills in `~/.claude/skills/` | [Skills docs](https://code.claude.com/docs/en/skills), "Where skills live" table |
| Skill precedence: enterprise > personal > project | [Skills docs](https://code.claude.com/docs/en/skills), precedence note |
| Skills survive compaction (5K tokens each, 25K total) | [Skills docs](https://code.claude.com/docs/en/skills), "Skill content lifecycle" |
| Skills support `effort`, `model`, `hooks` frontmatter | [Skills docs](https://code.claude.com/docs/en/skills), frontmatter reference table |
| Arrays merge across settings scopes | [Settings docs](https://code.claude.com/docs/en/settings), merge behavior |
| 24 hook event types | [Hooks reference](https://code.claude.com/docs/en/hooks), event list |
| Agent hooks are experimental | [Hooks reference](https://code.claude.com/docs/en/hooks): "Agent hooks are experimental and may change" |
| gstack: 66K+ stars, plugin distribution | [gstack repo](https://github.com/garrytan/gstack); [TechCrunch coverage](https://techcrunch.com/2026/03/17/why-garry-tans-claude-code-setup-has-gotten-so-much-love-and-hate/) |
| Cerebro scaffold.go: string-match detection | Codebase: `cmd/cerebro/scaffold.go` line 79 |
| Cerebro templates: 5 files, <15KB | Codebase: `cmd/cerebro/templates/` |
| settings.local.json can clobber permissions | Original document Section 7.3 (verified as Claude Code behavior) |
| September 2025 postmortem: evals failed to detect degradation | evidence-catalog.md, Historical Precedent section |
| CLAUDE.md best practices: "Would removing this cause Claude to make mistakes?" | [Best practices](https://code.claude.com/docs/en/best-practices) |
