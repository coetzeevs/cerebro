# Research: Extending Cerebro as a Universal Agentic Harness Manager

**Date:** 2026-04-22
**Status:** Research complete — ready for design & implementation
**Context:** Investigation into why Claude Code quality degrades outside well-configured project contexts, and how Cerebro can solve this

---

## 1. Problem Statement

A comprehensive harness built in `/Users/q/projects/` produces consistently high-quality Claude Code sessions: verified assumptions, structured workflows, review gates, persistent memory. But this quality **does not transfer** to other projects. Outside this context, Claude reverts to assumption-making, shallow validation, and compounding errors.

The root cause: the harness is a **composition of 6 independent layers**, and only 2 are portable:

| Layer | Mechanism | Portable today? |
|-------|-----------|-----------------|
| Rules | `CLAUDE.md` behavioral instructions | Partially (global CLAUDE.md is 6 lines; heavy rules are project-local) |
| Hooks | `settings.json` lifecycle hooks | Yes (Cerebro already scaffolds these) |
| Skills | `.claude/skills/` workflow definitions | **No** (project-scoped, no global mechanism) |
| Agents | `~/.claude/agents/` role definitions | Yes (global directory, but nothing invokes them without skills) |
| Memory | Cerebro brain (SQLite + vectors) | Yes (per-project, managed by cerebro) |
| Permissions | `settings.json` deny-lists | **No** (project-specific) |

**Key discovery:** `cerebro init` already scaffolds hooks, skills (recall/remember/consolidate), and CLAUDE.md sections. `cerebro init --force` updates to latest templates. Cerebro is **already half a harness manager**. The natural path is to extend it into a two-faceted tool: brain (memory) + harness (Claude Code configuration management).

---

## 2. Current Harness Audit (Working Reference: /Users/q/projects/)

### 2.1 What exists

**Global (`~/.claude/`)**
- `CLAUDE.md` — 6 lines: reference Cerebro memories, use acli for Jira, no co-author info, never push to main, always confirm before pushing
- `settings.json` — permissions allowlist, plugins (context7, typescript-lsp, pyright-lsp, ralph-loop), env vars
- `agents/` — 12 swarm agents (product-manager, architect, implementation-engineer, tech-lead, security-specialist, qa-engineer, platform-engineer, de-strategist, principal-de, principal-ae, ui-designer, lsp-navigator)

**Project root (`~/projects/.claude/`)**
- `settings.json` — Cerebro lifecycle hooks (SessionStart, UserPromptSubmit, PostCompact, SessionEnd)
- `settings.local.json` — MCP servers (Atlassian Rovo, context7), WebFetch domain allowlist, Jira MCP permissions
- `skills/` — 5 skills:
  - `/recall` — Cerebro memory retrieval
  - `/remember` — Cerebro memory storage with reconciliation
  - `/consolidate` — Episodic memory synthesis
  - `/implement` — 6-phase ticket-to-completion workflow (280 lines)
  - `/plan-week` — Kanban weekly planning with dependency analysis

**Repo-specific (e.g., `gg-terraform-foundation/.claude/`)**
- `CLAUDE.md` — 400+ lines: forbidden commands, Terraform-first rules, layer workflows, subagent delegation, Checkov baselines, PostgreSQL permissions pattern, documentation index
- `settings.json` — 85-line permission deny-list (blocks `terraform apply`, `gcloud` writes, `kubectl` writes)
- `agents/` — 11 domain agents (terraform-validator, terraform-plan, checkov-scanner, gcp-explorer, drift-detector, iam-auditor, k8s-validator, kubectl-debug, kubectl-logs)
- `commands/` — 27 slash commands (validate, plan, security-scan, pr-ready, add-iam-binding, lookup, etc.)

### 2.2 What prevents assumption-making (the specific mechanisms)

