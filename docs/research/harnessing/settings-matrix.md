# Claude Code Settings Audit: Quality-Relevant Levers

**Companion to:** approach.md
**Last updated:** April 22, 2026
**Source:** code.claude.com/docs/en/ (official documentation)

---

## How to Read This Matrix

Each setting is evaluated for its impact on reasoning quality, user control, and durability across model versions. The "Quality Impact" column indicates how directly the setting affects the degradation patterns documented in the evidence catalog. The "Durability" column indicates whether the setting survives model transitions.

**Priority key:** 🔴 Critical (test immediately) · 🟡 Important (test this week) · 🟢 Useful (implement when convenient)

---

## 1. Thinking & Reasoning Controls

These settings directly affect how deeply the model reasons before acting — the primary mechanism implicated in the #42796 analysis.

| Setting | Type | Default | Recommended | Quality Impact | Durability | Notes |
|---------|------|---------|-------------|---------------|------------|-------|
| 🔴 **Effort level** (`/effort`, `CLAUDE_CODE_EFFORT_LEVEL`, `effortLevel` in settings) | Session / Env / Settings | `xhigh` on Opus 4.7; `high` on 4.6 at launch (Feb 9); changed to `medium` on Pro/Max (Mar 3) | `high` minimum. `xhigh` or `max` for complex tasks. | **Direct.** Controls adaptive reasoning depth. The effort→medium change on Mar 3 is one of the confirmed causes of degradation. **However, Boris Cherny confirmed that even effort=high does not prevent zero-thinking turns** — adaptive thinking can still allocate zero reasoning tokens on certain turns, causing hallucinations. Effort=high is necessary but not sufficient. | Moderate. Available levels change per model. `xhigh` only exists on 4.7. | `max` is session-only unless set via env var. Env var overrides all other methods. Can also set per-skill/subagent via frontmatter (`effort` field). Env var applies globally including to sub-agents. |
| 🔴 **`CLAUDE_CODE_DISABLE_ADAPTIVE_THINKING`** | Env var | Not set (adaptive on) | Set to `1` on Opus 4.6 / Sonnet 4.6 | **Direct. This is the only confirmed workaround for the zero-thinking-tokens bug.** Boris Cherny provided this as the interim fix after confirming adaptive thinking allocated zero reasoning on hallucination-producing turns. Reverts to fixed thinking budget. | **Low.** Explicitly unsupported on Opus 4.7. Being phased out — which means the only confirmed fix for a confirmed bug is being removed. | When set to `1`, thinking depth is controlled by `MAX_THINKING_TOKENS` instead of adaptive reasoning. |
| 🟡 **`MAX_THINKING_TOKENS`** | Env var | Model-dependent (up to 31,999) | Experiment: 10000–31999 when adaptive is disabled | **Direct** (when adaptive is off). Controls fixed thinking budget. | **Low.** Only meaningful when adaptive thinking is disabled. Deprecated on 4.6+, ignored on 4.7 unless set to `0`. | Setting to `0` disables thinking entirely. On adaptive models, only `0` is honoured; other values are ignored. |
| 🟡 **`CLAUDE_CODE_DISABLE_THINKING`** | Env var | Not set | `0` (ensure thinking is NOT disabled) | **Direct.** Force-disables extended thinking regardless of model or other settings. | High (model-agnostic) | More forceful than `MAX_THINKING_TOKENS=0`. Verify this is NOT set in your environment. |
| 🟡 **`showThinkingSummaries`** | Settings.json | `false` (interactive mode) | `true` | **Indirect.** Doesn't change reasoning depth, but makes it visible so you can detect upstream changes. | High (model-agnostic) | Essential for monitoring. Without this, you can't tell if thinking depth has changed. |
| 🟢 **`ultrathink` in prompt** | In-context instruction | N/A | Use for one-off deep reasoning tasks | **Indirect.** Adds an in-context instruction but does NOT change effort level at the API level. | High (model-agnostic) | Only works when `MAX_THINKING_TOKENS` is not set (see #18072 — silently ignored otherwise). |

---

## 2. Context Window & Compaction Controls

These settings affect how much context the model retains and when it loses track of earlier instructions — the mechanism behind convention drift and context degradation. Community reports document quality degradation at 20% of the 1M window, context compression kicking in at 40%, and the model itself recommending a fresh session at 48% usage.

| Setting | Type | Default | Recommended | Quality Impact | Durability | Notes |
|---------|------|---------|-------------|---------------|------------|-------|
| 🔴 **`CLAUDE_AUTOCOMPACT_PCT_OVERRIDE`** | Env var | ~95% of context capacity | Experiment: `50`–`70` for earlier compaction | **Direct.** Controls when auto-compaction triggers. Lower values compact more aggressively, preventing the quality degradation that occurs at high context usage. | High | Community reports document degradation starting at 20% of 1M window. A practical rule: keep context under 50%. Compacting at 50–60% may maintain quality better than letting it fill to 95%. |
| 🟡 **`CLAUDE_CODE_AUTO_COMPACT_WINDOW`** | Env var | Model's full context window (200K or 1M) | Consider `500000` on 1M models | **Direct.** Treats the window as smaller than it actually is for compaction purposes. Prevents the late-window degradation documented in community reports. | High | Decouples compaction from the 1M window. The 1M window was marketed as an upgrade but degrades quality before it fills. |
| 🟡 **`CLAUDE_CODE_DISABLE_1M_CONTEXT`** | Env var | Not set (1M enabled) | Test with `1` if experiencing context degradation | **Direct.** Forces 200K context window. Trades capacity for quality. | High | Nuclear option: you lose long-session capability but may gain quality consistency. |
| 🟡 **`DISABLE_COMPACT`** | Env var | Not set | Do NOT set (compaction is essential) | **Direct.** Disables automatic compaction entirely. Context fills and conversation fails. | N/A | Only useful for debugging. Never use in production. |
| 🟢 **`.claudeignore`** | File | None | Add build artifacts, node_modules, lock files, generated code | **Indirect.** Reduces token waste on irrelevant context, leaving more room for actual reasoning. | High (model-agnostic) | Model-agnostic, always beneficial. No downside. |

---

## 3. Model Selection & Routing

These settings control which model runs and how it's routed — important for the ecosystem-level pattern of degradation recurring across versions.

| Setting | Type | Default | Recommended | Quality Impact | Durability | Notes |
|---------|------|---------|-------------|---------------|------------|-------|
| 🔴 **`ANTHROPIC_MODEL`** / `/model` / `model` in settings | Multiple | Varies by plan (Opus 4.7 for Max/Team; Sonnet 4.6 for Pro/API) | Test current options against your workflow. Don't assume latest = best. | **Direct.** Different models exhibit different degradation patterns. | Settings persist; model aliases change what they resolve to. | `opus` resolves to 4.7 on Anthropic API, 4.6 on Bedrock/Vertex. Pin with full model name for stability. |
| 🟡 **`ANTHROPIC_DEFAULT_OPUS_MODEL`** | Env var | Latest opus for provider | Pin to a specific version if current is good | **Indirect.** Prevents surprise model changes when Anthropic updates alias resolution. | **Critical for stability.** Without this, `opus` silently changes what it points to. | Use full model name like `claude-opus-4-6` to pin. |
| 🟡 **`opusplan` model alias** | Model setting | N/A | Test for cost-quality balance | **Indirect.** Uses Opus for planning (deep reasoning) and Sonnet for execution (cheaper, faster). | Moderate. Depends on both models. | Does NOT get 1M context — runs at 200K. May be a feature, not a bug, given context degradation issues. |
| 🟢 **`CLAUDE_CODE_DISABLE_LEGACY_MODEL_REMAP`** | Env var | Not set (remap enabled) | Set to `1` if you want to pin older models | **Indirect.** Prevents auto-remapping of older model IDs to current versions. | High | Use when you've found a model version that works and don't want it silently upgraded. |

---

## 4. Session & Workflow Controls

These settings affect session management, which determines how much context accumulates and when it degrades.

| Setting | Type | Default | Recommended | Quality Impact | Durability | Notes |
|---------|------|---------|-------------|---------------|------------|-------|
| 🟡 **`BASH_DEFAULT_TIMEOUT_MS`** | Env var | 120000 (2 min) | Keep default or increase for complex builds | **Indirect.** Timeout failures can cause the model to abandon good approaches in favour of simpler ones. | High | |
| 🟡 **`CLAUDE_CODE_MAX_OUTPUT_TOKENS`** | Env var | Model-dependent | Increase for complex tasks (64K+ on Opus 4.7 at xhigh/max) | **Indirect.** If output is truncated, the model may produce incomplete reasoning. "Increasing this value reduces the effective context window available before auto-compaction triggers." | Model-dependent | Trade-off: more output room = earlier compaction. |
| 🟢 **`CLAUDE_CODE_MAX_TOOL_USE_CONCURRENCY`** | Env var | 10 | Default is fine for most workflows | **Indirect.** Higher values increase parallelism but may cause context pollution with concurrent results. | High | |
| 🟢 **`CLAUDE_BASH_MAINTAIN_PROJECT_WORKING_DIR`** | Env var | Not set | Set to `1` | **Indirect.** Prevents the model from losing track of which directory it's in after cd commands. | High | Simple quality-of-life improvement. |
| 🟢 **`CLAUDE_CODE_FILE_READ_MAX_OUTPUT_TOKENS`** | Env var | Default (model-dependent) | Increase if model is truncating file reads | **Indirect.** If files are truncated during reads, the model operates on incomplete information — directly causing the "edit without full context" pattern. | High | |

---

## 5. Hook Configuration

Hooks are the most durable mitigation category — they operate outside the model's control plane and survive model transitions. Given that the confirmed workaround for the zero-thinking-tokens bug (`CLAUDE_CODE_DISABLE_ADAPTIVE_THINKING`) has been deprecated on Opus 4.7, hooks become the primary line of defence for catching the *consequences* of zero-reasoning turns externally.

| Hook Event | Purpose | Quality Impact | Implementation Complexity |
|-----------|---------|---------------|--------------------------|
| 🔴 **Stop** | Block premature stopping, permission-seeking, ownership-dodging | **Direct.** Exit code 1 prevents session from stopping. Forces continuation. The #42796 stop-phrase-guard went from 0 violations (entire history) to 173 in 17 days — this is a proven quality canary. | Low. Can start with simple phrase matching (stellaraccident pattern). |
| 🔴 **PreToolUse** (Write/Edit) | Enforce read-before-edit pattern. Block or warn when editing a file not recently read. | **Direct.** Addresses the core symptom: read:edit ratio collapsed from 6.6 to 2.0 in #42796 data. One in three edits in the degraded period was to a file the model hadn't read. | Medium. Requires tracking which files were read in current context. |
| 🟡 **PreToolUse** (Bash) | Block destructive commands, enforce git safety | **Indirect.** Prevents catastrophic errors from shallow reasoning. | Low. Pattern matching on commands. |
| 🟡 **UserPromptSubmit** | Inject context reminders, conventions, or rules at each prompt | **Indirect.** Compensates for convention drift by re-stating critical rules. | Low. Cannot block (exit code ignored). |
| 🟡 **PostToolUse** | Log tool usage for monitoring (read:edit ratio tracking) | **Monitoring.** Doesn't affect quality directly but enables detection. | Low. Log to file, analyze offline. |
| 🟢 **SessionStart** | Environment setup, convention injection, dependency checks | **Indirect.** Ensures consistent starting state. | Low. |

---

## 6. Monitoring & Observability

These settings help detect degradation rather than prevent it.

| Setting | Type | Recommended | What it provides |
|---------|------|-------------|-----------------|
| 🔴 **`showThinkingSummaries: true`** | Settings.json | Enable | Makes thinking visible — essential for detecting depth changes |
| 🟡 **`CLAUDE_CODE_ENABLE_TELEMETRY=1`** + OTel config | Env var | Enable if you have OTel infrastructure | Structured metrics and logging for programmatic monitoring |
| 🟡 **PostToolUse logging hook** | Hook | Implement | Track read:edit ratios, tool call patterns, file access sequences per session |
| 🟡 **Stop hook violation logging** | Hook | Implement | Count premature stop attempts as a quality canary signal |
| 🟢 **`CLAUDE_CODE_DEBUG_LOG_LEVEL=verbose`** | Env var | Enable during testing | Full diagnostic output including status line data |

---

## 7. Settings That Should NOT Be Changed

| Setting | Why leave it alone |
|---------|-------------------|
| `DISABLE_COMPACT` | Disabling compaction causes sessions to fail when context fills. Never set this. |
| `CLAUDE_CODE_DISABLE_THINKING` / `MAX_THINKING_TOKENS=0` | Disabling thinking entirely removes all reasoning capability. |
| `CLAUDE_CODE_DISABLE_CLAUDE_MDS` | Disabling CLAUDE.md loading removes your primary instruction channel. |
| Very aggressive `.claudeignore` | Over-ignoring files means the model can't read code it needs. Be surgical. |

---

## 8. Recommended Baseline Configuration

This is a starting point for quality-first workflows. Test each setting independently before combining.

**Important limitation:** Boris Cherny confirmed that effort=high does not fully prevent zero-thinking turns on models with adaptive reasoning. The baseline below is necessary but may not be sufficient. Hooks (Section 5) are the primary defence against the consequences of zero-reasoning turns — they catch the symptoms (premature stopping, edit-without-reading) that settings alone cannot prevent on current models.

### settings.json (user-level: `~/.claude/settings.json`)
```json
{
  "showThinkingSummaries": true,
  "effortLevel": "high",
  "model": "opus"
}
```

### Environment variables (in `.zshrc` / `.bashrc` or settings.json `env` key)
```bash
# Effort — override if settings aren't respected
export CLAUDE_CODE_EFFORT_LEVEL=high

# Context management — compact earlier to avoid late-window degradation
export CLAUDE_AUTOCOMPACT_PCT_OVERRIDE=60

# Optional: shrink effective context window on 1M models
# export CLAUDE_CODE_AUTO_COMPACT_WINDOW=500000

# Optional: pin model to prevent surprise upgrades
# export ANTHROPIC_DEFAULT_OPUS_MODEL=claude-opus-4-6

# Optional: disable adaptive thinking on 4.6 for fixed budget control
# export CLAUDE_CODE_DISABLE_ADAPTIVE_THINKING=1
# export MAX_THINKING_TOKENS=20000

# Monitoring
export CLAUDE_CODE_DEBUG_LOG_LEVEL=info
```

### .claudeignore (project root)
```
# Build artifacts
build/
dist/
out/
*.min.js
*.min.css

# Dependencies
node_modules/
vendor/
.venv/

# Generated files
*.lock
package-lock.json
yarn.lock

# Large data
*.sqlite
*.db
*.csv
*.parquet

# IDE / OS
.idea/
.vscode/
.DS_Store
```

---

## 9. Testing Protocol

When changing settings, test one at a time against a consistent task:

1. **Baseline**: Run a representative task with current settings. Note: completion quality, number of corrections needed, time to completion.
2. **Change one setting**: Apply the change.
3. **Re-run the same task**: Compare quality, corrections, time.
4. **Record results**: Update this matrix with your findings.

For hooks, test with logging-only mode first (exit 0 always, but log matches), then enable blocking (exit 1) after validating the patterns.

---

## Appendix: Where Settings Are Applied

| Level | Location | Scope | Precedence |
|-------|----------|-------|------------|
| Managed | Server-managed or policy settings | Organisation-wide | Highest |
| User | `~/.claude/settings.json` | All your projects | Medium |
| Project | `.claude/settings.json` (committed) | Shared with team | Medium |
| Env var | Shell or `env` key in settings | Depends on where set | Varies (some override everything) |
| CLI flag | `claude --flag` | Single session | Varies |
| In-session | `/command` | Current session | Lowest (except some persist) |

Arrays (like permissions) are **merged** across levels. Scalar values (like model) are **overridden** by higher precedence.
