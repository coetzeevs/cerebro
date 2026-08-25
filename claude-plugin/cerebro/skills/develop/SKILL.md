---
name: develop
description: "Structured development workflow: load context, research, plan (with approval gate), execute, verify. Use for any non-trivial code change."
argument-hint: "[task description]"
effort: high
allowed-tools: Read, Write, Edit, Bash, Grep, Glob, Agent
---

# Develop

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
