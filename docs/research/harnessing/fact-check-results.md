# Fact-Check: Claude Code Hooks & Skills Claims

Verified against official Claude Code documentation via Context7 MCP server
(sources: `/anthropics/claude-code` and `/ericbuess/claude-code-docs`).

Date: 2026-04-22

---

## 1. Personal Skills Scope

**Claim**: Personal skills exist at `~/.claude/skills/` and are available across all projects.

**CONFIRMED.**

Documentation states:
> Skills can be placed in Enterprise settings for organization-wide use, in a personal directory for use across all your projects, or in a project-specific directory for use only within that project.

Filesystem verification commands from docs:
```bash
# Check project Skills
ls .claude/skills/*/SKILL.md

# Check personal Skills
ls ~/.claude/skills/*/SKILL.md
```

**Precedence order**: Enterprise > Personal > Project. Plugin skills use a `plugin-name:skill-name` namespace and do not conflict.

Documentation states:
> When multiple skills share the same name, a priority system determines which skill is used: enterprise skills take precedence over personal skills, which in turn take precedence over project skills. Plugin skills, using a `plugin-name:skill-name` format, do not conflict with skills at other levels.

---

## 2. Skill Frontmatter Fields

**Claim**: Check for `effort`, `model`, `hooks`, `context`, `allowed-tools`, `disable-model-invocation`.

**ALL SIX CONFIRMED** as valid frontmatter fields.

Documentation evidence for each:

### `description` (recommended)
> providing a description is recommended to help Claude understand the purpose of the skill

### `name`
Used as identifier. Example: `name: my-skill`

### `model`
> Advanced configuration options include setting the model
Example: `model: sonnet`

### `effort`
> defining the effort level for the skill
Example: `effort: medium` (seen in agent frontmatter reference)

### `context`
> using the context field to run the skill within a forked subagent environment
Example: `context: fork`

### `allowed-tools`
> allowed-tools specifies which tools Claude can use without additional permission when the skill is active
Example: `allowed-tools: Read Grep`

### `disable-model-invocation`
> disable-model-invocation prevents automatic loading
Example: `disable-model-invocation: true`

### `user-invocable`
> user-invocable hides the skill from the menu

### `hooks`
**CONFIRMED** as a valid frontmatter field for skills.
> Hooks can be defined within skills and subagents using frontmatter. These hooks are scoped to the component's lifecycle and only run when that specific component is active.

Example from docs:
```yaml
---
name: secure-operations
description: Perform operations with security checks
hooks:
  PreToolUse:
    - matcher: "Bash"
      hooks:
        - type: command
          command: "./scripts/security-check.sh"
---
```

**Summary**: All six queried fields (`effort`, `model`, `hooks`, `context`, `allowed-tools`, `disable-model-invocation`) are documented. Two additional fields (`name`, `user-invocable`) also exist. All fields are optional.

---

## 3. Stop Hook Decision Protocol

**Claim A**: Exit code 2 with stderr blocks the action.
**Claim B**: Exit 0 with JSON `{"decision": "block"}` on stdout blocks the action.

**BOTH ARE VALID but serve different mechanisms.**

### Exit code protocol (command hooks)

Documentation states:
> - **0 (Success)**: Claude Code parses stdout for JSON output.
> - **2 (Blocking Error)**: Claude Code ignores stdout. Stderr is fed back to Claude as an error message, blocking the associated action.
> - **Other**: Non-blocking error. A notice is shown in the transcript, and execution continues.

Example:
```bash
echo "Blocked: rm commands are not allowed" >&2
exit 2  # Blocking error: tool call is prevented
```

### JSON output protocol (exit 0 + stdout JSON)

Documentation states:
> JSON output provides finer-grained control over tool execution compared to exit codes. By exiting with code 0 and printing a JSON object to stdout, you can influence Claude Code behavior, including blocking, allowing, or escalating actions to the user.

For **Stop hooks specifically**, the decision format is:
```json
{
  "decision": "block",
  "reason": "Must be provided when Claude is blocked from stopping"
}
```

