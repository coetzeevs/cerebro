# Claude Code Ecosystem Degradation: Evidence Catalog

**Companion to:** approach.md
**Last updated:** April 22, 2026

---

## Timeline of Changes & Regressions

This table maps upstream changes to observed symptoms, community sources, and Anthropic's response. Dates are approximate where not precisely documented.

| Date | Change / Event | Source | Symptoms Observed | Anthropic Response |
|------|---------------|--------|-------------------|-------------------|
| **Jan 30 – Feb 8** | Baseline "good" period | #42796 data | Read:edit ratio 6.6, 0 stop-hook violations, 0.9 user interrupts/1K tool calls | N/A |
| **Feb 9, 2026** | Adaptive thinking forcibly enabled with Opus 4.6 release | Incrypted investigation | Fixed thinking budgets replaced by model-controlled adaptive reasoning. Users lost ability to guarantee reasoning depth. | Not directly acknowledged as a cause of degradation |
| **Mid-Feb 2026** | Estimated thinking depth drops ~67% from baseline | #42796 signature analysis | Read:edit ratio drops to 2.8, "simplest fix" language increases, convention drift begins | Not acknowledged |
| **Late Feb 2026** | Prompt caching bugs begin | #40524, ArkNill repo | 10–20x token inflation, rapid context exhaustion, unexpected billing spikes | Acknowledged as "top priority" (Boris Cherny) |
| **Mar 3, 2026** | Default effort level changed to "medium" | Incrypted, official docs | Shallow reasoning on complex tasks unless user explicitly overrides effort | Confirmed in docs. Later changed back. |
| **Mar 5–12** | Thinking content redaction rolled out (staged: 1.5% → 100%) | #42796 JSONL analysis | Thinking depth becomes invisible to users. `redact-thinking-2026-02-12` header appears. | Boris Cherny: "UI-only change" that "does not impact thinking itself" |
| **Mar 8, 2026** | Quality regression independently noticed by stellaraccident team | #42796 | Stop-hook goes from 0 to 8 violations on this date. Redacted thinking crosses 50%. | See above |
| **Mar 8–25** | Degraded period measured | #42796 | 173 stop-hook violations in 17 days. Read:edit ratio 2.0. User interrupts 5.9/1K tool calls. Reasoning loops triple. Full-file rewrites double. | Not directly addressed |
| **Mar 18, 2026** | Peak degradation day | #42796 Appendix B | 43 stop-hook violations in one day (~1 every 20 minutes across active sessions) | Not addressed |
| **Mar 26, 2026** | 5-hour session limits adjusted during peak hours | Thariq Shihipar (Anthropic) post | ~7% of Pro users hit limits they wouldn't have hit before. Weekday 5am–11am PT affected most. | Acknowledged. Said Team/Enterprise not affected. |
| **Mar 31 – Apr 7** | Token burn bug (prompt-cache attestation) | #49244, ArkNill repo | Partially fixed in v2.1.90 | Acknowledged |
| **Apr 2, 2026** | Issue #42796 filed by Stella Laurenzo | GitHub | Flagship quantitative degradation report published. 6,852 sessions, 234,760 tool calls analyzed. | Boris Cherny thanked the analysis, disputed thinking-depth conclusion |
| **Apr 4, 2026** | Anthropic blocks third-party tools from using Claude subscriptions | VentureBeat | OpenClaw and similar tools can no longer use Claude subscriptions. Cherny: "Capacity is a resource we manage thoughtfully." | Announced by Boris Cherny on X. Cited compute strain. |
| **~Apr 6, 2026** | Boris Cherny confirms zero-thinking-tokens bug on Hacker News | HN thread, pasqualepillitteri.it | **Anthropic's team lead confirmed that adaptive thinking allocated ZERO reasoning tokens on certain turns**, directly causing hallucinations (fabricated commit SHAs, non-existent apt packages, fake API versions). Cherny: "The specific turns where it fabricated had zero reasoning emitted, while the turns with deep reasoning were correct." Provided interim workaround: `CLAUDE_CODE_DISABLE_ADAPTIVE_THINKING=1` | **Confirmed.** This is the single strongest evidence point — Anthropic's own team lead acknowledged the mechanism causing hallucination. |
| **Apr 7, 2026** | Om Patel's viral X post summarizing "67% drop" | VentureBeat | "AI shrinkflation" label enters mainstream AI discourse | Not directly addressed |
| **Apr 11, 2026** | Thariq Shihipar denies deliberate degradation | X post | Stated Anthropic does not "degrade" models to serve demand. Said thinking summary changes affected how users measured "thinking." | Denied. Said no evidence backing the strongest qualitative claims. |
| **Apr 12, 2026** | BridgeBench hallucination benchmark claim goes viral | BridgeMind X post, VentureBeat, BeInCrypto | BridgeMind posted that Opus 4.6 fell from 83.3% to 68.3% accuracy. **However, this has been debunked**: original test used 6 tasks, retest used 30 (different scope). On the 6 overlapping tasks, accuracy barely changed (87.6% → 85.4%). Paul Calcraft called it "incredibly bad science." X community notes were added. | Not addressed, but the claim itself is methodologically flawed |
| **Apr ~13** | Anthropic spokesperson responds to VentureBeat inquiry | VentureBeat, TechBooky | **Spokesperson did not address questions about reasoning defaults, throttling, or benchmark claims.** Referred reporters to Boris Cherny and Thariq Shihipar's X posts. | Non-response. Questions about reasoning defaults, context handling, throttling, and inference parameters were not answered. |
| **~Apr 15, 2026** | 3-hour outage + quality regression noticed | #49244 | Login failures, usage limit glitches, API errors. Model behaviour "significantly degraded" even in fresh short sessions post-outage. | Outage acknowledged; quality regression not specifically addressed |
| **Apr 16, 2026** | Opus 4.7 released | anthropic.com/news | xhigh effort default, stricter instruction-following, new tokenizer (1.0–1.35x more tokens), `budget_tokens` deprecated, `CLAUDE_CODE_DISABLE_ADAPTIVE_THINKING` no longer supported | Positioned as addressing community concerns |
| **Apr 16–18** | Opus 4.7 community backlash begins | Reddit, HN, X, Apiyi, Medium | Degraded context retrieval, over-formatted outputs, up to 35% higher token costs (Anthropic documents 1.0–1.35x tokenizer increase), loss of user control levers. Community reports describe it as "upgrade for coding agents, downgrade for everything else" | Not yet addressed at time of writing |
| **Apr 20, 2026** | Marginlab independent performance tracker collecting 4.7 baseline | marginlab.ai | Daily SWE-bench evaluations of Claude Code. Historical data shows Opus 4.5 degraded from 58% → 54% pass rate before 4.6 transition. Currently paused for 4.7 baseline collection. | N/A |
| **Apr 22, 2026** | Claude Code appears to be removed from Pro plan (developing) | HN thread 47854477, support article diffs, pricing comparison matrix | **Situation is contradictory across Anthropic's own pages.** The pricing comparison matrix at claude.com/pricing now shows Claude Code as unavailable on Pro. Support articles have been updated from "Pro or Max" to just "Max." However, the feature bullets on the same pricing page and the product page (claude.com/product/claude-code) still list it as included with Pro. Anthropic's chatbot still says Pro includes it. No official announcement. Users on HN report the page changing between refreshes. | **No announcement.** Conflicting signals across Anthropic's own pages suggest a rollout in progress. |