| Mechanism | Location | How it works |
|-----------|----------|-------------|
| `/implement` Phase 1 (context loading) | `.claude/skills/implement/SKILL.md` | Forces reading Cerebro, Jira, design docs, repo state BEFORE any design work |
| `/implement` Phase 5 (approval gate) | Same | **HARD STOP** — presents plan, waits for explicit user approval before any code |
| "Never act on unverified assumptions" rule | `/implement` Rules section | Behavioral constraint: research via context7/grep/docs before acting |
| PM agent review (Phase 2) | Same, invokes `swarm-product-manager` | Independent requirements verification |
| Architect agent review (Phase 3) | Same, invokes `swarm-principal-architect` | Independent technical design review |
| context7 plugin | Global plugin | Enables doc-based assumption verification |
| Cerebro auto-priming hooks | `.claude/settings.json` | Loads project context at session boundaries (SessionStart, UserPromptSubmit, PostCompact) |
| Terraform permission deny-list | `gg-terraform-foundation/.claude/settings.json` | Prevents dangerous commands regardless of intent |
| `/validate`, `/plan`, `/pr-ready` commands | `gg-terraform-foundation/.claude/commands/` | Multi-step validation before any change lands |

### 2.3 What's NOT portable (and causes degradation in other projects)

- Skills (implement, plan-week) — project-scoped only, no global skills mechanism in Claude Code
- Domain CLAUDE.md — necessarily repo-specific
- Permission deny-lists — necessarily repo-specific
- Domain agents and commands — necessarily repo-specific
- The behavioral rules embedded in skill files — invisible outside the skill's project

### 2.4 Hook system detail

Cerebro currently scaffolds these hooks in `.claude/settings.json`:

| Event | Trigger | What it does |
|-------|---------|-------------|
| `SessionStart` (startup) | New session | `cerebro recall --prime` — loads high-value memories |
| `SessionStart` (resume) | Session resume | Same |
| `SessionStart` (compact) | After compaction | Same |
| `SessionStart` (clear) | After `/clear` | Same |
| `UserPromptSubmit` | Before each prompt | Fallback priming if startup hook missed (sentinel-based dedup) |
| `PostCompact` | After compaction | Clears sentinel so memories re-load on next prompt |
| `SessionEnd` | Session exit | `cerebro gc --threshold 0.01` — garbage collection |

---

## 3. External Research

### 3.1 Anthropic: Harness Design for Long-Running Applications (2026-03-24)

Source: https://www.anthropic.com/engineering/harness-design-long-running-apps

**Key architecture: Three-agent system (Planner → Generator → Evaluator)**

1. **Planner** — Takes a 1-4 sentence prompt and expands into a full product spec. Stays high-level intentionally: "if the planner tried to specify granular technical details upfront and got something wrong, the errors in the spec would cascade into the downstream implementation."

2. **Generator** — Implements the spec in sprints, one feature at a time. Self-evaluates at end of each sprint before handing off to QA.

3. **Evaluator** — Uses Playwright MCP to click through the running application. Grades against criteria with hard thresholds. Files specific bugs. This is the GAN-inspired innovation: separating generation from evaluation.

**Critical findings:**

- **Self-evaluation degeneracy**: "When asked to evaluate work they've produced, agents tend to respond by confidently praising the work—even when, to a human observer, the quality is obviously mediocre." Separating generator from evaluator is the key lever.
- **Context anxiety**: Models wrap up work prematurely as context fills. Context resets (fresh agent with structured handoff) address this better than compaction.
- **Sprint contracts**: Before each sprint, generator and evaluator negotiate what "done" looks like. This maps to our Phase 5 approval gate.
- **Evaluator calibration**: "Out of the box, Claude is a poor QA agent. In early runs, I watched it identify legitimate issues, then talk itself into deciding they weren't a big deal and approve the work anyway." Required several rounds of prompt tuning.
- **Simplify as models improve**: "Every component in a harness encodes an assumption about what the model can't do on its own, and those assumptions are worth stress testing." Opus 4.6 eliminated the need for sprint decomposition that Sonnet 4.5 required.
- **The harness space doesn't shrink**: "The space of interesting harness combinations doesn't shrink as models improve. Instead, it moves."

### 3.2 gstack (github.com/garrytan/gstack)

