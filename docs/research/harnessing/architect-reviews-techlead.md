# Architect Review of Tech Lead Harness Proposal

**Date:** 2026-04-22
**Reviewer:** Principal Architect
**Document under review:** `techlead-harness-proposal.md`
**Cross-reference:** `architect-harness-proposal.md`

---

## 1. Where the Tech Lead Is Right and I Was Wrong

### 1.1 The Stop hook uses JSON output protocol, not just exit codes

My proposal (Section 4.1) specifies a Stop hook that uses `exit 2` with stderr to block premature stopping. The tech lead's proposal (Section 3.6, line 200) correctly identifies that Stop hooks have their own JSON output protocol: exit 0 with `{"decision": "block", "reason": "..."}` on stdout, or `{"decision": "approve"}` to allow stopping.

The Context7 Claude Code docs confirm both mechanisms exist:
- Exit 0 + stdout JSON `{"decision": "block"}` is the Stop-hook-specific protocol (Context7: "Stop Decision Control JSON Output")
- Exit 2 + stderr is the generic hook blocking mechanism (Context7: "Exit code of 2 indicates a blocking error")

My proposal conflated these two mechanisms. The tech lead's implementation (line 313) using `printf '{"decision": "block", ...}'` with exit 0 is the more correct approach for Stop hooks specifically. The exit 2 approach works generically but bypasses the Stop hook's purpose-built decision protocol.

**Correction accepted.** The Stop hook should use exit 0 + JSON decision, not exit 2 + stderr.

### 1.2 The `cerebro stop-guard` subcommand is a better design than inline bash

My proposal (Section 4.1) specifies the Stop hook as an inline bash one-liner in settings.json. The tech lead's proposal (Section 5, Tier C) proposes a `cerebro stop-guard` Go subcommand instead. Their argument (line 358-363) is stronger:

- Phrase list ships atomically with the binary version
- Testable in Go with unit tests
- Cross-platform (no dependency on python3 for JSON parsing)
- Follows the existing pattern: `cerebro gc` runs from SessionEnd, `cerebro recall --prime` from SessionStart, `cerebro stop-guard` from Stop

I missed this. A Go subcommand is more consistent with Cerebro's existing architecture and eliminates the tech lead's own python3 dependency in their inline version (line 313). Both of our inline implementations are fragile; the subcommand is the right answer.

**Correction accepted.** `cerebro stop-guard` should be the target implementation, with the inline bash as a Phase 0 manual deployment step only.

### 1.3 The settings baseline from settings-matrix.md is missing from my proposal

The tech lead (Section 4, line 277-282) correctly identifies that deploying the settings baseline from `settings-matrix.md` (effort=high, showThinkingSummaries=true, earlier compaction) is the single lowest-effort, highest-impact change -- and my proposal does not mention it. My Phase 0 jumps straight to behavioral rules, evaluator agents, and Stop hooks without first deploying the already-researched settings baseline.

This is a real gap. The settings baseline requires zero code changes, zero file creation, and addresses Vector 1 (thinking depth) and Vector 3 (context degradation) more directly than anything else in my Phase 0.

**Correction accepted.** The settings baseline should be the very first item in Phase 0, before everything else.

### 1.4 The gstack "Go-native engine" attribution is wrong in the original document

The tech lead (Section 8, item 1; Section 9, line 453) correctly identifies that the original `harness-management-research.md` (line 125) attributes a "Go-native engine: ~10ms per phase processing" to gstack. gstack is a pure-Markdown slash command toolkit; the Go engine belongs to Chachamaru127's `claude-code-harness` (original document Section 3.6, line 175).

My proposal (Section 4.1, line 215) attributed the "sub-10ms" benchmark to "Chachamaru127's Go engine benchmarks," which is the correct attribution. But I did not flag the original document's misattribution. The tech lead caught it; I did not.

**Credit to the tech lead** for catching this factual error in the original document.

### 1.5 Cerebro should not scaffold to `~/.claude/agents/`

The tech lead (Section 3.9, line 253) argues that Cerebro writing to `~/.claude/agents/` crosses a scope boundary -- Cerebro manages per-project state (brain, hooks, skills, CLAUDE.md section), and global Claude Code configuration is a different domain. The user already has 12 global agents created manually.

My proposal (Section 6.1, Phase 0) includes creating `~/.claude/agents/evaluator.md` as a manual step, not a Cerebro-scaffolded step. But my Phase 0 framing blurs the line between "things the user does manually" and "things Cerebro should manage." The tech lead draws this boundary more cleanly: if it is in `~/.claude/`, it is the user's concern.