---

## Key Issues Index

| Issue | Title | Filed | Status | Key Contribution |
|-------|-------|-------|--------|-----------------|
| [#42796](https://github.com/anthropics/claude-code/issues/42796) | Claude Code unusable for complex engineering (Feb updates) | Apr 2 | Closed | Flagship quantitative analysis: 6,852 sessions, thinking depth proxy, read:edit ratio, stop-hook data |
| [#49244](https://github.com/anthropics/claude-code/issues/49244) | Opus 4.6 regression ~April 15 | Apr 16 | Open | Documents post-outage regression with 100+ skills/90+ memory files user |
| [#46347](https://github.com/anthropics/claude-code/issues/46347) | Fabricates false technical claims to justify refusal | Apr 2026 | Open | Documents hallucination-without-verification: model claims issues "don't exist" without searching |
| [#40524](https://github.com/anthropics/claude-code/issues/40524) | Prompt caching bugs | Mar 2026 | Partially fixed | 10–20x token inflation. Users reverse-engineered binary to find cause. |
| [#38335](https://github.com/anthropics/claude-code/issues/38335) | Related caching/performance issue | Mar 2026 | Open | 478 comments, 15 days without Anthropic response (per ArkNill repo) |
| [#18072](https://github.com/anthropics/claude-code/issues/18072) | MAX_THINKING_TOKENS vs ultrathink conflict | Jan 2026 | Open | Documents silently ignored user settings — a pattern of lever removal |
| [#46949](https://github.com/anthropics/claude-code/issues/46949) | Artificial degradation, acquisition bias, unacceptable throttling | Apr 2026 | Open | Referenced by The Register analysis. Covers throttling and bias claims. |
| [#46099](https://github.com/anthropics/claude-code/issues/46099) | Opus 4.6: Severe quality degradation on iterative coding tasks | Apr 2026 | Open | Referenced by The Register analysis. Iterative coding specifically affected. |
| [#46212](https://github.com/anthropics/claude-code/issues/46212) | Claude Code's prediction-first behaviour dangerous on capital-at-risk projects | Apr 2026 | Open | Safety concern: model acts on predictions rather than verified facts. |
| [#3511](https://github.com/anthropics/claude-code/issues/3511) | API usage limit unexpectedly reduced on MAX plan | Jul 2025 | Closed | **Historical precedent**: Same pattern of silent limit changes, user backlash, eventual acknowledgement. |

### Historical Precedent: September 2025 Infrastructure Bugs

In September 2025, Anthropic published a detailed postmortem (anthropic.com/engineering/a-postmortem-of-three-recent-issues) acknowledging three overlapping infrastructure bugs that intermittently degraded Claude's response quality between August and September 2025:

1. **Context window routing error** (Aug 5 onwards): Sonnet 4 requests were misrouted to servers configured for 1M context windows. Initially affected 0.8% of requests; peaked at 16% of Sonnet 4 requests after a load balancing change on Aug 29. Routing was "sticky" — once a user was misrouted, subsequent requests followed.

2. **Output corruption** (Aug 25–Sep 2): A TPU misconfiguration caused token generation errors — Thai/Chinese characters appearing in English responses, obvious syntax errors in code. Affected Opus 4.1, Opus 4, and Sonnet 4 on Claude API only.

3. **XLA compiler bug** (Aug 25 onwards): A compiler bug caused the approximate top-k operation to sometimes drop the *highest probability token* entirely. The bug was inconsistent — same prompt might work perfectly on one request and fail on the next. Root cause: mixed-precision arithmetic (bf16 vs fp32) in distributed sorting across TPU chips.

**Key statements from the postmortem:**
- "We never reduce model quality due to demand, time of day, or server load. The problems our users reported were due to infrastructure bugs alone."
- "The evaluations we ran simply didn't capture the degradation users were reporting."
- "Each bug produced different symptoms on different platforms at different rates. This created a confusing mix of reports that didn't point to any single cause."

**Why this matters for the current investigation:** It establishes that (a) Anthropic's infrastructure *can and does* cause real quality degradation independent of model weights, (b) multiple overlapping bugs make diagnosis extremely difficult, (c) Anthropic's own evaluations can fail to detect degradation that users experience, and (d) user complaints about quality decline have been validated before. The current 2026 situation exhibits similar characteristics: overlapping causes, inconsistent symptoms, user reports preceding official acknowledgement.

---

## Primary Source: Boris Cherny HN Thread (Apr ~14, 2026)

**Source:** news.ycombinator.com/item?id=47664442 (bcherny's comment) and item?id=47668520 (zero-thinking-tokens confirmation)

This is the primary source for Anthropic's most significant acknowledgement in the current degradation. Boris Cherny (Claude Code lead) posted in response to the viral #42796 issue discussion on HN.

**Cherny's main response** (item 47664442):
- Stated `redact-thinking-2026-02-12` is UI-only — hides thinking from the interface but does not impact thinking depth or budgets
- Acknowledged two changes: (1) Opus 4.6 → adaptive thinking default on Feb 9; (2) effort default changed to medium (effort=85) on Mar 3
- Described effort=85 as "a sweet spot on the intelligence-latency/cost curve for most users"
- Said the effort change was rolled out with a dialog for users to opt out
- Recommended `/effort high` or `settings.json` for users wanting deeper reasoning
- Said they would "test defaulting Teams and Enterprise users to high effort"

**The zero-thinking-tokens confirmation** (item 47668520):
After a user submitted 5 feedback IDs via `/bug`, Cherny read all 5 transcripts and responded:
- Confirmed the sessions were sending effort=high on every request (verified in telemetry) — **meaning the effort→medium default was not the cause in this case**
- Stated: "The data points at adaptive thinking under-allocating reasoning on certain turns"
- Stated: "the specific turns where it fabricated (stripe API version, git SHA suffix, apt package list) had zero reasoning emitted, while the turns with deep reasoning were correct"
- Said: "we're investigating with the model team"
- Provided workaround: `CLAUDE_CODE_DISABLE_ADAPTIVE_THINKING=1` forces a fixed reasoning budget

**Why this is the most important finding:** This confirms that even with effort=high, adaptive thinking's turn classifier can route complex tasks as simple and allocate *zero* reasoning tokens. The hallucinations are not from the model being "dumb" — they're from the model literally not thinking at all on certain turns. The workaround (disabling adaptive thinking) was then deprecated for Opus 4.7.

**Notable community responses in the thread:**
- `koverstreet`: "even on high effort there's been a very significant increase in 'rush to completion' behavior"
- `richardjennings`: "I was not aware the default effort had changed to medium until the quality of output nosedived. This cost me perhaps a day of work to rectify."
- `richardjennings`: "You cannot control the effort setting sub-agents use" — a gap in the mitigation surface
- `murkt`: Referenced a leaked/extracted system prompt gist suggesting Claude Code's internal prompt pushes toward "simple" solutions at a 5:1 ratio
- `stefan_`: Reported thinking content leaking into code patches — the model inserting reasoning text directly into source code

---

## Community Analysis Repos

| Repo | Author | What it provides |
|------|--------|-----------------|
| [claude-code-hidden-problem-analysis](https://github.com/ArkNill/claude-code-hidden-problem-analysis) | ArkNill | Cache bug documentation, token inflation measurement, 11 bugs cataloged (B1–B11), fix guides |
| [everything-claude-code](https://github.com/affaan-m/everything-claude-code) | affaan-m | Agent harness optimization: skills, instincts, memory, security scanning (agentshield) |
| [claude-code-harness](https://github.com/Chachamaru127/claude-code-harness) | Chachamaru127 | Go-native guardrail engine: 13 rules, plan→work→review cycle, sub-10ms hook response |
| [claude-code-best-practice](https://github.com/shanraisshan/claude-code-best-practice) | shanraisshan | Settings reference, configuration guide, environment variable documentation |

---

## Press Coverage

| Source | Date | Title/Topic | Key Insight |
|--------|------|-------------|------------|
| VentureBeat | Apr 13 | "Is Anthropic 'nerfing' Claude?" | Anthropic spokesperson **did not answer questions** about reasoning defaults, throttling, or benchmarks. Referred to X posts only. |
| VentureBeat | Apr 4 | Anthropic cuts off third-party Claude subscription use | OpenClaw crackdown. Cherny: "Capacity is a resource we manage thoughtfully." |
| Incrypted | Apr 2026 | "Is Claude Breaking?" | Timeline: adaptive thinking (Feb 9), effort → medium (Mar 3). Boris Cherny confirmed both changes. |
| Axios | Apr 16 | Opus 4.7 release + Mythos | Anthropic denied compute redirection to Mythos. "Nerfing" speculation documented. |
| The Register | Apr 13 | "Claude is getting worse, according to Claude" | Used Claude itself to analyze quality complaints in the repo. Notes Anthropic's auto-close script may mask unresolved issues. |
| Fortune | Apr 14 | Claude Code performance controversy | First mainstream coverage of the effort→medium change and user backlash. |
| Gizmodo | Apr 2026 | "Anthropic Is Jacking Up the Price for Power Users" | Framed as simultaneous quality decline + pricing increase. |
| The Decoder | Sep 2025 | Anthropic confirms technical bugs | **Historical**: Anthropic confirmed two bugs in Sonnet 4/Haiku 3.5, investigating Opus 4.1. Precedent for acknowledged infrastructure-caused degradation. |
| Medium (Vibe Coding) | Apr 20 | "Opus 4.7 is the worst release" | 35% higher costs from tokenizer, budget_tokens errors, context retrieval drop |
| Apiyi.com | Apr 17 | "Why is 4.7 less durable than 4.6?" | "Upgrade for coding agents, downgrade for everything else." Configuration workarounds. |
| Substack (dgtldept) | Apr 17 | "Opus 4.6 actually did get dumber" | 1M context window degrading at 20% usage. Cherny's counter-arguments documented fairly. |
| Substack (boringbot) | Apr 18 | PM perspective on 4.7 | Structured benchmarking: 4.7 won 4/5 PM tasks but lost narrative fluency |
| MindwiredAI | Apr 15 | "Is Claude Getting Dumber? The AMD Report" | Fact-checked both sides. Notes the $26→$42,121 claim conflates scaling with degradation. |
| pasqualepillitteri.it | Apr 22 | Zero-thinking-tokens deep dive | Documents Cherny's HN zero-thinking-tokens confirmation in detail. Also claimed Claude Code removed from Pro plan — **this was verified as incorrect against claude.com/pricing on Apr 22** (Pro still includes Claude Code at $17/mo). |
| Marginlab | Ongoing | Independent performance tracker | Opus 4.5: degraded from 58%→54%. Currently collecting 4.7 baseline. |
| r/ClaudeAI weekly reports | Ongoing | Performance megathreads | ~70% negative sentiment. Recurring themes: outages, throttling, quality degradation, "lobotomised" output. |

---

## Patterns Across the Evidence

### Pattern 1: The "fix" cycle
Each model release is positioned as addressing the previous version's problems. Community reports consistently show the same behavioural degradation reappearing within days. This suggests ecosystem-level root causes (infrastructure, cost optimisation, default settings) rather than model-level deficiencies.

### Pattern 2: Progressive lever removal
User control levers are deprecated or removed with each release: `budget_tokens` → deprecated on 4.6+. `CLAUDE_CODE_DISABLE_ADAPTIVE_THINKING` → unsupported on 4.7. Thinking content → redacted. Fixed budgets → replaced by adaptive. Each removal makes it harder for users to compensate for upstream changes.

### Pattern 3: Acknowledged changes that Anthropic says shouldn't matter, but do
Anthropic confirmed: adaptive thinking enabled, effort dropped to medium, thinking content redacted. Anthropic says these are UI changes or improvements. Community data says they correlate precisely with measured quality decline. The gap between official position and user experience is the core unresolved tension.

### Pattern 4: Cost-quality spiral
When quality drops, the model thrashes — more attempts, more corrections, more token burn. This accelerates context exhaustion, triggering compaction, which further degrades quality. The #42796 cost data (estimated 122x increase, even accounting for scaling) and the Latenode community reports of Max subscribers seeing 85% unreliable responses both illustrate this feedback loop.

### Pattern 5: Confirmed mechanism — zero-thinking-tokens turns
Boris Cherny's Hacker News confirmation is the single most important data point in this catalog. Anthropic's own team lead confirmed that adaptive thinking can allocate **zero reasoning tokens** on certain turns, and that these exact turns produced hallucinations. This means the degradation is not speculative — it has a confirmed mechanism, an acknowledged cause (adaptive thinking's turn classifier routing complex tasks as simple), and a known workaround (`CLAUDE_CODE_DISABLE_ADAPTIVE_THINKING=1`). The workaround was then removed for Opus 4.7. This is the clearest example of Pattern 2 (lever removal): the confirmed fix for a confirmed bug was deprecated in the next release.

### Pattern 6: Commercial pressure on compute
The ecosystem degradation isn't only technical. Anthropic is simultaneously managing compute scarcity by cutting third-party access (OpenClaw crackdown, Apr 4), tightening session limits (Mar 26), deploying a tokenizer that charges up to 35% more for the same input (Apr 16), and apparently beginning to restrict Claude Code to higher-priced tiers (Apr 22, developing — see timeline). Each of these actions increases user cost or reduces access without improving quality. The timing — during a period of acknowledged quality complaints — suggests commercial constraints are a significant driver of the ecosystem-level pattern.

---

## Open Items

- [ ] Retrieve the specific comment `issuecomment-4194007103` on #42796 — multiple approaches failed due to GitHub page length. The comment may be Boris Cherny's pinned response, which is documented via secondary sources (VentureBeat, MindwiredAI, pasqualepillitteri.it). **Recommendation: the user may need to manually retrieve and paste this comment, or access it while logged into GitHub.**
- [x] ~~Index additional `area:model` and `model` labelled issues~~ — Added #46949, #46099, #46212, #3511 from The Register analysis and community references
- [x] ~~Monitor Marginlab tracker for Opus 4.7 baseline data~~ — Tracker is currently collecting baseline; historical data shows Opus 4.5 degraded 58%→54%. Marginlab also tracks model transitions (4.6 on Feb 6, 4.7 on Apr 17). Will need ongoing monitoring.
- [x] ~~Search Reddit r/ClaudeAI for additional community threads~~ — r/ClaudeAI moderators publish weekly performance reports with ~70% negative sentiment. Recurring themes: outages, throttling, quality degradation, plan-mode hallucinations, policy-filter false positives.
- [x] ~~Track Anthropic's response to VentureBeat inquiry~~ — Anthropic spokesperson **did not answer questions** about reasoning defaults, context handling, throttling, or benchmarks. Referred reporters to X posts by Boris Cherny and Thariq Shihipar. Shihipar denied deliberate degradation (Apr 11). This non-response is itself significant evidence.

### New items identified during research
- [x] ~~Retrieve Boris Cherny's full Hacker News thread~~ — Retrieved from primary source (HN items 47664442, 47668520). Full findings documented above.
- [ ] Track Marginlab Opus 4.7 baseline when published
- [x] ~~Index the Anthropic September 2025 postmortem~~ — Full postmortem retrieved and documented above. Three overlapping infrastructure bugs confirmed.
- [ ] Monitor Claude Code / Pro plan situation — check whether Anthropic issues an announcement, whether existing Pro subscribers retain access, and whether the pricing page contradictions resolve

### Process note
The pasqualepillitteri.it claim that Claude Code was removed from the Pro plan was initially included, then removed after the pricing page feature bullets still showed it included. The HN thread (47854477) later revealed the situation is more complex: the pricing comparison matrix and support articles *have* been changed, but the feature bullets and product page haven't — Anthropic's own pages contradict each other. **Lesson: when sources conflict, present the conflict rather than picking one side. Follow the evidence thread to multiple primary sources. When a situation is developing, say so explicitly rather than asserting a conclusion.**
