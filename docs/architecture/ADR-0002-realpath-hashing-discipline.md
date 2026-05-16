# ADR-0002: Realpath Hashing Discipline for brain.ProjectPath

**Status:** Accepted
**Date:** 2026-05-16
**Ticket:** HS-008
**Deciders:** Principal Architect, Tech Lead (Pre-Implementation Review), Security Specialist (Pre-Implementation Review)

---

## Context

Cerebro locates a project's brain SQLite file via `brain.ProjectPath(dir)`, which computes `~/.cerebro/projects/<sha256(filepath.Abs(dir))>.sqlite`. On macOS, `/tmp` is a symlink to `/private/tmp`. Sessions initiated from `/tmp/myproject` produced a different hash — and therefore a different brain file — than sessions initiated from `/private/tmp/myproject`, even though they reference the same on-disk directory. This violates Operational Ontology §5.14 rule 26 ("realpath-resolved path before hash") and caused duplicate brains, silent memory loss, and inconsistent state visible to the operator.

The same defect was present in `internal/metrics.MetricsPath`, which independently duplicated the hash logic without the EvalSymlinks fix.

---

## Decision

### Option A (selected): Silent best-effort EvalSymlinks with Abs fallback

`brain.ProjectPath` and `internal/metrics.MetricsPath` both apply `filepath.EvalSymlinks(filepath.Abs(dir))` before computing the SHA-256 hash. If `EvalSymlinks` fails (path does not exist yet, permissions error, broken symlink), the function falls back to `filepath.Abs(dir)` — preserving the pre-HS-008 hash for that call.

```go
func ProjectPath(projectDir string) string {
    abs, _ := filepath.Abs(projectDir)
    resolved, err := filepath.EvalSymlinks(abs)
    if err != nil {
        resolved = abs // pre-HS-008 fallback for nonexistent paths
    }
    hash := fmt.Sprintf("%x", sha256.Sum256([]byte(resolved)))
    return filepath.Join(cerebroDir(), "projects", hash+".sqlite")
}
```

Resolution order is **Abs-then-EvalSymlinks**, not EvalSymlinks-then-Abs. `filepath.EvalSymlinks` on a relative path is implementation-defined; running `Abs` first provides a deterministic absolute starting point.

### Why not Option B (error-returning): `ProjectPath(s string) (string, error)`

Option B would surface `EvalSymlinks` errors to callers, enabling explicit handling. Rejected because:

1. **SemVer cost**: `v2.0.0` just shipped (OO-013). A second breaking API change within days of the previous one (`OO-011` / `QWX-001`) is operator-hostile.
2. **No actionable failure mode**: The existing `filepath.Abs` error is already silently swallowed (`_ =`). The `EvalSymlinks` failure case (path does not exist) has a valid silent fallback: the CI-bootstrap caller creates a brain before the project directory exists. This is a documented real use case.
3. **The fallback is correct**: For the nonexistent-path case, falling back to `abs` produces a deterministic hash identical to pre-HS-008 behaviour. No data loss, no regression.

### Why not Option C (sister function): Add `ProjectPathResolved(s string) (string, error)`

Option C would preserve backward compatibility at the cost of two separate API paths to the same answer. Rejected as a maintenance smell; callers should not have to choose between the old and new functions.

---

## Scope: Stack-Wide Hardening (N1 fold-in)

`internal/metrics.MetricsPath` independently duplicated the `filepath.Abs + sha256` computation with a comment reading "Mirrors brain.ProjectPath()". Without this fold-in, `.metrics.sqlite` files for `/tmp/proj` and `/private/tmp/proj` would remain separate post-HS-008, re-introducing the rule-26 violation on the metrics path.

**Decision**: Both functions are fixed identically but kept as **separate implementations** (not cross-coupled via an import). This preserves package decoupling — `internal/metrics` must not import `brain`. The algorithm is simple enough (three lines) that duplication is acceptable and safer than coupling.

---

## Migration command: `cerebro migrate --realpath-hashes`

Because `brain.ProjectPath` is a one-way hash function and brains carry no source-path metadata in `schema_meta` (verified: the KV table stores only `embedding_provider`, `embedding_model`, `embedding_dimensions`, `schema_version`, `has_pending_embeddings`, and `config:<key>`), it is impossible to enumerate existing brain files and invert the hash to find their source path. A brain-enumeration migration strategy would require a dictionary attack on every possible path alias — impractical.