Documentation for Stop hook decision control:
> `decision` -- `"block"` prevents Claude from stopping. Omit to allow Claude to stop.
> `reason` -- Required when `decision` is `"block"`. Tells Claude why it should continue.

**IMPORTANT**: The docs also show a third field `systemMessage` in the plugin-dev reference:
```json
{
  "decision": "approve|block",
  "reason": "Explanation",
  "systemMessage": "Additional context"
}
```

However the hooks.md reference only documents `decision` and `reason` for Stop hooks and does NOT mention `"approve"` as a decision value -- it says to **omit** the decision field to allow stopping.

**Conclusion**: Both exit-code-2 and JSON-on-stdout are valid blocking mechanisms. For Stop hooks, the JSON protocol uses `{"decision": "block", "reason": "..."}`. The `"approve"` value appears only in the plugin-dev skill documentation (which is a teaching/example resource), while the canonical hooks.md says to simply omit the decision field to allow stopping.

---

## 4. Stop Hook Input Format

**Claim**: Check for `stop_message` field.

**NO `stop_message` FIELD EXISTS.** The field is called `last_assistant_message`.

Documentation states:
> In addition to common input fields, Stop hooks receive `stop_hook_active` and `last_assistant_message`. The `stop_hook_active` field is `true` when Claude Code is already continuing as a result of a stop hook. The `last_assistant_message` field contains the text content of Claude's final response, so hooks can access it without parsing the transcript file.

Full Stop hook input JSON:
```json
{
  "session_id": "abc123",
  "transcript_path": "~/.claude/projects/.../00893aaf-19fa-41d2-8238-13269b9b3ca0.jsonl",
  "cwd": "/Users/...",
  "permission_mode": "default",
  "hook_event_name": "Stop",
  "stop_hook_active": true,
  "last_assistant_message": "I've completed the refactoring. Here's a summary..."
}
```

Common fields present in all hook inputs:
- `session_id`
- `transcript_path`
- `cwd`
- `permission_mode`
- `hook_event_name`

Stop-specific fields:
- `stop_hook_active` (boolean)
- `last_assistant_message` (string)

---

## 5. Skill Compaction Survival

**Claim**: Skills survive compaction with a 5K per-skill / 25K total token budget.

**CONFIRMED.**

Documentation states:
> Invoked skill bodies are re-injected after compaction, subject to a 5,000-token cap per skill and a 25,000-token total limit. Because truncation preserves the beginning of a file, it is recommended to place the most critical instructions at the top of SKILL.md files to ensure they remain within the context window.

---

## 6. Hook Event Types

**Claim**: How many hook event types exist?

**At least 20 documented event types** (from the plugins-reference available-events table):

| # | Event                | When it fires |
|---|----------------------|---------------|
| 1 | `SessionStart`       | When a session begins or resumes |
| 2 | `UserPromptSubmit`   | When you submit a prompt, before Claude processes it |
| 3 | `PreToolUse`         | Before a tool call executes. Can block it |
| 4 | `PermissionRequest`  | When a permission dialog appears |
| 5 | `PermissionDenied`   | When a tool call is denied by the auto mode classifier |
| 6 | `PostToolUse`        | After a tool call succeeds |
| 7 | `PostToolUseFailure` | After a tool call fails |
| 8 | `Notification`       | When Claude Code sends a notification |
| 9 | `SubagentStart`      | When a subagent is spawned |
| 10 | `SubagentStop`      | When a subagent finishes |
| 11 | `TaskCreated`       | When a task is being created via TaskCreate |
| 12 | `TaskCompleted`     | When a task is being marked as completed |
| 13 | `Stop`              | When Claude finishes responding |
| 14 | `StopFailure`       | When the turn ends due to an API error |
| 15 | `TeammateIdle`      | When an agent team teammate is about to go idle |
| 16 | `InstructionsLoaded`| When a CLAUDE.md or rules file is loaded into context |
| 17 | `ConfigChange`      | When a configuration file changes during a session |
| 18 | `CwdChanged`        | When the working directory changes |
| 19 | `FileChanged`       | When a watched file changes on disk |
| 20 | `WorktreeCreate`    | When a worktree is being created |
| 21 | `WorktreeRemove`    | When a worktree is being removed |
| 22 | `PreCompact`        | Before context compaction |

