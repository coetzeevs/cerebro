# Tech Lead Reviews Architect's Harness Proposal

**Date:** 2026-04-22
**Reviewing:** `architect-harness-proposal.md`
**Against:** `techlead-harness-proposal.md` (my own)
**Method:** Point-by-point comparison, factual disputes resolved against primary sources (Context7 Claude Code docs, codebase, evidence-catalog.md, approach.md)

---

## 1. Where the Architect Is Right and I Was Wrong

### 1.1 Skill portability -- I was wrong

The original `harness-management-research.md` states: "Skills are project-scoped only -- no global mechanism exists" (Section 1, portability table; Section 4.3; Section 7.3). My proposal did not challenge this claim. The architect's proposal (Section 1.2) identifies this as false.

**Verification via Context7 (Claude Code docs, ericbuess/claude-code-docs):**

> "When multiple skills share the same name, a priority system determines which skill is used: enterprise skills take precedence over personal skills, which in turn take precedence over project skills."

> "Skills can be placed in Enterprise settings for organization-wide use, in a personal directory for use across all your projects, or in a project-specific directory for use only within that project."

The docs confirm `~/.claude/skills/` exists as a "personal" scope available across all projects. The original document's portability argument -- and my acceptance of it -- was based on an incorrect premise. The architect caught this; I did not.

**Impact on my proposal:** My Section 3.3 accepts the portability problem at face value and proposes a project-level `/implement` skill as the solution. The architect's correction does not invalidate this recommendation (project-level scaffolding is still correct because project overrides personal in precedence) but it weakens the urgency -- the user could create `~/.claude/skills/implement/SKILL.md` today and get global coverage without any Cerebro changes.

### 1.2 Skill precedence direction -- I was wrong

The architect identifies (Section 5.3) that skill precedence is enterprise > personal > project, meaning personal skills override project skills, not the other way around. My proposal (Section 3.3) states "Claude Code loads project-local skills over global templates" which is incorrect per the official docs.

**Impact:** This reverses the layering assumption. If Cerebro scaffolds a lightweight `/implement` to project `.claude/skills/`, the user cannot simply place a heavy version in the same project directory and have it win -- the project-level version does win because it is the lowest priority (enterprise > personal > project). Wait: re-reading the precedence, project is lowest priority. So if the user has a personal `~/.claude/skills/implement/SKILL.md`, it will override Cerebro's project-level scaffold. This is the opposite of what both the original document and I assumed, but it actually works in our favor: Cerebro scaffolds the lightweight version at project level, the user's personal heavy version at `~/.claude/skills/` overrides it everywhere, and project-specific heavy versions... cannot override the personal one.

The architect catches this conflict and correctly flags it: "A personal `/implement` in `~/.claude/skills/` would be a bad idea because it would override every project's custom version." This is a genuine design constraint that I missed entirely.

### 1.3 Stop hook prioritization -- the architect is correct

My proposal (Section 3.6) conditionally accepts the Stop hook and recommends it as Tier A (template-only change). The architect goes further and makes it Phase 0 (immediate, no code changes). The architect is correct that the original document's deferral of the Stop hook to Phase 5 is backwards -- the stop-phrase-guard is the single most quantitatively validated mitigation in the evidence base (0 to 173 violations in 17 days, evidence-catalog.md timeline).

I agree with the architect's prioritization here. My proposal reached the same conclusion (the stop hook is the highest-evidence intervention) but was less emphatic about making it the very first thing deployed. The architect is right to make it Phase 0.

### 1.4 Root cause coverage analysis -- the architect's is better

The architect's Section 2 performs a systematic vector-by-vector analysis of what a harness can and cannot address. My proposal does not do this. This is a genuine gap in my analysis.

The architect's conclusion -- "A harness meaningfully addresses 3 of 6 vectors (V3, V5, V6), partially addresses 1 (V1), and cannot address 2 (V2, V4)" -- is sound and well-evidenced. This framing prevents overpromising what a harness can deliver, which the original document does.

