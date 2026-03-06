# ADR-004: Project-Scoped Memory with Global Promotion

## Status
Proposed

## Context

Cerebro serves as memory for an AI orchestrator. The orchestrator works across multiple projects (codebases, repositories). Some knowledge is project-specific ("this project uses PostgreSQL 16"), while other knowledge is universal ("user prefers concise responses").

We need a scoping model that:
1. Isolates project-specific knowledge so it doesn't leak into unrelated projects
2. Enables useful knowledge to be shared across all projects
3. Follows a familiar precedence model (local overrides global)
4. Maintains the single-file portability of each project's brain

## Decision

Use a **multi-file architecture** with separate SQLite databases for project and global memory:

```
~/.cerebro/
  config.toml                     # Global configuration
  global.sqlite                   # Global memory store
  projects/
    <sha256_of_abs_path>.sqlite   # Project-specific brain
```

Each project's brain is initialized when the orchestrator is first invoked in that directory. The global brain is created on first use.

At query time, both stores are consulted with the project store taking precedence (weight 1.0) over global (weight 0.7). Promotion from project to global is explicit and involves content generalization.

## Alternatives Considered

### 1. Single database with namespace column
```sql
ALTER TABLE nodes ADD COLUMN scope TEXT;  -- 'project' or 'global'
ALTER TABLE nodes ADD COLUMN project_id TEXT;
```
- **Rejected because:** Breaks the single-file-per-project portability. A single database containing all projects can't be easily shared, backed up per-project, or placed inside a project directory. Complicates queries (every query needs scope filters). Larger file = slower for any individual project. Cross-project data leakage risk through query bugs.

### 2. Brain file inside each project directory (`.cerebro/brain.sqlite`)
- **Considered and deferred.** Pros: visible, co-located, easy to find. Cons: risks accidental git commits (needs .gitignore), clutters project directories, doesn't work for projects where user doesn't have write access. May be offered as an option alongside the centralized approach.

### 3. No global memory (project-only)
- **Rejected because:** User preferences and general knowledge would need to be re-learned for every new project. The orchestrator would have no memory of the user's working style when starting a fresh project.

### 4. Always-shared memory (single brain for everything)
- **Rejected because:** Project-specific knowledge would pollute other projects. "This project uses PostgreSQL" is irrelevant (and potentially misleading) when working on a project that uses MongoDB. No isolation boundary means search quality degrades as projects accumulate.

## Promotion Criteria

### Automatic (no confirmation)
- **User preferences** with no project-specific references: "user prefers TypeScript over JavaScript"
- **General tool/technology knowledge**: "pytest `-x` flag stops on first failure"
- **Communication patterns**: "user prefers concise responses with code examples"

Detection signal: Node type is `preference` or general `concept`, content contains no project-specific paths/names/identifiers, and has been reinforced across multiple sessions.

### Require Human Confirmation
- **Cross-project patterns**: same lesson learned in 2+ projects independently
- **Technology knowledge** that might be project-specific vs general

### Never Promote
- Project-specific architecture, entities, file paths, dependency graphs
- Project-specific procedures (deploy commands, build steps)
- Project-specific relationships between components

## Query Precedence

```
1. Query project store  → results weighted at 1.0
2. Query global store   → results weighted at 0.7
3. Merge results
4. Deduplicate: cosine similarity > 0.9 between results = duplicate → keep project version
5. Return top-K by weighted composite score
```

This follows the git-config mental model: `~/.gitconfig` (global) provides defaults, `.git/config` (project) overrides. Users already understand this precedence intuitively.

## Consequences

### Positive
- **Clean isolation.** Each project's brain contains only relevant knowledge.
- **Portable.** A project brain can be copied, shared, or backed up independently.
- **Familiar model.** git-config-style precedence is well understood.
- **Graceful cold start.** New projects inherit global knowledge from day one (user preferences, general patterns).
- **No cross-contamination.** PostgreSQL knowledge from Project A won't confuse search results in MongoDB Project B.

### Negative
- **Two files to manage.** The orchestrator must open and query two SQLite databases at session start. Slight added complexity.
- **Promotion overhead.** Identifying and promoting memories requires LLM calls for content generalization and conflict checking.
- **Disk usage.** The global brain stores some content that also exists (in project-specific form) in project brains. Minor duplication.

### Risks
- **Incorrect promotion.** Project-specific knowledge might be incorrectly generalized and promoted to global, then mislead other projects. Mitigation: conservative promotion criteria, human confirmation for ambiguous cases, ability to demote/remove global memories.
- **Orphaned project brains.** If a project directory is deleted, its brain file remains in `~/.cerebro/projects/`. Mitigation: `cerebro prune` command to identify and clean up orphaned brains.
- **Hash collisions.** SHA-256 of absolute paths is extremely unlikely to collide but theoretically possible. Mitigation: store the original path in the SQLite file's `schema_meta` table for verification.

## Schema Impact

Each brain.sqlite file includes a `schema_meta` table with project context:

```sql
INSERT INTO schema_meta VALUES ('project_path', '/absolute/path/to/project');
INSERT INTO schema_meta VALUES ('project_hash', '<sha256>');
INSERT INTO schema_meta VALUES ('scope', 'project');  -- or 'global'
```

The `promoted_to` edge relation links project nodes to their global counterparts:
```sql
-- In project brain
INSERT INTO edges VALUES (
    'concept_local_123',
    'global:concept_456',  -- Prefixed to indicate it's in the global store
    'promoted_to',
    1.0, NULL, CURRENT_TIMESTAMP
);
```

## References
- [Research: Agent memory architectures — Section 5](../research/agent-memory-architectures-research.md)
- Git configuration scoping: https://git-scm.com/docs/git-config
- VS Code settings precedence: User Settings < Workspace Settings < Folder Settings