**What it is:** Y Combinator CEO Garry Tan's toolkit for Claude Code. Transforms it into a virtual engineering team.

**Key patterns:**
- **Process-driven cycle**: Think → Plan → Build → Review → Test → Ship → Reflect
- **23+ specialized roles** via slash commands (`/office-hours`, `/plan-ceo-review`, `/review`, `/qa`, `/ship`)
- **Smart routing**: Detects work type and suggests appropriate reviews (design changes skip architecture review)
- **Parallel sprints**: Coordinates 10-15 simultaneous Claude Code sessions via a tool called Conductor
- **Continuous learning**: `/learn` command maintains project-specific patterns across sessions
- **Preflight self-check**: Validation before independent review — each work phase includes a self-check step
- **Go-native engine**: ~10ms per phase processing

**Relevance to Cerebro:** gstack's `/learn` is conceptually similar to Cerebro's `/remember` but lighter. The preflight self-check pattern maps to our `/implement` Phase 4 validation. Smart routing (skipping irrelevant reviews) is an optimization we don't have.

### 3.3 Twelve Agentic Harness Patterns

Source: https://generativeprogrammer.com/p/12-agentic-harness-patterns-from

**Memory & Context (5 patterns):**
1. **Persistent Instruction File** — CLAUDE.md loaded every session
2. **Scoped Context Assembly** — Multiple CLAUDE.md files at different hierarchy levels
3. **Tiered Memory** — Compact always-loaded index + topic files loaded on demand + searchable transcripts
4. **Dream Consolidation** — Background dedup/prune during idle (maps to our `/consolidate`)
5. **Progressive Context Compaction** — Multi-stage compression based on conversation age

**Workflow & Orchestration (3 patterns):**
6. **Explore-Plan-Act Loop** — Separate read-only exploration → planning → execution
7. **Context-Isolated Subagents** — Separate agents with restricted tool sets
8. **Fork-Join Parallelism** — Spawn multiple agents in isolated git worktrees

**Tools & Permissions (3 patterns):**
9. **Progressive Tool Expansion** — Start small, additional tools activate on demand
10. **Command Risk Classification** — Pre-parse commands for deterministic permission gating
11. **Single-Purpose Tool Design** — Typed tools replacing general shell access

**Automation (1 pattern):**
12. **Deterministic Lifecycle Hooks** — Shell commands at specific lifecycle points

### 3.4 Official Claude Code Best Practices

Source: https://code.claude.com/docs/en/best-practices

**Key guidance:**
- **CLAUDE.md should be 50-100 lines max** — frontier models can follow 150-200 instructions but Claude Code's system prompt already uses ~50
- **Use skills for domain knowledge loaded on demand**, not in CLAUDE.md
- **Hooks are deterministic enforcement; CLAUDE.md instructions are advisory**
- **Provide tests, screenshots, or expected outputs** so Claude can verify its own work
- **Workflow**: Explore → Plan → Implement → Commit
- **Use Plan Mode** for read-only exploration and detailed planning

### 3.5 Everything Claude Code (github.com/affaan-m/everything-claude-code)

A maximalist reference implementation:
- 48 specialized agents
- 183 reusable skills organized by domain
- Hook-based automation at lifecycle phases
- Memory persistence across sessions

Shows the ceiling of the approach. Over-engineered for most users but demonstrates scalability.

### 3.6 Claude Code Harness (github.com/Chachamaru127/claude-code-harness)

A lighter reference implementation:
- Plan-Work-Review-Release cycle
- 4 analytical perspectives in review (security, performance, quality, accessibility)
- Preflight self-check validation before review

---

## 4. Proposal: Cerebro as Brain + Harness Manager

### 4.1 Two facets, one CLI

```
cerebro
├── Brain (existing)
│   ├── add, recall, search, gc, edge, consolidate...
│   └── Project-scoped SQLite + vector search
│
└── Harness (new)
    ├── cerebro init           (existing — scaffolds hooks + skills + CLAUDE.md)
    ├── cerebro init --force   (existing — updates to latest templates)
    ├── cerebro init --agents  (new — scaffolds agent definitions)
    ├── cerebro harness sync   (new — updates managed files to latest templates)
    ├── cerebro harness status (new — shows installed vs available, modified vs template)
    ├── cerebro harness diff   (new — three-way diff: old-template vs user-file vs new-template)
    └── Template registry (embedded in binary via //go:embed)
```