**Partially accepted.** The evaluator agent is still a valid Phase 0 manual action, but the tech lead is right that Cerebro should never scaffold it. My proposal does not propose Cerebro scaffolding it, but I should have been more explicit about this boundary.

---

## 2. Where the Tech Lead Is Wrong or Their Evidence Is Insufficient

### 2.1 CRITICAL: "Claude Code has no `~/.claude/skills/` mechanism" is factually false

The tech lead states (Section 3.3, line 150): "Claude Code has no `~/.claude/skills/` mechanism."

This is wrong. The official Claude Code documentation, verified via Context7 (source: code.claude.com/docs/en/skills), states:

> "Personal skills are accessible across all your projects and are stored in `~/.claude/skills/`."

And (source: code.claude.com/docs/en/slash-commands):

> "When skills have the same name across different locations, the higher-priority location takes precedence: Enterprise > Personal > Project."

The tech lead also states in Section 9 (line 454) that the original document's claim ("Skills are project-scoped only") is "Correct, but understated." It is not correct. Personal skills exist at `~/.claude/skills/` and are documented in the official docs.

The fact that `~/.claude/skills/` does not currently exist on the user's machine (verified: `ls ~/.claude/skills/` returns "Directory does not exist") means the user has not used this feature, not that it does not exist. The tech lead appears to have relied on the user's filesystem state rather than checking the documentation.

This error matters because it affects several downstream conclusions:
- The tech lead's verdict on `/implement` skill placement (Section 3.3): "Skills are project-scoped" is the basis for rejecting personal skill placement. The actual precedence (enterprise > personal > project) means a personal `/implement` would override every project's version -- which is what my proposal flagged as a risk (Section 5.3, lines 334-338).
- The tech lead's Section 9 "correction" of the original document is itself incorrect.

### 2.2 Rejecting behavioral rules in CLAUDE.md is not evidence-based

The tech lead (Section 3.1, line 132) rejects adding behavioral rules to the Cerebro CLAUDE.md template, arguing: "The user's global CLAUDE.md is the correct place for universal behavioral rules. Adding them to every project's CLAUDE.md via Cerebro creates duplication."

This reasoning has two problems:

1. **The duplication argument is valid but the conclusion is wrong.** The correct response to "these rules should be global, not per-project" is to scaffold them at the global level or document them as a recommendation -- not to reject them entirely. My proposal (Section 5.4, lines 354-359) addresses this by recommending the rules go in `~/.claude/CLAUDE.md` (user-managed) and only appear in the project template as a fallback for projects without a global CLAUDE.md.

2. **The "rules fail at zero thinking depth" argument proves too much.** The tech lead uses the zero-thinking-tokens failure mode (approach.md Vector 1) to argue against CLAUDE.md rules. By this logic, the `/implement` skill they ACCEPT should also be rejected -- skills are equally "advisory" when the model allocates zero thinking tokens. The tech lead acknowledges this at line 343: "the skill is advisory -- the model must choose to follow it." The same applies to CLAUDE.md rules. The difference is degree (skills are explicitly invoked; rules are passively loaded), not kind.

### 2.3 Rejecting paired CLAUDE.md markers lacks technical justification

The tech lead (Section 5, Tier D table, line 377) rejects paired section markers in CLAUDE.md: "Current single-marker + trailing-section preservation works (tested in `scaffold_test.go`)."

The current implementation (scaffold.go) finds the `## Cerebro Memory System` marker and assumes everything below it is the cerebro section, preserving content above the marker. This works only when the cerebro section is the LAST section in the file. If a user adds content after the cerebro section, `cerebro init --force` will either delete it or require fragile line-counting.

My proposal (Section 4.2, line 249) recommends paired markers (`<!-- cerebro:memory-system:start -->` / `<!-- cerebro:memory-system:end -->`) precisely because the current approach is fragile. The tech lead dismisses this without engaging with the fragility argument. The fact that tests pass today does not mean the approach handles all real-world CLAUDE.md layouts.

### 2.4 The Stop hook stdin format uncertainty is overstated

The tech lead (Section 13, question 1, line 527) lists "What exactly does Claude Code pass to Stop hook stdin?" as an open question. But their own implementation (line 313) already parses `stop_message` from stdin JSON, and the Context7 docs show the exact JSON structure for Stop hooks. The format is documented; it is not an open question.

### 2.5 The python3 dependency in the Stop hook implementation