The Python SDK enum also lists these (subset):
```python
HookEvent = Literal[
    "PreToolUse", "PostToolUse", "PostToolUseFailure",
    "UserPromptSubmit", "Stop", "SubagentStop",
    "PreCompact", "Notification", "SubagentStart",
    "PermissionRequest",
]
```

The Python SDK enum is a subset of the full list. The canonical complete list from the plugins-reference documentation contains **22 event types**.

**Note**: `SessionEnd` is mentioned in the lifecycle section ("once per session: SessionStart, SessionEnd") but does NOT appear in the available-events table. It may be an internal event. The Python SDK does not include it either.

---

## 7. Stop Hooks in VSCode

**Claim**: There is a known issue with Stop hooks not firing in VSCode.

**NOT CONFIRMED by documentation.**

The documentation does describe general reasons hooks may not fire:
> If a configured hook is not firing, first verify its presence under the correct event using `/hooks`. Ensure the matcher pattern precisely matches the tool name, as matchers are case-sensitive.

And specific Stop hook limitations:
> `Stop` hooks are triggered when Claude finishes responding, not exclusively upon task completion, and are not fired by user interrupts.
> Hooks may not fire when the agent hits the `max_turns` limit because the session ends before hooks can execute.

**No VSCode-specific issue is mentioned in the documentation.** The docs do not differentiate behavior between terminal and VSCode extension for hooks. If such a bug exists, it is not documented in the official sources queried.

---

## 8. Agent Hooks

**Claim**: Agent-type hooks are experimental.

**NOT CONFIRMED as experimental.** The documentation describes agent hooks as a standard hook type without any experimental label.

Four hook types are documented:
> There are four types of hooks available: `command` for running shell commands, `http` for sending data to a URL, `prompt` for single-turn LLM evaluations, and `agent` for multi-turn verification with tool access. The `command` type is the most common.

Agent hook configuration:
> Configure an agent hook by setting `type` to `"agent"` and providing a `prompt` string. The configuration fields are the same as prompt hooks, with a longer default timeout.

Agent hook parameters:
- `type`: `"agent"` (required)
- `prompt`: string describing what to verify (required). `$ARGUMENTS` placeholder for hook input JSON
- `model`: optional, defaults to a fast model
- `timeout`: optional, default 60 seconds

Agent hooks support up to 50 tool-use turns.

**The documentation does not label agent hooks as experimental, beta, or unstable.** They are documented alongside command, http, and prompt hooks as a standard option. Events that support all four hook types (including agent): `PermissionRequest`, `PostToolUse`, `PostToolUseFailure`, `PreToolUse`, `Stop`, `SubagentStop`, `TaskCompleted`, `TaskCreated`, and `UserPromptSubmit`.

---

## Summary Table

| # | Claim | Verdict |
|---|-------|---------|
| 1 | Personal skills at `~/.claude/skills/`, available across projects | CONFIRMED |
| 1b | Precedence: enterprise > personal > project | CONFIRMED |
| 2 | `effort`, `model`, `hooks`, `context`, `allowed-tools`, `disable-model-invocation` | ALL CONFIRMED |
| 3 | Stop hooks block via exit code 2 + stderr | CONFIRMED |
| 3b | Stop hooks block via exit 0 + JSON `{"decision":"block"}` on stdout | CONFIRMED |
| 3c | Both mechanisms are valid | CONFIRMED (different levels of control) |
| 4 | Stop hook input has `stop_message` field | DENIED -- field is `last_assistant_message` |
| 5 | Skills survive compaction (5K/skill, 25K total) | CONFIRMED |
| 6 | Hook event types count | 22 documented event types |
| 7 | Stop hooks don't fire in VSCode (known issue) | NOT FOUND in documentation |
| 8 | Agent hooks are experimental | NOT FOUND -- documented as standard hook type |