### 1.5 Skill frontmatter capabilities -- the architect found features I missed

The architect's Section 3.3 identifies several skill features that neither my proposal nor the original document discusses:

- `effort` frontmatter field: Skills can override session effort level. An `/implement` skill with `effort: high` addresses Vector 1 directly.
- `hooks` frontmatter: Skills can carry their own hooks, enabling scoped enforcement (read-before-edit only during `/implement`).
- `context: fork` for subagent execution.
- Skills survive compaction (first 5K tokens per skill, 25K total budget).

These are verified via Context7. The `effort` frontmatter finding is particularly significant: it means the `/implement` skill can guarantee `effort: high` regardless of session defaults, which is the single most direct intervention against the zero-thinking-tokens problem (approach.md Section 4 Vector 1).

My proposal's Section 3.3 recommends the `/implement` skill but does not mention `effort: high` frontmatter. The architect's version is materially better because of this.

---

## 2. Where the Architect Is Wrong or Evidence Is Insufficient

### 2.1 Exit code semantics -- the architect conflates two mechanisms

The architect (Section 8, point 2) states: "Exit code 1 blocks the stop" is false and "Exit code 2 is required to block." This is presented as a critical error in the original document.

**Verification via Context7 (Claude Code hooks reference):**

> "An exit code of `0` signifies success, and its standard output is included in the transcript. An exit code of `2` indicates a blocking error, causing the hook's standard error output to be fed back to Claude."

The architect is correct that exit 2 is the blocking mechanism. However, the original document (Section 6.2) actually uses the correct JSON decision format (`{"decision": "approve"}` / `{"decision": "block", "reason": "..."}`), not exit codes. The original document's Stop hook specification in Section 6.2 shows exit 0 with JSON stdout decision, which is a valid alternative per the official docs.

My own proposal (Section 3.6) also uses the JSON stdout decision format on exit 0, not exit code 2. Both the JSON-decision-on-exit-0 pattern and the exit-2-with-stderr pattern are valid. The architect's Stop hook specification (Section 4.1) uses exit 2 with grep, which is the exit-code approach. Both work. The architect overstates this as a "critical" error when the original document's actual specification is correct.

**However:** The architect's Phase 0 Stop hook implementation (Section 4.1) uses `exit 2` and writes to stderr, while the docs show that exit 0 with JSON stdout `{"decision": "block"}` is the canonical Stop hook pattern. The architect's own implementation mixes paradigms: using `exit 2` (the PreToolUse blocking pattern) for a Stop hook where JSON decision output is the documented approach. The Context7 docs for Stop hooks specifically show the JSON decision format, not exit code 2.

This is a factual conflict where the architect is partly right (exit 2 does block) but applies it to the wrong hook type. For Stop hooks specifically, the documented pattern is exit 0 with `{"decision": "block"}` in stdout.

### 2.2 Behavioral rules in CLAUDE.md template -- disagree on approach

The architect (Section 4.2, Phase 1) proposes adding behavioral rules to the Cerebro CLAUDE.md template. My proposal (Section 3.1) rejects this.

My reasoning: Adding behavioral rules to every project's CLAUDE.md via `cerebro init` creates duplication with `~/.claude/CLAUDE.md` where the user already has rules. The Claude Code best practices say CLAUDE.md should be 50-100 lines (original document Section 3.4). Duplicating rules across global and every project CLAUDE.md wastes tokens.

The architect acknowledges this tension (Section 5.4): "Cerebro should NOT scaffold behavioral rules into the CLAUDE.md template because: 1. They are universal, not project-specific. 2. `~/.claude/CLAUDE.md` is user-managed." But then adds: "However, Cerebro CAN add behavioral rules to the CLAUDE.md template for projects that lack a global CLAUDE.md."