The tech lead's inline Stop hook implementation (Section 5, A1, line 313) uses `python3 -c "import sys,json; ..."` for JSON parsing. This introduces a runtime dependency on python3 that may not be present on all systems (Windows, minimal Docker containers). This contradicts their own argument for a Go subcommand (line 361): "Cross-platform (no dependency on python3 for JSON parsing)." The inline version and the Go subcommand are presented as alternatives, but the inline version has the exact flaw the Go subcommand is designed to fix.

My proposal's inline version uses `grep -oiE` for pattern matching without JSON parsing, which is more portable (POSIX) but less precise (matches against the full stdin, not just the stop_message field). Neither inline approach is ideal, which reinforces the case for `cerebro stop-guard`.

---

## 3. Factual Conflicts Between Our Proposals

### 3.1 Do personal skills exist at `~/.claude/skills/`?

| Claim | Architect proposal | Tech lead proposal | Correct answer |
|-------|-------------------|-------------------|----------------|
| Personal skills exist | Yes (Section 1.2, line 46) | No (Section 3.3, line 150; Section 9, line 454) | **Yes.** Context7 (code.claude.com/docs/en/skills): "Personal skills are accessible across all your projects and are stored in `~/.claude/skills/`." |

### 3.2 What is the skill precedence order?

| Claim | Architect proposal | Tech lead proposal | Correct answer |
|-------|-------------------|-------------------|----------------|
| Precedence | Enterprise > Personal > Project (Section 5.3, line 336) | Not addressed (assumes project-scoped only) | **Enterprise > Personal > Project.** Context7 (code.claude.com/docs/en/slash-commands): "higher-priority location takes precedence: Enterprise > Personal > Project." |

### 3.3 Does the original document claim "exit code 1 blocks"?

| Claim | Architect proposal | Tech lead proposal | Correct answer |
|-------|-------------------|-------------------|----------------|
| Original doc's exit code claim | "Exit code 1 blocks" is wrong; should be exit 2 (Section 8, item 2) | "Exit code 1 blocks" is wrong; should be exit 0 + JSON (Section 9, line 456) | **The original document does not mention exit codes 1 or 2 at all.** Verified by grep against `harness-management-research.md`. Both our proposals criticize a claim the original document does not make. The original document's Stop hook section (Section 6) discusses prompt-type vs agent-type vs command-type without specifying exit code behavior. |

This is an error in both proposals. We both attributed a claim to the original document that it did not make.

### 3.4 How should the Stop hook block stopping?

| Claim | Architect proposal | Tech lead proposal | Correct answer |
|-------|-------------------|-------------------|----------------|
| Blocking mechanism | Exit 2 + stderr message (Section 4.1, line 228) | Exit 0 + stdout JSON `{"decision": "block"}` (Section 5, A1, line 313) | **Both work, but the JSON decision protocol is the Stop-hook-specific mechanism.** Context7 shows Stop hooks support `{"decision": "block"}` on exit 0 (Stop-specific) AND exit 2 with stderr (generic). The JSON protocol is purpose-built for Stop hooks and provides structured decision/reason output. The exit 2 approach works but is the generic blocking mechanism, not the Stop-specific one. The tech lead is more correct here. |

---

## 4. Gaps in the Tech Lead's Proposal

### 4.1 No discussion of skill precedence implications

The tech lead's proposal never addresses what happens when skills share names across locations. Since personal skills override project skills (enterprise > personal > project), a user who creates `~/.claude/skills/implement/SKILL.md` would override every project's `/implement`. The tech lead does not analyze this because they believe personal skills do not exist (Section 2.1 above). My proposal (Section 5.3) identifies this as a significant scaffolding strategy concern.

### 4.2 No discussion of skill frontmatter capabilities

The tech lead never mentions that skills support `effort`, `model`, `hooks`, or `context` frontmatter. My proposal (Section 3.3, lines 187-189) identifies these as significant design levers:
- `effort: high` can enforce deep reasoning during `/implement` invocations (addresses Vector 1)
- `hooks` frontmatter allows skill-scoped hooks (e.g., read-before-edit only during `/implement`)
- `context: fork` enables the evaluator pattern as a skill rather than a global agent

These capabilities change the design space. A skill with `effort: high` partially mitigates the zero-thinking-tokens problem the tech lead uses to argue against CLAUDE.md rules. If skills can force `effort: high`, they become a stronger enforcement mechanism than the tech lead acknowledges.

### 4.3 No discussion of skills surviving compaction