### 4.2 What `cerebro init` should scaffold

**Tier 1 — Core (every project, default):**

```
.claude/settings.json       → Existing lifecycle hooks (unchanged)
.claude/skills/recall/      → Existing (unchanged)
.claude/skills/remember/    → Existing (unchanged)
.claude/skills/consolidate/ → Existing (unchanged)
.claude/skills/implement/   → NEW: lightweight universal implementation workflow (60-80 lines)
.claude/skills/preflight/   → NEW: pre-commit verification checklist
CLAUDE.md                   → Existing Cerebro section + NEW behavioral rules section
```

**Tier 2 — Agents (opt-in: `cerebro init --agents`):**

```
~/.claude/agents/evaluator.md          → NEW: skeptical reviewer (Anthropic GAN pattern)
~/.claude/agents/product-manager.md    → Requirements review
~/.claude/agents/architect.md          → Technical design review
~/.claude/agents/tech-lead.md          → Code quality gate
```

Note: agents go in the GLOBAL directory (`~/.claude/agents/`) because they are stateless and available everywhere. This is already the pattern for the existing 12 swarm agents.

**Tier 3 — Domain packs (opt-in: `cerebro init --pack <name>`, DEFERRED):**

```
cerebro init --pack terraform  → TF-specific agents, deny-lists, validation commands
cerebro init --pack node       → Node-specific patterns
cerebro init --pack python     → Python-specific patterns
```

### 4.3 Placement matrix

| Artifact | Placement | Rationale |
|----------|-----------|-----------|
| Cerebro lifecycle hooks | Project `.claude/settings.json` | Per-project brain; different projects may not have cerebro initialized |
| Skills (recall, remember, consolidate, implement, preflight) | Project `.claude/skills/` | Skills are project-scoped in Claude Code — no global skills mechanism exists |
| Agents (evaluator, PM, architect, tech-lead) | Global `~/.claude/agents/` | Already the established pattern; agents are stateless |
| CLAUDE.md behavioral rules | Project `CLAUDE.md` (cerebro-managed section) | Uses existing section marker pattern |
| Domain-specific CLAUDE.md | Project `CLAUDE.md` (user-managed section) | User writes above the cerebro marker |
| Permission deny-lists | Project `.claude/settings.json` | Part of domain packs (Tier 3, deferred) |

### 4.4 The lightweight `/implement` skill

The single most impactful addition. Prevents assumption-compounding by enforcing context → research → approval → execution.

**Minimum viable phases (target 60-80 lines):**

```
Phase 1 — Context:
  Read target files before proposing changes.
  Check for CLAUDE.md or project documentation.
  Run /recall if Cerebro is initialized.

Phase 2 — Research:
  Verify assumptions via docs/grep/context7.
  No unverified claims about APIs, configs, or behaviors.
  If unsure, research first — don't guess.

Phase 3 — Plan:
  Present approach, file list, and key decisions.
  **STOP. Wait for explicit user approval before writing any code.**

Phase 4 — Execute:
  Implement with atomic commits.
  Never push to main/default branch.
  Never push without confirming with the user.

Phase 5 — Verify:
  Run tests/linters if applicable.
  Check diff against the approved plan.
  Flag any deviations from the plan.

Rules:
- Never act on unverified assumptions.
- Always read files before editing them.
- Prefer the simplest approach that works.
- Never proceed past Phase 3 without explicit user approval.
```

Projects like the EDP can layer their heavy 280-line version on top — Claude Code loads project-local skills over global templates.

### 4.5 The evaluator agent

A global agent implementing the Anthropic generator/evaluator separation:

```markdown
---
name: evaluator
model: sonnet
---
You are a skeptical reviewer. Your job is to find what's WRONG, not confirm
what's right. Specifically check:

1. Unverified assumptions — were APIs, configs, or behaviors confirmed against
   docs or the actual codebase, or just assumed?
2. Edge cases — does the implementation handle error paths, empty inputs,
   concurrent access, or just the happy path?
3. Missing verification — are there claims about "how X works" that weren't
   confirmed via context7, grep, or reading the actual code?
4. Production readiness — would this work in production, or only in a demo?

Be specific. Point to exact files and lines. Explain what could go wrong.
Do NOT praise the work. Focus exclusively on problems and risks.
```

Note: uses `model: sonnet` not `model: opus` to keep cost proportional. The evaluator needs systematic checking, not creative reasoning.

### 4.6 Behavioral rules for CLAUDE.md

Added as a new section in the cerebro-managed CLAUDE.md block:

```markdown
## Behavioral Rules

- Never act on unverified assumptions. Form hypotheses, then confirm through
  research (context7, codebase grep, provider docs) before acting on them.
- Always read files before editing them. Never propose changes to code you
  haven't read.
- Always link to documentation sources. Never reference a doc by name alone
  without a resolvable link.
- Prefer the simplest approach that works. Don't add abstractions, features,
  or error handling beyond what the task requires.
```

---

## 5. Template Management Design

### 5.1 Ownership markers

Files scaffolded by Cerebro should contain an ownership marker:

```markdown
<!-- managed-by: cerebro v0.5.0 -->
```

`cerebro harness sync` only touches files with this marker. User-created files (like the heavy `/implement` or `/plan-week`) are never touched because they lack the marker.

### 5.2 Version metadata

Each scaffolded file includes a version comment:

```markdown
<!-- cerebro-template: v0.5.0 implement -->
```

`harness status` compares this against the current binary's embedded template version to show outdated files.

### 5.3 Conflict resolution strategy (checksum-based skip)

On `cerebro harness sync`:

1. Record the SHA-256 of each template at install time in `.claude/.cerebro-manifest.json`
2. On sync, compare the file's current hash against the manifest hash:
   - **Hash matches**: File is unmodified → safe to overwrite with new template
   - **Hash differs**: User has customized → skip, warn, write new template to `.new` sidecar file for manual merge
3. `cerebro harness diff` shows three-way comparison: template-at-install-time vs current-template vs user's-file

This is the `create-react-app` eject pattern — users understand it.

### 5.4 CLAUDE.md section markers

Extend the existing single-marker pattern to use paired markers:

```markdown
<!-- cerebro:behavioral-rules:start -->
## Behavioral Rules
...
<!-- cerebro:behavioral-rules:end -->

<!-- cerebro:memory-system:start -->
## Cerebro Memory System
...
<!-- cerebro:memory-system:end -->
```

Content between markers is managed by Cerebro. Content outside markers is user-owned. More robust than the current approach which assumes the cerebro section is always last.

### 5.5 Embedding vs external registry

**Decision: Embed templates in the binary.**

Rationale:
- Cerebro is a single-binary Go CLI distributed via goreleaser
- `//go:embed` (already in use in `scaffold.go`) ships templates atomically with the CLI version
- No network dependency, no registry to maintain, no version skew
- Total template size is under 100KB even with all proposed additions
- Template iteration speed is limited by release cadence — acceptable for a small, opinionated template set

**Escape hatch for development:**
```
cerebro harness sync --from /path/to/local/templates
```

This flag allows testing new templates without building a release.

---

## 6. Stop Hook Analysis

### 6.1 Prompt-type Stop hook — REJECTED

The plan initially proposed a prompt-type Stop hook for self-evaluation:

```json
{
  "hooks": {
    "Stop": [{
      "type": "prompt",
      "prompt": "Self-check before completing: (1) Were all assumptions verified?..."
    }]
  }
}
```

**Why it was rejected (both reviewers agreed):**

