# Claude Code Performance Degradation: Investigation & Mitigation

## Project Approach Document
**Version:** 1.2 — April 22, 2026
**Status:** Draft for review

---

## 1. Executive Summary

This project investigates a systemic pattern of reasoning quality degradation in the Claude Code ecosystem — not tied to any single model version, but recurring across releases. The pattern has been documented in Opus 4.5, 4.6, and community reports already indicate 4.7 exhibits similar issues despite Anthropic positioning it as a fix. The goal is to build evidence-based mitigations that live within the user's control and remain effective regardless of which model version is current.

The core tension is this: Anthropic controls the models, the thinking budgets, the effort defaults, the caching infrastructure, and the tokenizer. The user controls CLAUDE.md, hooks, settings, session management, and workflow design. When Anthropic makes upstream changes — whether acknowledged or not — the user's workflow degrades and the user has no recourse except to reverse-engineer what changed and compensate locally. This dynamic repeats with each release cycle, suggesting the problem is structural to the ecosystem rather than incidental to any one model.

This document defines the research workstreams, lays out the evidence landscape, proposes a mitigation strategy, and flags risks and open questions.

**Success criteria — how we know this project has delivered value:**

- A measurable improvement in autonomous task completion rate (fewer user interrupts per session)
- A documented settings/hooks baseline that can be version-controlled and restored after upstream changes
- An evidence-backed understanding of which degradation vectors affect *this user's* workflow specifically (not all will apply equally)
- A harness or hook set that provides early warning when quality regresses, rather than requiring manual discovery

---

## 2. Problem Statement

Users of Claude Code report that since approximately February 2026, the models have exhibited:

- **Reduced research-before-action behaviour** — editing files without reading them or their context first
- **Increased hallucination and assumption-based actions** — making claims without verification, fabricating technical details
- **Premature stopping and permission-seeking** — asking to stop, dodging responsibility, labelling work as "future work"
- **"Simplest fix" mentality** — gravitating toward the cheapest possible action rather than the correct one
- **Convention drift** — ignoring project-specific rules documented in CLAUDE.md
- **Apologetic self-awareness** — the model acknowledges its own poor output when challenged, suggesting it "knows" the right answer but didn't invest the reasoning to reach it

The practical impact is a shift from autonomous, trustworthy agent behaviour to a mode that requires continuous user supervision — eroding the productivity gains that justified the subscription cost.

---

## 3. Critical Analysis of the Evidence

### 3.1 The Flagship Report: Issue #42796

Filed on April 2, 2026 by Stella Laurenzo (Senior Director, AMD AI Group), this is the most rigorous public analysis of the degradation. It draws on 6,852 Claude Code session files, 17,871 thinking blocks, and 234,760 tool calls spanning January through March 2026.

**Key findings:**

- The read-to-edit ratio (file reads per file edit) dropped from 6.6 in the "good" period to 2.0 in the "degraded" period — a 70% reduction in research before action
- A programmatic stop-phrase-guard hook went from 0 violations (entire history before March 8) to 173 violations in 17 days after
- User interrupt rate increased 12x (0.9 to 11.4 per 1,000 tool calls)
- Estimated thinking depth dropped ~67% by late February, before thinking content was even redacted
- Full-file rewrites doubled (4.9% → 11.1% of mutations), replacing surgical edits

**Where the analysis is strong:**

- Quantitative, not anecdotal — multiple independent metrics converge on the same timeline
- Temporal precision: quality regression was independently noticed on March 8, the exact date redacted thinking blocks crossed 50%
- The signature-length proxy (0.971 Pearson correlation with thinking content length) is a clever method for estimating thinking depth even after redaction
- The stop-hook violation log is a machine-readable quality signal that could be replicated
- The behavioural catalog (Appendix A) is directly actionable — each pattern maps to a specific mitigation

**Where the analysis has gaps or risks:**

