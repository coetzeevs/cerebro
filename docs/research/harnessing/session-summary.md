# Session Summary: Claude Code Ecosystem Degradation Investigation

**Date:** April 22, 2026
**Session scope:** Project inception, research, evidence gathering, document creation, independent review

---

## 1. What We Did

This session established a research project investigating systemic reasoning quality degradation in the Claude Code ecosystem. Over the course of the session we produced three interconnected deliverables, iterated them through multiple rounds of research and critical review, and built an evidence base grounded in primary sources.

### Deliverables produced

1. **approach.md** (v1.2) — The project's single source of truth. Defines the problem, critically analyses the flagship evidence (#42796), taxonomises six root cause vectors, lays out four research workstreams with completion status, proposes a four-tier mitigation strategy, and documents risks and open questions. Updated three times during the session as new evidence was gathered and errors were caught.

2. **evidence-catalog.md** — A timestamped evidence catalog with 25+ timeline entries spanning January 30 to April 22, 2026. Indexes 10 GitHub issues, 4 community analysis repos, 16+ press/analysis sources, and documents 6 ecosystem-level patterns. Includes full primary-source documentation of the Boris Cherny Hacker News thread and the Anthropic September 2025 infrastructure postmortem.

3. **settings-matrix.md** — A settings audit covering 30+ quality-relevant configuration levers, organised by category (thinking/reasoning, context/compaction, model routing, session management, hooks, monitoring). Includes a deployable baseline configuration, a testing protocol, and evidence-backed notes on the limitations of settings-only mitigations.

### Research scope

The research drew on the following source categories, in order of trustworthiness applied:

- **Primary sources**: Anthropic's official documentation (code.claude.com, platform.claude.com), Anthropic's September 2025 postmortem, Boris Cherny's Hacker News comments (retrieved from primary URL), the claude.com/pricing page
- **High-quality secondary sources**: GitHub issue #42796 (full issue body retrieved), VentureBeat, Axios, The Register, Incrypted
- **Community sources**: GitHub issues (#49244, #46347, #40524, #38335, #18072, #46949, #46099, #46212, #3511), community repos (ArkNill, Chachamaru127, affaan-m, shanraisshan), Substacks (dgtldept, boringbot, buildtolaunch), Marginlab tracker, r/ClaudeAI weekly performance reports
- **Sources treated with caution**: Latenode forum (single user report, flagged as unverified), BridgeBench (claim debunked — methodology flaw), pasqualepillitteri.it (Claude Code Pro plan claim contradicted by primary source, later found to be a developing situation with conflicting signals)

---

## 2. How We Built Everything Up

### Phase 1: Scoping
The user provided a project brief describing the degradation problem and two source URLs (the claude-code repo and issue #42796). We attempted to retrieve the specific comment linked (`issuecomment-4194007103`) but GitHub's page rendering was too long for automated retrieval. We retrieved the full issue body instead, which contained the complete quantitative analysis.

### Phase 2: Approach document (v1.0)
We researched the broader ecosystem — community evidence, Anthropic's official documentation, settings references, hook architectures, and press coverage. The first approach document was drafted, then immediately self-reviewed. The review identified missing success criteria, an incorrect CLAUDE.md recommendation (the "under 500 tokens" advice contradicted the #42796 evidence), and the need for effort estimates on workstreams. These were corrected in v1.1.

### Phase 3: Ecosystem reframing (v1.1)
The user correctly identified that the document was too model-specific. We researched Opus 4.7 community reports and found the same degradation patterns recurring. The document was reframed to be model-agnostic, treating the problem as ecosystem-level. A new root cause vector was added (Vector 6: Systematic Removal of User Control Levers).

### Phase 4: Parallel workstream execution
Both the evidence catalog (Workstream 1) and settings matrix (Workstream 2) were built in a single pass, drawing on all accumulated research.

### Phase 5: Open items resolution
Five open items were identified in the evidence catalog. We systematically tackled each:

- **Boris Cherny HN thread** — Retrieved from primary source. This yielded the session's most important finding: Cherny confirmed that adaptive thinking allocates zero reasoning tokens on certain turns, directly causing hallucinations, even when effort=high is set.
- **Additional GitHub issues** — Four more issues indexed (#46949, #46099, #46212, #3511) from The Register analysis.
- **Marginlab tracker** — Historical data retrieved (Opus 4.5: 58%→54% pass rate). Currently collecting 4.7 baseline.
- **Reddit r/ClaudeAI** — Weekly performance reports found with ~70% negative sentiment.
- **Anthropic's VentureBeat response** — Spokesperson did not answer specific questions about reasoning defaults, throttling, or benchmarks. Referred reporters to X posts.

### Phase 6: Verification failure and correction
A claim from pasqualepillitteri.it that Claude Code was removed from the Pro plan was initially included, then removed after checking claude.com/pricing showed it still included. The user then provided a Hacker News thread revealing the situation was more complex — Anthropic's own pages contradicted each other (comparison matrix changed, support articles changed, feature bullets and product page hadn't). This became a methodological lesson: when sources conflict, present the conflict rather than picking a side.

### Phase 7: Independent review and comprehensive update (v1.2)
The approach document, evidence catalog, and settings matrix were all reviewed against primary sources. Corrections made:

- **BridgeBench claim debunked**: The 83.3%→68.3% accuracy drop was methodologically flawed (6 tasks vs 30 tasks; overlapping results nearly identical at 87.6% vs 85.4%). Paul Calcraft called it "incredibly bad science."
- **"35% higher token costs" corrected**: Anthropic documents 1.0–1.35x, meaning *up to* 35%, not a flat 35%.
- **Latenode claim qualified**: Single community forum post, not independently verified.
- **Sub-agent effort gap corrected**: Effort *can* be set per-subagent via frontmatter or globally via env var, contradicting the HN claim that sub-agent effort can't be controlled.
- **Section 3.3 expanded**: Zero-thinking-tokens confirmation added to "What Anthropic Has Acknowledged."
- **Open questions partially answered**: Both now reference the Cherny confirmation as new evidence.
- **Settings matrix updated**: Effort level and adaptive thinking rows updated with evidence-backed limitations, hook section updated with #42796 metrics, baseline configuration carries a limitations note.

---

## 3. Key Findings

### Finding 1: Confirmed mechanism — zero-thinking-tokens bug
Boris Cherny (Claude Code lead) confirmed on Hacker News that adaptive thinking's turn classifier allocates **zero reasoning tokens** on certain turns, even when effort=high is set. The specific turns where the model fabricated (Stripe API versions, git SHA suffixes, apt package names) had zero reasoning emitted. Turns with deep reasoning were correct. The interim workaround (`CLAUDE_CODE_DISABLE_ADAPTIVE_THINKING=1`) was subsequently deprecated for Opus 4.7. This is the single strongest evidence point — a confirmed mechanism from Anthropic's own team lead, with the fix being removed.

### Finding 2: The degradation is ecosystem-level, not model-specific
The same behavioural patterns (premature stopping, edit-without-reading, hallucination, convention drift) recur across Opus 4.5, 4.6, and 4.7. Each release is positioned as a fix but the patterns reappear within days. The root causes are infrastructure, default settings, and commercial pressure — not model weights.

### Finding 3: User control levers are being systematically removed
`budget_tokens` deprecated on 4.6+. `CLAUDE_CODE_DISABLE_ADAPTIVE_THINKING` unsupported on 4.7. Thinking content redacted. Fixed budgets replaced by adaptive thinking the user can't inspect. Each release removes a lever that power users depended on for the previous version.

### Finding 4: Settings are necessary but not sufficient
Effort=high is necessary but doesn't prevent zero-thinking turns. The 1M context window degrades quality at 20% usage. CLAUDE.md conventions are ignored when thinking depth is insufficient to apply them. The settings baseline in the matrix is a floor, not a solution.

### Finding 5: Hooks are the most durable mitigation
Hooks operate outside the model's control plane and survive model transitions. The stop-phrase-guard from #42796 (0 violations → 173 in 17 days) is a proven quality canary. The read-before-edit enforcer addresses the core read:edit ratio collapse (6.6 → 2.0). These are the Tier 2 mitigations to design next.

### Finding 6: Anthropic's September 2025 postmortem establishes precedent
Three overlapping infrastructure bugs caused genuine quality degradation. Anthropic's own evaluations failed to detect what users were reporting. They stated they "never reduce model quality due to demand, time of day, or server load." The current situation exhibits the same characteristics: overlapping causes, inconsistent symptoms, user reports preceding official acknowledgement.

---

## 4. Guidelines and Guardrails Used

### Research methodology
- **Primary sources first**: Always verify claims against official documentation or primary source URLs before including them.
- **Present conflicts, don't resolve them**: When sources contradict each other (e.g., the pricing page situation), document the conflict rather than picking a side.
- **Flag source quality**: Single community forum posts are not equivalent to quantitative analyses or official documentation. Mark unverified claims as such.
- **Follow evidence threads**: When a claim appears in a secondary source, trace it back to the primary source before asserting it as fact.
- **Say "developing" when developing**: When a situation is in flux, say so explicitly rather than asserting a conclusion.
- **Never include unverified claims that fit the narrative**: This is confirmation bias. The BridgeBench debunking and the pricing page incident are concrete examples of where this discipline was tested.

### Document standards
- Each claim in the evidence catalog links to a named source.
- The "What Anthropic Has Acknowledged" vs "Disputed or unacknowledged" separation prevents conflating community claims with confirmed facts.
- The approach document includes a "where the analysis has gaps or risks" section for the flagship #42796 report — applying the same critical lens to the strongest evidence in the project.
- The settings matrix marks each setting with quality impact, durability across model versions, and evidence-backed notes.

---

## 5. Agent Handoff: Session Primer

The following section is intended as input for an independent AI agent session that will continue this work. The agent will have `approach.md`, `evidence-catalog.md`, and `settings-matrix.md` as context.

### Project state

Workstreams 1 (evidence catalog) and 2 (settings audit) are substantially complete. The next action is **Workstream 3: Tier 2 hooks design** — specifically the stop-phrase-guard and read-before-edit enforcer. Workstream 4 (personal project audit) is deferred pending the user's decision on whether Tier 1+2 resolve their specific issues.

### What the agent should do next

Design the Tier 2 hooks described in approach.md Section 6. The two highest-priority hooks are:

1. **Stop hook (stop-phrase-guard)**: Matches phrases indicating premature stopping, permission-seeking, or ownership-dodging. Exit code 1 blocks the stop and forces continuation. Reference implementations: `stellaraccident`'s stop-phrase-guard.sh from #42796 (matched 30+ phrases across 5 categories), `claudefa.st` Code Kit stop hook patterns, `Chachamaru127/claude-code-harness` guardrail engine.

2. **PreToolUse:Write/Edit hook (read-before-edit enforcer)**: Tracks which files have been read in the current context. When the model attempts to write or edit a file it hasn't recently read, the hook warns or blocks. This directly addresses the read:edit ratio collapse documented in #42796 (6.6 → 2.0). Implementation requires state tracking across tool calls within a session.

The agent should also consider: PostToolUse logging hook (for read:edit ratio monitoring), SessionStart context injection, and quality gate hooks (run tests before session completion).

### What the agent must know

- The project is ecosystem-level, not model-specific. Don't design hooks that only work on one model version.
- Boris Cherny confirmed that adaptive thinking can allocate zero reasoning tokens even at effort=high. Settings alone don't solve this. Hooks catch the *consequences* externally.
- Sub-agent effort can be controlled via frontmatter or the `CLAUDE_CODE_EFFORT_LEVEL` env var, but not via `/effort` directly.
- The September 2025 postmortem shows Anthropic's infrastructure can and does cause real degradation independent of model weights, and that their own evaluations can fail to detect it.
- The user values rigour. Claims must be verified against primary sources. When sources conflict, present the conflict. Don't include unverified claims that fit the narrative.
- The settings matrix baseline configuration is ready to deploy but carries a documented limitation: it is necessary but not sufficient on models with adaptive reasoning.

### Documents to reference

- **approach.md**: Section 5 (Workstream 3 specification), Section 6 (Tier 2 hooks), Section 4 Vector 1 (confirmed mechanism), Vector 6 (lever removal rationale for hooks-first approach)
- **evidence-catalog.md**: Boris Cherny HN thread section (primary source for zero-thinking-tokens confirmation), #42796 Appendix A behavioural catalog (the specific patterns hooks should catch), #42796 Appendix B stop-hook violation data (the proof that the stop-phrase-guard works)
- **settings-matrix.md**: Section 5 (hook configuration table with evidence-backed priorities), Section 8 baseline (the settings foundation hooks build on)