1. **Known regressions**: Prompt-type Stop hooks had regressions in Claude Code v2.0.37+ where the prompt evaluator received only metadata, NOT the conversation transcript. The self-evaluation prompt cannot actually inspect what Claude did. Open issues: #11610, #11786.
2. **Self-evaluation degeneracy**: This is precisely the problem the Anthropic article warns about. "Tuning a standalone evaluator to be skeptical turns out to be far more tractable than making a generator critical of its own work." A prompt hook shares the full context (and biases) with the generator.
3. **Latency on every turn**: Adds inference overhead to every task completion, including trivial ones where it always reports "all clear."
4. **False confidence**: The hook gives the appearance of verification without the substance. Worse than no hook because it masks the gap.

### 6.2 Agent-type Stop hook — DEFERRED

An agent-type Stop hook would be stronger:

```json
{
  "hooks": {
    "Stop": [{
      "type": "agent",
      "prompt": "Check modified files for unverified assumptions. Return {\"decision\": \"approve\"} or {\"decision\": \"block\", \"reason\": \"...\"}.",
      "model": "sonnet"
    }]
  }
}
```

**Advantages:**
- Spawns a subagent with fresh context and tool access (Read, Grep, Glob)
- Can read modified files and check for patterns indicating assumptions
- Implements the Anthropic generator/evaluator separation properly