This is internally contradictory. The architect correctly identifies why it should not be done, then adds it back as Phase 1 anyway (Section 4.2: "Add behavioral rules section to CLAUDE.md template"). The evidence does not support adding advisory rules that fail when thinking depth is zero (approach.md Vector 1) to a template that will be duplicated across every `cerebro init`'d project.

### 2.3 Paired CLAUDE.md markers -- premature

The architect (Section 4.2, Phase 1) proposes paired markers (`<!-- cerebro:memory-system:start -->` / `<!-- cerebro:memory-system:end -->`) to replace the current single-marker approach.

My proposal (Section 3.5, Tier D, and Section 12 summary) rejects this as unnecessary. The current implementation in `scaffold.go` lines 232-297 uses a single marker (`## Cerebro Memory System`) and finds the next `## ` heading to determine the section boundary. This is tested in `scaffold_test.go` with 14 tests including trailing section preservation.

The architect calls the current approach "fragile" but does not cite a failure case. The existing tests cover: new file creation, force-replace with trailing sections, non-force skip-if-present. The current implementation works. Adding paired markers changes the format of every existing CLAUDE.md, requires a migration path for users who already ran `cerebro init`, and adds complexity without a demonstrated failure to fix.

### 2.4 Evaluator agent as Phase 0 -- premature

The architect places the evaluator agent in Phase 0 (immediate, no code changes). My proposal (Section 3.2) defers it.

The evidence for the evaluator agent is theoretical (Anthropic harness article). The Anthropic article itself warns: "Out of the box, Claude is a poor QA agent... required several rounds of prompt tuning." No one has demonstrated an effective evaluator agent in the `~/.claude/agents/` context with measurable improvement.

The user already has 12 swarm agents including `swarm-tech-lead.md`. Adding another agent file costs nothing, but including it in the recommendation set without evidence of effectiveness dilutes the signal. Phase 0 should contain only proven interventions (the Stop hook) and the single highest-leverage structural change (the `/implement` skill).

### 2.5 gstack star count -- unverified and irrelevant

The architect cites gstack as having "66K+ stars" (Section 0, Section 9). This number is not verifiable from the documents we have and is irrelevant to the technical analysis. Star counts do not establish technical merit. The architect uses it as a credibility signal ("gstack (66K+ stars)") but this is appeal to popularity, not evidence.

### 2.6 Claude Code plugin suggestion -- scope creep

The architect raises (Section 9, point 6): "Should Cerebro become a plugin?" and describes the plugin system as "precisely what the harness manager proposal describes."

This is a strategic question that does not belong in a harness design document. It changes Cerebro's distribution model, dependency structure, and governance. Introducing it as an open question in a proposal about template improvements creates scope confusion. The architect correctly marks it as needing "its own analysis" but should not have raised it here at all.

---

## 3. Factual Conflicts Between Proposals

### 3.1 Stop hook implementation pattern

| Point | Tech Lead proposal | Architect proposal | Primary source says |
|-------|-------------------|-------------------|-------------------|
| Stop hook blocking mechanism | Exit 0 + JSON stdout `{"decision": "block"}` | Exit 2 + stderr message | **Both are valid.** Context7 shows the JSON decision format as the Stop hook pattern (exit 0). Exit 2 is the general blocking pattern for PreToolUse/other hooks. For Stop hooks specifically, JSON decision on stdout is the documented canonical form. |
| Where to deploy Stop hook | Project `.claude/settings.json` template (Tier A) | User `~/.claude/settings.json` (Phase 0) | Both work. User-level applies globally; project-level is per-project. For immediate deployment, user-level is correct (architect wins). For Cerebro template work, project-level is correct (both agree on Phase 1). |

### 3.2 Skill portability