My proposal (Section 3.3, line 192) notes that skills survive compaction with 5K tokens per skill (25K total budget). The tech lead does not mention this. This is relevant because the tech lead argues CLAUDE.md rules fail during long sessions (when compaction occurs). Skills are actually MORE durable than CLAUDE.md during long sessions, which strengthens the case for putting behavioral rules in skills rather than CLAUDE.md -- an argument neither of us makes explicitly.

### 4.4 Missing consideration of the Claude Code plugin system

My proposal (Section 9, question 6, line 528) raises whether Cerebro should become a Claude Code plugin. The plugin system packages skills, hooks, subagents, and MCP servers into installable units -- which is structurally what a "harness manager" does. The tech lead does not consider this alternative architecture. If Cerebro shipped as a plugin, the entire scaffolding system (`cerebro init`) would be replaced by plugin installation, and template management would be handled by the plugin infrastructure.

### 4.5 No CHANGELOG entry guidance

The tech lead's implementation plan (Section 6) does not mention CHANGELOG.md updates. The project conventions (CLAUDE.md) require: "Every PR must include a CHANGELOG.md entry in the same branch." This is a process gap, not a design gap, but it matters for the implementation handoff.

---

## 5. Gaps in MY Proposal That the Tech Lead Exposed

### 5.1 Missing settings baseline deployment

As acknowledged in Section 1.3 above, my Phase 0 skips the settings baseline entirely. The tech lead correctly identifies this as the actual lowest-effort, highest-impact change. Settings from `settings-matrix.md` (effort=high, showThinkingSummaries=true, compaction threshold) are already researched and require zero code or file creation.

### 5.2 The "60-70% value" claim is unmeasured

The tech lead (Section 4, line 275) correctly calls out that my Phase 0 value estimate ("delivers 60-70% of value") has no measurement behind it. I borrowed this framing from the original document and did not challenge it. The tech lead is right: until Phase 0 is deployed and measured, any percentage is a guess.

### 5.3 VSCode Stop hook incompatibility not sufficiently highlighted

The tech lead (Section 3.6, line 209) flags that Stop hooks do not fire in the VSCode extension (GitHub #40029). My proposal mentions this in passing (Appendix, line 573) but does not assess its impact on the Stop hook's value proposition. If the user uses the VSCode extension for some work, the Stop hook provides no protection there. The tech lead's "CLI-only" caveat (line 305) is a more honest framing.

### 5.4 The inline Stop hook is not production-quality

The tech lead's observation (Section 3.6, line 200-215) that the inline bash hook needs to live somewhere accessible, and the comparison of three options (inline, separate script, Go subcommand), is a more thorough treatment of the implementation question than my proposal provides. My proposal presents the inline version as the answer; the tech lead correctly identifies it as a stepping stone toward the Go subcommand.

---

## 6. The Biggest Open Disagreement

**The scope of what Cerebro should manage.**

The tech lead draws a hard boundary: Cerebro manages per-project state only (brain, project hooks, project skills, project CLAUDE.md section). Anything in `~/.claude/` is the user's domain. This means Cerebro should never scaffold evaluator agents, never scaffold personal skills, and never touch global settings.

My proposal draws the boundary differently: Cerebro's `init` command scaffolds project-level files (as it does today), but Phase 0 includes manual deployment of global files (evaluator agent, behavioral rules, Stop hook) that the user creates themselves. Cerebro-as-a-tool does not own global files, but Cerebro-as-a-project provides the templates and documentation for them.

The disagreement matters because it determines:
- Whether the `/implement` skill template should reference cerebro-specific features (recall, remember) or be generic
- Whether the Stop hook should be a `cerebro stop-guard` subcommand (ties the hook to the cerebro binary) or a standalone script (no cerebro dependency)
- Whether Cerebro's documentation should include global deployment guides or stay strictly per-project

**What would resolve it:** Measurement. Deploy the Phase 0 changes (which both proposals agree on in substance, differing only in scope) and measure whether the user invokes `cerebro init` in new projects to get the Stop hook and `/implement` skill. If yes, the per-project scaffolding approach is sufficient. If the user instead copies files to `~/.claude/` for global availability, that signals demand for a broader scope -- possibly the plugin architecture from Section 4.4 above.

The secondary resolution would be investigating the Claude Code plugin system. If Cerebro can ship as a plugin, the entire per-project vs global debate becomes moot: the plugin infrastructure handles distribution, and Cerebro's skills/hooks/agents are available wherever the plugin is enabled. This is the strategic question neither proposal fully addresses.