**Concerns:**
- Adds 5-15 seconds latency per turn
- Cost of additional model invocation per turn (Sonnet pricing)
- Infinite loop risk: if subagent's Stop event triggers the same hook (needs guard)
- VSCode extension may not fire Stop hooks at all (issue #40029)
- Overkill for trivial interactions (questions, file reads, small edits)

**Decision: Defer.** The behavioral rules in CLAUDE.md + the evaluator agent + the `/implement` skill's approval gate provide three layers of defense. Ship without the Stop hook, measure whether assumptions still slip through, and add it only if they do. This follows the plan's own principle: "Don't add complexity without measuring the failure."

### 6.3 Alternative: Command-type Stop hook as a deterministic fallback

If a Stop hook is eventually warranted, a bash-type hook that checks `git diff --name-only` and verifies basic invariants (no `.env` files staged, no files modified without corresponding tests) is deterministic and regression-proof. It doesn't catch assumption-making but it catches hygiene failures cheaply.

---

## 7. Architect Review Findings

### 7.1 Architectural soundness — APPROVED

The two-faceted model (brain + harness) is architecturally sound. The brain manages state (SQLite, vectors, knowledge graph). The harness manages configuration (static files that Claude Code reads). They share no runtime coupling. The tiered approach (core → agents → domain packs) mirrors Claude Code's own layering (global → project → repo).

### 7.2 Scope creep assessment

**Brain + harness coupling is justified** for Tiers 1-2:
- `cerebro init` already does harness work; splitting would worsen UX (two tools for one action)
- Skills invoke cerebro commands; shipping from the same binary guarantees version compatibility
- The template registry is trivially small (8-10 embedded markdown files)

**Tier 3 (domain packs) is where scope creep risk is real.** Terraform agents, Node deny-lists, Python validation — this becomes a configuration management system. The number of domain packs is unbounded. **Explicitly defer until Tiers 1-2 are validated** with a clear trigger condition: "when 3+ projects are running with the core harness and domain-specific patterns emerge organically."

### 7.3 Claude Code compatibility findings

| Assumption | Status | Detail |
|------------|--------|--------|
| Hooks merge across scopes (global + project) | **Confirmed** | Hooks from `~/.claude/settings.json` and `.claude/settings.json` are concatenated |
| Skills are project-scoped only | **Confirmed** | No `~/.claude/skills/` mechanism exists; skills must live in project `.claude/skills/` |
| Agents are global | **Confirmed** | `~/.claude/agents/` is available everywhere |
| CLAUDE.md files layer (global → project → repo) | **Confirmed** | All are loaded; project/repo CLAUDE.md adds to global |
| `settings.local.json` can clobber permissions | **Confirmed bug** | Project-level `settings.local.json` can replace (not merge) global permissions arrays |
| Prompt-type Stop hooks receive conversation content | **BROKEN** | Regressions in v2.0.37+; open issues #11610, #11786 |
| Stop hooks fire in VSCode extension | **BROKEN** | Open issue #40029 |

### 7.4 Lightweight /implement — architect's recommended structure

Target 60-80 lines. Minimum viable phases:

1. **Context**: Read target files, check for project docs/CLAUDE.md, run /recall if available
2. **Research**: Verify assumptions via docs/grep/context7. No unverified claims.
3. **Plan**: Present approach, file list, and key decisions. **STOP. Wait for approval.**
4. **Execute**: Implement with atomic commits. Never push to main. Never push without asking.
5. **Verify**: Run tests/linters. Check diff against plan. Flag deviations.

The heavy version layers on top: Jira integration, swarm agent reviews, TDD enforcement, draft PR workflow. A project with the heavy version in `.claude/skills/implement/` overrides the lightweight one.

### 7.5 Missing pieces identified

| Gap | Severity | Detail |
|-----|----------|--------|
| No `settings.local.json` distinction | Medium | Some harness elements (Stop hook) might be personal preferences; plan doesn't address which file they go in |
| No migration path for existing projects | Medium | Primary project already has MORE than templates; `harness sync` needs to detect and leave custom files alone |
| No `allowed-tools` specification for new skills | Medium | Lightweight `/implement` needs Read, Write, Edit, Bash, Grep, Glob; overly permissive sets are a security risk |
| No `/preflight` skill specification | Medium | Listed in Tier 1 but never described; unclear if it's standalone or a phase within `/implement` |
| No testing strategy for new templates | Medium | Each template needs unit tests for scaffold behavior + integration tests for valid Claude Code configuration |
| No cost/performance analysis | Low | Evaluator agent (even with Sonnet) adds per-invocation cost; should estimate per-session overhead |

---

## 8. Tech Lead Review Findings

### 8.1 Phase 1 delivers 60-70% of value — do it first

Phase 1 requires ZERO cerebro changes:
- Add 4 behavioral rules to `~/.claude/CLAUDE.md` (currently 6 lines → ~15 lines, well under 100-line guidance)
- Create `~/.claude/agents/evaluator.md`
- Optionally add Stop hook to `~/.claude/settings.json` (but see: DEFERRED above)

**Measure improvement across 5-10 real tasks in non-EDP projects before investing in Phase 2+.** Only proceed if Phase 1 demonstrably fails to prevent degradation. This is the plan's own principle: "find the simplest solution possible, and only increase complexity when needed."

### 8.2 Template conflict resolution — the hardest problem

**Recommended approach: Layering model + checksum-based skip**

1. **Layering**: Global agents in `~/.claude/agents/` (managed by cerebro). Project-local skill overrides in `.claude/skills/` (user-managed). No merge needed — project-local takes precedence.
2. **Checksum skip**: For skills managed by cerebro in `.claude/skills/`, record SHA-256 at install time in `.claude/.cerebro-manifest.json`. On sync: unmodified files overwrite safely; modified files skip with warning + `.new` sidecar.
3. **Ownership markers**: Only touch files containing `<!-- managed-by: cerebro -->`.

### 8.3 Maintenance burden

Every template change requires a Go build, version bump, and release. For a solo engineer, this creates coupling between content iteration and binary releases.

**Mitigation options:**
- Accept it (small template set, infrequent changes)
- Add `cerebro harness sync --from /path/` escape hatch for development
- Hybrid: core templates embedded, content templates fetchable from a git repo (like cookiecutter/yeoman). Decouples content iteration from binary releases.

### 8.4 Testing strategy

Need a **regression test suite** of 5-10 real prompts that historically produced degraded output:
- Document: the prompt, the project context, the specific failure (e.g., "assumed GCS bucket was auto-provisioned, did not check provider docs")
- Run with and without the harness
- Binary pass/fail

Also need: latency measurement for any hooks, and a long-form test (run `/implement` on a real task in a non-EDP project).

### 8.5 Risk assessment

| Risk | Impact | Mitigation |
|------|--------|------------|
| Global CLAUDE.md rules conflict with project needs | Low | The 4 proposed rules are truly universal; monitor for edge cases |
| Stop hook causes recursive loops | Medium | Guard: check for subagent indicator, exit early. Or: defer Stop hook entirely |
| Template sync corrupts settings.json | High | Always backup before modifying; extend existing brain backup pattern |
| Phase 1 behavioral rules ignored by Claude | Medium | Rules are advisory, not deterministic; if ignored, escalate to hook-based enforcement |

**Rollback plan:**
- Phase 1: Revert 3 files (`~/.claude/CLAUDE.md`, `~/.claude/settings.json`, `~/.claude/agents/evaluator.md`)
- Phase 2+: `cerebro init --force` with previous binary version, or manual deletion of scaffolded files

---

## 9. Implementation Phases

### Phase 1: Immediate improvements (no cerebro changes)

| Action | File | Detail |
|--------|------|--------|
| Add behavioral rules | `~/.claude/CLAUDE.md` | 4 universal rules (assumptions, read-before-write, doc linking, simplicity) |
| Create evaluator agent | `~/.claude/agents/evaluator.md` | Skeptical reviewer, model: sonnet |
| Measure | — | Run 5-10 regression prompts across non-EDP projects |

### Phase 2: Cerebro template expansion

| Action | Detail |
|--------|--------|
| Add `/implement` template | Lightweight 60-80 line universal workflow |
| Add `/preflight` template | Pre-commit verification checklist |
| Add behavioral rules to CLAUDE.md scaffold | New section with paired markers |
| Add `--agents` flag to `cerebro init` | Scaffolds evaluator, PM, architect, tech-lead to `~/.claude/agents/` |
| Add ownership markers | `<!-- managed-by: cerebro v{version} -->` in all managed files |
| Add version metadata | `<!-- cerebro-template: v{version} {name} -->` in scaffolded files |
| Add `.cerebro-manifest.json` | SHA-256 checksums for conflict detection |
| Update `scaffold_test.go` | Tests for all new templates |

### Phase 3: Harness management commands

| Command | Purpose |
|---------|---------|
| `cerebro harness status` | Show installed vs available template versions, modified files, shadowed skills |
| `cerebro harness sync` | Update managed files to latest templates (checksum-based skip) |
| `cerebro harness diff` | Three-way diff: old-template vs user-file vs new-template |

### Phase 4: Domain packs (DEFERRED)

**Trigger condition:** When 3+ projects are running with the core harness and domain-specific patterns emerge organically.

| Pack | Content |
|------|---------|
| `--pack terraform` | TF-specific agents, deny-lists, validation commands |
| `--pack node` | Node-specific patterns |
| `--pack python` | Python-specific patterns |
| `--pack data-engineering` | DE-specific patterns |

### Phase 5: Stop hook (DEFERRED)

**Trigger condition:** Phase 1+2 demonstrably fail to prevent assumption-making in measured regression tests.

If triggered, implement as agent-type (not prompt-type) with:
- `model: sonnet` (cost-efficient)
- Read-only tool access (Read, Grep, Glob)
- Guard against recursive invocation
- Opt-in via `cerebro init --stop-hook`

---

## 10. Key Principles

From the Anthropic article and confirmed by both reviewers:

1. **Rules aren't suggestions — they're infrastructure.** Hooks execute automatically; CLAUDE.md is advisory. Prefer hooks and skills over CLAUDE.md for critical behaviors.

2. **Separate generator from evaluator.** The model is poor at self-evaluation. Independent review (via agents or hooks) catches what self-review misses.

3. **Every harness component encodes an assumption.** Stress-test those assumptions as models improve. If the model doesn't fail at something, don't build scaffolding for it.

4. **Simplify toward the model.** "Find the simplest solution possible, and only increase complexity when needed." Phase 1 first. Measure. Add complexity only where failure persists.

5. **The harness space moves, not shrinks.** As models improve, the interesting work is finding the next novel combination — not maintaining scaffolding for problems already solved.

6. **Context management is everything.** Context window degradation is the bottleneck. Cerebro's memory priming, compaction recovery, and structured handoffs are the foundation everything else builds on.