| Point | Tech Lead proposal | Architect proposal | Primary source says |
|-------|-------------------|-------------------|-------------------|
| `~/.claude/skills/` exists | Not addressed (accepted original doc's "No") | Yes, personal skills exist | **Architect is correct.** Context7 confirms `~/.claude/skills/` as personal scope. |
| Skill precedence | "Claude Code loads project-local skills over global templates" | Enterprise > Personal > Project | **Architect is correct.** Docs confirm this order. |

### 3.3 gstack "Go-native engine" attribution

| Point | Tech Lead proposal | Architect proposal | Primary source says |
|-------|-------------------|-------------------|-------------------|
| Who has the Go-native engine | "The Go-native engine is Chachamaru127's claude-code-harness, not gstack" (Section 8, point 1; Section 9 table) | Lists gstack and claude-code-harness as separate projects (Section 0) | **Unable to fully verify.** The original document (Section 3.2) lists "Go-native engine: ~10ms per phase processing" under gstack. The architect does not challenge this attribution. My proposal claims this is an error but I cannot verify definitively without checking the gstack repository. The architect's treatment (listing both as separate tools without cross-attributing) is more cautious. I should have been less confident in this correction without verification. |

---

## 4. Gaps in the Architect's Proposal

### 4.1 No `cerebro stop-guard` subcommand

My proposal (Section 5, Tier C) recommends a `cerebro stop-guard` Go subcommand that reads stdin and outputs the JSON decision. This follows the existing pattern (`cerebro gc` from SessionEnd, `cerebro recall --prime` from SessionStart). The architect's proposal has zero new Go commands -- the Stop hook is an inline bash script in settings.json.

The inline bash approach has two problems:
1. It depends on `grep -oiE` behavior which varies across platforms (GNU vs BSD grep)
2. The phrase list cannot be updated atomically with the binary -- users must re-run `cerebro init --force` AND the hook script must be updated in settings.json

A Go subcommand is testable, cross-platform, and ships phrase list updates atomically. The architect misses this entirely.

### 4.2 No testing strategy for Stop hook

The architect's Phase 0 deploys a Stop hook manually with no test plan. My proposal (Section 10) specifies 5 test cases for the stop-guard. The architect mentions "Phase 0 measurement" but this is outcome measurement (did degradation improve?), not unit testing of the hook itself.

For a hook that blocks the agent from stopping, false positives are costly -- they prevent Claude from completing legitimately finished tasks. Testing the phrase list against normal completion messages is essential before deployment. The architect does not address this.

### 4.3 No maintenance burden analysis

My proposal (Section 11) includes a maintenance burden table for each proposed change. The architect's proposal does not assess ongoing maintenance cost. For a project maintained by a solo engineer (per the codebase context), this matters.

### 4.4 Missing the settings baseline

Both proposals mention settings.json changes, but the architect does not explicitly call out deploying the settings baseline from `settings-matrix.md` as the highest-priority zero-code action. My proposal (Section 4, Section 5 "DO FIRST") identifies this as the single highest-ROI action: `effortLevel: "high"`, `showThinkingSummaries: true`, earlier compaction thresholds. The architect's Phase 0 focuses on CLAUDE.md rules, evaluator agent, and Stop hook, but does not mention settings baseline deployment.

This is a significant gap. Settings are deterministic (the system enforces them regardless of model behavior), unlike CLAUDE.md rules (which are advisory and fail on zero-thinking turns).

### 4.5 The `~/.claude/skills/implement/SKILL.md` placement is wrong by the architect's own logic

The architect's Phase 0 (Section 6.1) places the lightweight `/implement` at `~/.claude/skills/implement/SKILL.md` (personal scope). But Section 5.3 correctly identifies that personal skills override project skills. This means if the user later creates a project-specific heavy `/implement`, the personal lightweight version will win.

The architect catches this contradiction in Section 5.3 ("A personal `/implement` in `~/.claude/skills/` would be a bad idea") and then corrects the placement to project-level. But Phase 0 (Section 6.1) still shows the file at `~/.claude/skills/implement/SKILL.md`. This is an internal inconsistency within the architect's document.

---

## 5. Gaps in MY Proposal That the Architect's Review Exposed

### 5.1 I did not question skill portability

The original document's claim about skills being project-only was central to the "portability" argument. I accepted it without verification. The architect checked the official docs and found it was wrong. This is a failure of due diligence on my part.

### 5.2 I missed the `effort` frontmatter for skills

The ability to set `effort: high` per-skill is the single most targeted intervention against Vector 1 (zero-thinking-tokens). My `/implement` skill proposal does not include this. The architect's does. This is a material improvement I should have found.

### 5.3 I did not perform root cause coverage analysis

The architect's vector-by-vector analysis (Section 2) establishes clear boundaries for what a harness can and cannot do. My proposal jumps from "what the evidence shows" to "what to build" without this intermediary analysis. This makes my proposal vulnerable to scope creep -- without explicit boundaries, it is harder to justify why certain proposals are rejected.

### 5.4 I missed skill compaction survival

The architect notes that skills survive compaction (first 5K tokens per skill, 25K total budget). This makes skills a more durable vehicle for behavioral rules than CLAUDE.md content in long sessions. My proposal does not discuss compaction survival at all.

### 5.5 I missed the 24 hook event types

The architect notes (Section 3.3) that Claude Code now supports 24 hook events, not just the 6 currently used. While most of the additional events are not immediately relevant, I should have documented the full landscape rather than analyzing only the events the original document discussed.

---

## 6. The Biggest Open Disagreement

**The single most important thing we see differently: whether Phase 0 should include the evaluator agent and behavioral rules, or only the Stop hook and settings baseline.**

The architect's Phase 0 includes four items: behavioral rules, evaluator agent, `/implement` skill, and Stop hook. My recommended "DO FIRST" is: settings baseline deployment and Stop hook.

**Why this matters:** Phase 0 is the measurement baseline. Everything built afterward is justified (or not) by Phase 0 results. If Phase 0 includes too many interventions, you cannot attribute improvement to any specific one. If it includes unproven interventions (evaluator agent), a positive result may be driven entirely by the proven ones (Stop hook), giving false confidence in the unproven ones.

**My position:** Phase 0 should contain the minimum set of proven interventions: (1) settings baseline from settings-matrix.md, (2) command-type Stop hook. Measure. Then add the `/implement` skill (proven pattern via the user's existing 283-line version). Then add the evaluator agent only if assumption-making persists.

**The architect's position:** Deploy everything cheap simultaneously (all four items cost ~1 hour), measure the aggregate effect. Practical efficiency over scientific rigor.

**What would resolve this:** Run Phase 0 with only the Stop hook + settings baseline for 5 tasks. Then add `/implement` for 5 tasks. Then add the evaluator for 5 tasks. Sequential introduction with measurement after each step. This takes longer but produces attributable results. If time pressure makes this impractical, the architect's "deploy everything" approach is defensible -- but the resulting measurement will be confounded.

---

## 7. Summary of Corrections to My Proposal

| Item | My original position | Corrected position | Source |
|------|---------------------|-------------------|--------|
| Skill portability | Accepted "project-only" claim | `~/.claude/skills/` exists as personal/global scope | Context7: Claude Code skills docs |
| Skill precedence | "Project-local overrides global" | Enterprise > Personal > Project (project is lowest priority) | Context7: Claude Code skills docs |
| `/implement` effort frontmatter | Not mentioned | Should include `effort: high` in frontmatter | Context7: Claude Code skills docs |
| Root cause coverage analysis | Not performed | Should have mapped proposals to specific vectors | architect-harness-proposal.md Section 2 |
| Skill compaction survival | Not discussed | Skills survive compaction (5K tokens each, 25K total) | Context7: Claude Code skills docs |
| Stop hook exit code claim | My JSON-decision-on-exit-0 approach is correct | Exit 2 also works but is the PreToolUse pattern, not the Stop hook canonical form | Context7: Claude Code hooks reference |
| gstack Go-native engine attribution | Claimed it was Chachamaru127's, not gstack's | Cannot verify without checking repos; was too confident | Insufficient evidence to make the correction |