- **Single environment:** All data comes from one team's workflow (high-complexity systems programming with 50+ concurrent sessions). The regression may manifest differently in simpler workflows.
- **Confounding variable in cost data:** The 122x cost increase (February → March) conflates a *deliberate* scaling from 1–3 to 10+ concurrent sessions with degradation-induced thrashing. The report acknowledges this, but the headline number is misleading in isolation.
- **Correlation vs. causation:** Thinking depth and quality both declined, but they could share a common cause (e.g., infrastructure load) rather than one causing the other.
- **Anthropic disputes the core claim:** Boris Cherny (Claude Code lead) responded that `redact-thinking-2026-02-12` is a UI-only change that hides thinking output but does not reduce thinking depth or budget. If this is true, the signature-length proxy method may be measuring something other than effective reasoning depth.
- **No controlled A/B test:** The report doesn't include a comparison where thinking budget was explicitly set high via `MAX_THINKING_TOKENS` to see if that restores quality. This would be the most direct test of the hypothesis.
- **January data is incomplete:** The cost comparison's January baseline is asterisked as having only partial API data (sessions from Jan 9–31).

### 3.2 Broader Community Evidence

The #42796 report didn't exist in a vacuum. Multiple independent signals corroborate the pattern:

- **Issue #49244** (April 16, 2026): Reports Opus 4.6 regression starting ~April 15, describing behaviour as if "a different, less capable version" despite same model ID. User with 100+ skills and 90+ memory files reports clear quality drop.
- **Issue #46347**: Documents Claude fabricating technical claims to justify refusal — stating GitHub issues "don't exist" and that environment variables are "not real" without any verification tool calls. This is the hallucination-without-checking pattern at its worst.
- **Issue #40524 / ArkNill analysis repo**: The `claude-code-hidden-problem-analysis` repository documents prompt caching bugs causing 10–20x token inflation, with users having to reverse-engineer the Claude Code binary to find them.
- **Issue #18072**: Documents the conflict between `MAX_THINKING_TOKENS` as a safety rail and the `ultrathink` keyword being silently ignored when the env var is set.
- **Opus 4.7 community reports** (April 2026): Within days of 4.7's release, community sentiment shifted from "comprehensive upgrade" to "selective upgrade." Reports document degraded context retrieval, over-formatted outputs, up to 35% higher token costs from the new tokenizer (Anthropic documents a 1.0–1.35x increase depending on content type), and loss of user control levers (`budget_tokens` deprecated, `CLAUDE_CODE_DISABLE_ADAPTIVE_THINKING` unsupported). Latenode community threads report Max subscribers seeing significantly unreliable responses (note: this is a single community forum report, not independently verified). This confirms the ecosystem-level framing: the same degradation patterns reappear across model versions.
- **Marginlab Performance Tracker**: An independent third party running daily SWE-bench evaluations of Claude Code with the current model. Historical data showed Opus 4.5 degraded from 58% → 54% pass rate on SWE-Bench-Pro. Currently collecting baseline data for Opus 4.7. This is a key resource for ongoing monitoring.
- **VentureBeat reporting** (April 2026): Confirms Anthropic acknowledged adjusting 5-hour session limits during peak hours, with ~7% of Pro users hitting limits they wouldn't have before. Confirms Boris Cherny's dispute of the thinking-depth claim.
- **Incrypted investigation**: Reports that on February 9, 2026, adaptive thinking was forcibly enabled with Opus 4.6, and on March 3, 2026, the default effort level was changed to "medium."
- **BridgeBench hallucination benchmark** (April 12): BridgeMind posted that Claude Opus 4.6 fell from 83.3% accuracy to 68.3%. However, this claim has been **substantively debunked**: the original test used 6 tasks while the retest used 30 tasks (different scope). On the 6 overlapping tasks, accuracy barely changed (87.6% → 85.4%), well within normal variance. Computer scientist Paul Calcraft called it "incredibly bad science." The claim went viral but does not constitute reliable evidence of degradation. X community notes were added to the original post explaining the methodology flaw.

### 3.3 What Anthropic Has Acknowledged

Separating what Anthropic admits from what the community claims is important for building credible mitigations:

**Acknowledged:**
- Adaptive thinking was enabled by default with Opus 4.6 (Feb 9, 2026)
- Default effort level was changed to "medium" (March 3, 2026; since raised to "xhigh" for Opus 4.7). Boris Cherny described medium as "effort=85" internally, calling it "a sweet spot on the intelligence-latency/cost curve." He said it was changed in response to user feedback about excessive token usage, and was rolled out with a dialog for users to opt out.
- Prompt caching bugs caused unexpected token inflation (Issue #40524, described as "top priority")
- 5-hour session limits adjusted during peak hours, affecting ~7% of Pro users
- Thinking content display was redacted in the UI (Anthropic says this is UI-only, not depth-affecting)
- **Zero-thinking-tokens bug**: Boris Cherny confirmed on Hacker News (item 47668520) that adaptive thinking under-allocated reasoning on certain turns — allocating **zero** reasoning tokens even when effort=high was set. The specific turns where the model fabricated (Stripe API versions, git SHA suffixes, apt package names) had zero reasoning emitted. He said "we're investigating with the model team" and provided an interim workaround (`CLAUDE_CODE_DISABLE_ADAPTIVE_THINKING=1`). This workaround was subsequently deprecated for Opus 4.7.
- **September 2025 precedent**: Anthropic published a detailed postmortem acknowledging three overlapping infrastructure bugs that caused genuine quality degradation for weeks. Their own evaluations failed to detect what users were reporting. They stated: "We never reduce model quality due to demand, time of day, or server load."

**Disputed or unacknowledged:**
- That thinking depth (not just display) was systematically reduced (though the zero-thinking-tokens bug partially validates this concern)
- That the signature-length proxy reflects real thinking depth changes
- That load-sensitive thinking allocation exists (the time-of-day analysis in #42796 suggests it; Anthropic's September 2025 postmortem denies time-of-day/load-based quality reduction)

---

## 4. Root Cause Taxonomy

Based on the evidence, the degradation appears to stem from multiple independent vectors, not a single cause. This matters because each vector requires a different mitigation:

### Vector 1: Thinking Depth / Effort Level Changes
**What happened:** Adaptive thinking replaced fixed budgets; default effort has changed multiple times across versions (dropped to "medium" for 4.6, raised to "xhigh" for 4.7). Each release changes the defaults, and user-controlled overrides are progressively being deprecated (e.g., `budget_tokens` deprecated on 4.6, `CLAUDE_CODE_DISABLE_ADAPTIVE_THINKING` unsupported on 4.7).
**Confirmed mechanism:** Boris Cherny confirmed on Hacker News (item 47668520) that adaptive thinking's turn classifier can allocate **zero reasoning tokens** on certain turns, even when effort=high is set. The specific turns where the model fabricated (Stripe API versions, git SHA suffixes, apt package names) had zero reasoning emitted. Turns with deep reasoning were correct. This is not a theory — it's a confirmed bug acknowledged by Anthropic's Claude Code lead.
**Impact:** Model does less internal reasoning per step, leading to edit-without-reading and shallow planning. On zero-reasoning turns, the model hallucinates confidently. The specific manifestation varies by version, but the pattern — reduced reasoning without user consent — recurs.
**Mitigation path:** Explicit effort level control (`/effort high` or higher), `CLAUDE_CODE_DISABLE_ADAPTIVE_THINKING=1` (Opus 4.6 only — deprecated on 4.7), and monitoring thinking summaries to detect when upstream changes reduce depth. Note: sub-agent effort can be controlled via frontmatter in subagent definition files or via the `CLAUDE_CODE_EFFORT_LEVEL` environment variable (which overrides all other methods), but not via `/effort` directly.

### Vector 2: Prompt Caching Bugs
**What happened:** Two bugs in caching infrastructure caused cache misses, inflating token usage 10–20x.
**Impact:** Rapid context window exhaustion, premature compaction, and budget burn without quality gain.
**Mitigation path:** Claude Code version management (v2.1.90+ has partial fixes), monitoring token usage.

### Vector 3: Context Window Degradation
**What happened:** The 1M token context window was shipped, but quality degrades well before the limit — reports suggest problems at 20–40% usage. One user reported the model itself stated at 48% usage that it was "not being effective" and recommended starting a fresh session.
**Impact:** In long sessions, the model loses track of earlier context, instructions, and conventions.
**Mitigation path:** Session length management, aggressive `.claudeignore`, lean CLAUDE.md, manual compaction, use of worktrees for parallel short sessions, `CLAUDE_AUTOCOMPACT_PCT_OVERRIDE` to trigger compaction earlier, `CLAUDE_CODE_AUTO_COMPACT_WINDOW` to treat the window as smaller.

### Vector 4: Load-Sensitive Resource Allocation
**What happened:** Time-of-day analysis in #42796 suggests thinking depth became variable after redaction — consistent with a load-sensitive allocation system rather than a fixed budget. Anthropic denies reducing quality based on demand or time of day (September 2025 postmortem).
**Impact:** Inconsistent quality — same prompt may get deep or shallow reasoning depending on platform load.
**Mitigation path:** Limited — working off-peak (late night PST) shows some improvement, but this is unreliable. Explicit high effort settings may help override allocation decisions. The evidence here is correlational rather than causal.

### Vector 5: Behavioural Drift Across Model Updates
**What happened:** Each model version changes behaviour independent of thinking depth — instruction-following strictness, tool-use patterns, convention adherence, and even the tokenizer shift between versions. Community reports on 4.7 already document degraded context retrieval, over-formatted outputs, and up to 35% higher token costs from the new tokenizer, despite benchmark improvements in agentic coding.
**Impact:** CLAUDE.md instructions that worked on one version stop being followed on the next, with no configuration change on the user's side. Prompts must be re-tuned with each release.
**Mitigation path:** Hooks as enforcement (stop-phrase-guard, pre-tool-use validation), CLAUDE.md optimization, version-aware testing before upgrading.

### Vector 6: Systematic Removal of User Control Levers
**What happened:** With each release, Anthropic deprecates or removes settings that power users relied on. `budget_tokens` is deprecated on Opus 4.6+. `CLAUDE_CODE_DISABLE_ADAPTIVE_THINKING` doesn't work on 4.7. Fixed thinking budgets are being replaced by adaptive thinking that the user can't inspect or override. Thinking content is redacted.
**Impact:** The user's ability to compensate for upstream changes shrinks with each release. Mitigations built for one version may not be available on the next.
**Mitigation path:** This is the strongest argument for hooks and external harnesses rather than settings-based mitigations. Hooks operate outside the model's control plane and survive model transitions. The project should prioritise mitigations that are durable across versions over those that depend on version-specific levers.

---

## 5. Research Workstreams

### Workstream 1: Community Evidence Catalog
**Status: Substantially complete.**
**Deliverable:** `evidence-catalog.md` — timestamped timeline (25+ entries, Jan 30 – Apr 22), key issues index (10 issues), community repos, press coverage, six ecosystem patterns, primary source documentation (Boris Cherny HN thread, Anthropic September 2025 postmortem).
**Remaining:** Marginlab Opus 4.7 baseline tracking (ongoing).

### Workstream 2: Anthropic Documentation & Settings Audit
**Status: Complete (first pass).**
**Deliverable:** `settings-matrix.md` — 30+ quality-relevant settings organised by category, deployable baseline configuration, testing protocol.
**Remaining:** Could be deepened with context7 MCP server for documentation that web fetching truncated.

### Workstream 3: Harness & Hook Architecture
**Status: Not started.**
**Objective:** Design and document a defensive harness around Claude Code that enforces quality regardless of upstream model changes.
**Estimated effort:** 3–5 sessions (design), ongoing (implementation and tuning)

**Reference implementations:**
- `stellaraccident`'s stop-phrase-guard.sh (from #42796)
- `Chachamaru127/claude-code-harness` (Go-native guardrail engine, 13 rules)
- `affaan-m/everything-claude-code` (skills, instincts, memory, security scanning)
- `claudefa.st` Code Kit (Stop hooks, BiomeValidator, build-then-validate)
- Blake Crosley's 95-hook system

**Deliverable:** A modular hook/harness specification covering:
- **Stop hooks**: Prevent premature stopping, permission-seeking, ownership-dodging
- **PreToolUse hooks**: Enforce read-before-edit patterns, validate tool call rationale
- **Quality gates**: Run tests, lint, format checks before allowing session to complete
- **Context injection**: Automatically inject critical reminders at session start
- **Metrics collection**: Log read:edit ratios, stop-hook violations, thinking depth proxies per session

### Workstream 4: Personal Project Audit
**Status: Not started. Decision pending.**
**Objective:** Analyse the user's own Claude Code session logs for the same patterns documented in #42796. This grounds the project in first-hand evidence rather than relying solely on community reports.
**Estimated effort:** 2–4 sessions (depending on log volume)

**Prerequisites:** Access to `~/.claude/projects/` JSONL session files. If the user has been running Claude Code on active projects, these logs exist and contain the same fields (thinking blocks, tool calls, timestamps) that powered the #42796 analysis.

**Method:** Adapt the analysis methodology from #42796:
- Extract thinking blocks and tool calls from `~/.claude/projects/` JSONL files
- Calculate read:edit ratios by time period
- Measure user interrupts and frustration indicators
- Estimate thinking depth from signature field length

**Deliverable:** A project-specific degradation report with before/after comparison.

---

## 6. Mitigation Strategy

Mitigations are organised from lowest-effort to highest-effort. Each is independent — implement in any order.

### Tier 1: Settings & Configuration (Immediate)

| Setting | Recommended Value | Why |
|---------|------------------|-----|
| Effort level | `high` minimum; `xhigh` or `max` for complex tasks | Overrides any "medium" defaults that cause shallow reasoning. The specific levels available depend on the model version — check `/effort` for what's supported. |
| Model | Test current best option; don't assume latest = best | Community reports show 4.7 is not universally better than 4.6. The `opusplan` alias (Opus for planning, Sonnet for execution) may be more cost-effective. Evaluate against *your* workflow, not benchmarks. |
| `showThinkingSummaries` | `true` in settings.json | Makes thinking visible so you can monitor reasoning depth — essential for diagnosing whether upstream changes have affected your sessions |
| `CLAUDE_CODE_DISABLE_ADAPTIVE_THINKING` | `1` (test on current model) | Reverts to fixed thinking budget on models that support it. Not supported on Opus 4.7 — which itself is evidence of the ecosystem pattern: each release removes a mitigation lever from the previous one. |
| `MAX_THINKING_TOKENS` | Experiment: 10000–31999 | Sets explicit thinking budget (only meaningful when adaptive thinking is disabled). Being deprecated on newer models — another lever being removed. |
| `.claudeignore` | Add build artifacts, node_modules, large generated files | Reduces wasted tokens on irrelevant context. Model-agnostic; always beneficial. |
| CLAUDE.md | Focused on critical conventions; test different lengths | Loaded every turn — but the #42796 team used 5,000+ words successfully in the "good" period. The problem isn't size per se, it's whether the model has enough thinking budget to apply the conventions. Lean is good for cost; comprehensive is good if thinking depth supports it. Test both. |

### Tier 2: Hooks (Days)

Implement key hooks in `.claude/settings.json`:

- **Stop hook**: Block premature stopping with a phrase-matching guard (adapted from stellaraccident's approach)
- **PreToolUse:Write/Edit hook**: Warn or block file edits when the file hasn't been read in the current context
- **Session start hook**: Inject a brief "rules of engagement" reminder
- **Quality gate hook**: Run tests before allowing session completion

### Tier 3: Harness (Weeks)

Build or adopt a harness that wraps Claude Code with:

- Read:edit ratio monitoring per session
- Automatic session length management (compact or restart before context degrades)
- Token usage tracking and alerting
- A/B testing framework: same task run at different effort levels to measure quality delta
- Convention compliance checking (automated scan of output against CLAUDE.md rules)

### Tier 4: Workflow Adaptation (Ongoing)

- **Short sessions over long sessions**: Context quality degrades across all model versions; prefer many 15–20 minute focused sessions over multi-hour marathons
- **Plan mode first**: Use `--permission-mode plan` for analysis and architecture, then switch to execution
- **Worktrees for parallelism**: Use Git worktrees instead of concurrent agents in one session
- **Version-test before upgrading**: When a new model drops, run a standard set of tasks against both old and new before committing. Don't assume newer = better — the community evidence consistently shows otherwise
- **Monitor the Marginlab tracker**: Independent daily benchmarks at `marginlab.ai/trackers/claude-code/` provide early warning of regressions that Anthropic may not announce

---

## 7. Risks & Open Questions

### Risk: Confirmation bias
The project begins with the hypothesis "Claude has degraded." There is a real risk of selectively gathering evidence that confirms this while dismissing counter-evidence. **Mitigation:** The evidence catalog (Workstream 1) must include Anthropic's responses and any reports of quality *improving* after settings changes.

### Risk: Chasing a moving target
Anthropic ships updates frequently. Each release changes defaults, adds controls, removes others, and shifts the tokenizer. Mitigations built for one version's problems may be irrelevant or counterproductive on the next. **Mitigation:** Design mitigations to be model-agnostic where possible. The harness should detect *behavioural* regression (read:edit ratio, stop-hook violations, user interrupts) regardless of which model is running. Version-specific settings should be documented as such and reviewed on each upgrade.

### Risk: Over-engineering the harness
The stellaraccident team had 50+ concurrent agents, 30-minute autonomous runs, and 191,000 lines merged in a weekend. Most users don't operate at that scale. A 95-hook system is overhead for a single-developer workflow. **Mitigation:** Start with Tier 1 and Tier 2. Only build a full harness if simpler interventions prove insufficient.

### Risk: Conflicting mitigations
Increasing thinking budget increases cost. Reducing CLAUDE.md length may lose important conventions. Disabling adaptive thinking removes the model's ability to scale reasoning to task complexity. **Mitigation:** Test each change independently before combining. Track both quality and cost metrics.

### Risk: The "fix" cycle
Each new model release is positioned by Anthropic as addressing the previous version's problems (e.g., Opus 4.7's xhigh effort and stricter instruction-following). Community reports consistently show the same degradation patterns reappearing within days. This suggests the root causes are ecosystem-level (infrastructure, cost optimisation, default settings) rather than model-level. **Mitigation:** This is exactly why the project is framed at the ecosystem level. Mitigations should survive model transitions. If a new release genuinely fixes the problem, that's the ideal outcome — and the monitoring framework will confirm it.

### Open question: Is the signature-length proxy valid?
Anthropic says thinking content redaction doesn't affect thinking depth. The proxy method hasn't been validated externally. If it's measuring something other than effective reasoning, the entire "thinking depth decline" narrative may be wrong, and the real cause is elsewhere. **Partial answer:** The zero-thinking-tokens confirmation provides an alternative explanation — the model may think deeply on some turns and not at all on others, with the proxy capturing an average that reflects the mix of deep and zero-reasoning turns rather than a uniform reduction.

### Open question: Does explicit effort/thinking configuration actually override allocation?
Boris Cherny's zero-thinking-tokens confirmation partially answers this: the user was sending effort=high on every request, yet the model still allocated zero reasoning on some turns. This means effort=high is necessary but not sufficient to prevent zero-reasoning turns on models with adaptive thinking. The only confirmed workaround (disabling adaptive thinking entirely) has been deprecated on Opus 4.7.

---

## 8. Resource Index

### Primary Sources
| Resource | URL | What it provides |
|----------|-----|-----------------|
| Issue #42796 | `github.com/anthropics/claude-code/issues/42796` | Flagship quantitative degradation report |
| Boris Cherny HN thread | `news.ycombinator.com/item?id=47664442` | Primary source: Anthropic's response, zero-thinking-tokens confirmation (item 47668520) |
| Anthropic Sept 2025 postmortem | `anthropic.com/engineering/a-postmortem-of-three-recent-issues` | Three infrastructure bugs causing quality degradation; evals failing to detect user-reported issues |
| Issue #49244 | `github.com/anthropics/claude-code/issues/49244` | April 2026 Opus 4.6 regression report |
| Issue #46347 | `github.com/anthropics/claude-code/issues/46347` | Fabrication-without-verification report |
| Cache bug analysis | `github.com/ArkNill/claude-code-hidden-problem-analysis` | 10–20x token inflation documentation |
| Marginlab Tracker | `marginlab.ai/trackers/claude-code/` | Independent daily SWE-bench performance tracking |
| Claude Code docs | `code.claude.com/docs/en/` | Official configuration, workflows, settings |
| API docs | `platform.claude.com/docs/en/` | Extended thinking, effort, adaptive thinking, migration |

### Community Tools & Harnesses
| Resource | URL | What it provides |
|----------|-----|-----------------|
| claude-code-harness | `github.com/Chachamaru127/claude-code-harness` | Go-native guardrail engine (13 rules) |
| everything-claude-code | `github.com/affaan-m/everything-claude-code` | Skills, instincts, memory, security scanning |
| claude-code-best-practice | `github.com/shanraisshan/claude-code-best-practice` | Settings reference, configuration guide |
| Code Kit (claudefa.st) | `claudefa.st` | Hooks guide, stop hook patterns, changelog |

### Press & Analysis
| Resource | URL | Key insight |
|----------|-----|------------|
| VentureBeat | `venturebeat.com/.../is-anthropic-nerfing-claude...` | Anthropic's official response, session limit changes |
| Incrypted | `incrypted.com/en/what-happening-claude/` | Timeline of adaptive thinking and effort changes |
| Axios | `axios.com/2026/04/16/anthropic-claude-opus-model-mythos` | Anthropic denies compute redirection to Mythos |
| Token optimization guide | `buildtolaunch.substack.com/p/claude-code-token-optimization` | Practical CLAUDE.md and session cost management |

---

## 9. Next Steps

1. **Immediate**: Apply Tier 1 settings changes (effort, model, thinking summaries) and observe
2. **This week**: Design Tier 2 hooks based on patterns from stellaraccident's stop-phrase-guard and community implementations
3. **Next week**: Decide whether Workstream 4 (personal audit) is worth the effort based on whether Tier 1+2 resolve the user's specific issues
4. **Ongoing**: Monitor Marginlab tracker for Opus 4.7 baseline data

---

## Appendix: Document Maintenance

This document serves as the project's single source of truth. Update it as new evidence emerges or mitigations are tested. Key fields to maintain:

- **Evidence catalog** (Workstream 1 deliverable — `evidence-catalog.md`, substantially complete)
- **Settings matrix** (Workstream 2 deliverable — `settings-matrix.md`, complete first pass)
- **Hook specification** (Workstream 3 deliverable — not yet started)
- **Version log**: Track what changed in this document and when

| Date | Change | Reason |
|------|--------|--------|
| 2026-04-22 | v1.0 created | Initial project scoping and analysis |
| 2026-04-22 | v1.1 revised | Reframed from model-specific to ecosystem-level; added 4.7 community evidence; added Vector 6 (lever removal); corrected CLAUDE.md advice; added success criteria; added Marginlab tracker; added effort estimates to workstreams |
| 2026-04-22 | v1.2 revised | Comprehensive update: added confirmed zero-thinking-tokens mechanism to Section 3.3 and Vector 1; corrected BridgeBench claim (debunked methodology — 6 vs 30 tasks, overlapping results nearly identical); added September 2025 postmortem as historical precedent; corrected "35% higher tokens" to "up to 35%"; qualified Latenode claim as single-source; added sub-agent effort control via frontmatter/env var; updated open questions with partial answers from new evidence; added Boris Cherny HN thread and Anthropic postmortem to resource index; updated workstream statuses; rewrote next steps to reflect current state |