Instead, the migration is **path-driven**: it walks the host filesystem from one or more `--scan-root` directories (default `$HOME`) to `--max-depth` levels (default 4), and for each directory it encounters:

1. Computes `oldHash = sha256(filepath.Abs(dir))` and `newHash = sha256(filepath.EvalSymlinks(dir))`.
2. If `oldHash == newHash` (no symlink divergence): skip.
3. Otherwise, reconciles by case:
   - **Case A** (only old brain): atomic `os.Rename` to new hash path, including WAL/SHM companions.
   - **Case B** (only new brain): already migrated, no-op.
   - **Case C** (both): backup both → export old as JSON → import into new with `ConflictSkip` → delete old.
   - **Case D** (neither): no-op.

### Safety properties

- **Mandatory unconditional backup** for Case C before any mutation. No `--no-backup` flag.
- **Process-level lockfile** (`~/.cerebro/migrate.lock`, acquired via `O_EXCL`) prevents two concurrent migration invocations from racing each other.
- **Explicit symlink skip** in the walk: directory entries with `d.Type()&os.ModeSymlink != 0` return `filepath.SkipDir`. The scan root itself (which may be a symlink) is always processed at depth 0 as its unresolved path — this is the primary use case (scanning from `/tmp/myproject`).
- **ConflictSkip merge strategy**: destination (new hash, the active post-HS-008 brain receiving live writes) wins on ID conflict. Source (old hash orphan) contributes only new nodes absent from destination.
- **Idempotent**: every operation is a closed transition (`A → rename → B`, `C → merge+delete → B`). Second run finds only Case B/D and exits with "Nothing to migrate."

### Why path-driven rather than brain-driven

SHA-256 is one-way. A hash-to-path dictionary would require enumerating every possible symlink prefix on the host filesystem — impractical and fragile. Path-driven scanning trades coverage (only directories reachable from scan roots) for tractability. Brains for deleted or unreachable projects are left in place (harmless; zero-cost storage).

---

## Consequences

**Positive:**
- Eliminates the `/tmp` vs `/private/tmp` duplicate-brain class of bugs on macOS permanently at the API boundary.
- Every caller (helpers.go, metrics, qraftworx-cli, pi-init, future callers) inherits correct realpath hashing automatically.
- Public Go signature is unchanged; no SemVer break.
- Migration is one-shot, safe, and reversible (backup preserved).

**Negative / Watchpoints:**
- Callers passing symlinked paths have their brains silently moved to a new hash on the first post-HS-008 call. Operators must run `cerebro migrate --realpath-hashes` to consolidate.
- Migration only consolidates brains for directories reachable from scan roots. Brains for deleted projects or paths outside the default `$HOME` tree are not touched (acceptable: they are inert).

---

## Forward-Looking Deployment-Conditional Postures (PM-D)

These postures are ACCEPTED on the single-operator workstation. Each re-grades if deployment context shifts:

| ID | Surface | Single-workstation | Hosted/multi-tenant |
|----|---------|-------------------|---------------------|
| PM-D-1 | Symlink walk under `$HOME` (CWE-59) | ACCEPTED | Graded HIGH; explicit symlink skip + tighter `--max-depth` |
| PM-D-2 | Path disclosure in migration stdout (CWE-200) | ACCEPTED | Graded MEDIUM; non-operator capture of stdout |
| PM-D-3 | Backup dir inherits store umask 0o750 (CWE-732) | ACCEPTED | Graded MEDIUM; stack-wide hardening ticket needed |
| PM-D-4 | TOCTOU between export and source-delete (CWE-362) | ACCEPTED | Graded LOW under CI parallelism; lockfile mitigates |

---

## References

- Operational Ontology §5.14 rule 26 (realpath-before-hash mandate)
- Operational Ontology §7 rules 27, 28, 29 (CHANGELOG, documentation, PM-D routing)
- HS-008 ticket and Pre-Implementation Reviews (Tech Lead N1-N4, Security S-MED-1/-2/-3)
- ADR-004: Project-scoped memory (hash-based location strategy)
- ADR-009: Safe init and backup (backup discipline referenced by Case C)
