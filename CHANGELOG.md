# Changelog

All notable changes to this project will be documented in this file.

## [1.1.0] - 2026-03-21

### Features
- Fix hook reliability, add short ID prefix resolution (#13) ([92c613c](https://github.com/coetzeevs/cerebro/commit/92c613c5481587bfd36c412e9fb31103959645d2)) ([#13](https://github.com/coetzeevs/cerebro/pull/13))

## [1.0.2] - 2026-03-11

### Bug Fixes
- Ad-hoc codesign binaries and strip quarantine on install (#12) ([c94a8ec](https://github.com/coetzeevs/cerebro/commit/c94a8ecb0eb766b642d6e2dee5c537c861257f75)) ([#12](https://github.com/coetzeevs/cerebro/pull/12))

## [1.0.1] - 2026-03-11

### Miscellaneous
- Configure Homebrew cask publishing via GoReleaser (#11) ([aed32e2](https://github.com/coetzeevs/cerebro/commit/aed32e2b4548c0fcbfd92467bc2313b6727caf62)) ([#11](https://github.com/coetzeevs/cerebro/pull/11))

## [1.0.0] - 2026-03-11

### Bug Fixes
- Use cosine distance metric in vec0 for correct similarity scores ([f0299af](https://github.com/coetzeevs/cerebro/commit/f0299aff5f260cfceed965d57198b097f4a257b0))
- Check AddNode error returns in test to satisfy errcheck ([10216a5](https://github.com/coetzeevs/cerebro/commit/10216a50a331c9ee9c13064190b260e7194a4104))
- Lint issues (gofmt, rangeValCopy) and install pre-commit hooks ([0d921ac](https://github.com/coetzeevs/cerebro/commit/0d921acace407ac04265edb16c0febc0f5b8ad1a))
- Add compact and clear matchers to SessionStart hooks ([c87d206](https://github.com/coetzeevs/cerebro/commit/c87d206cb8f8d3aec251bf31dfa15794c695c075))
- Resolve lint issues surfaced by golangci-lint v2 ([e23d334](https://github.com/coetzeevs/cerebro/commit/e23d3342f80bd3245286e31250d06299dbcfebe3))
- Upgrade golangci-lint-action to v7 for lint v2 support ([c898c99](https://github.com/coetzeevs/cerebro/commit/c898c997dcb19ac052090b280bd499a2dbc0bd82))
- Golangci-lint v2 config and nolint explanations ([8ad89ba](https://github.com/coetzeevs/cerebro/commit/8ad89babbed76b8dedb9d586b8e3d865d96be716))

### Documentation
- Add future musings, v0 gaps, gitignore settings.local.json ([50c40d8](https://github.com/coetzeevs/cerebro/commit/50c40d8790f956208f1637304eed72cfd609f1e2))

### Features
- Integrate sqlite-vec for vector search ([9fceba8](https://github.com/coetzeevs/cerebro/commit/9fceba8820137537af25985decdb6ce590b6e027))
- Add Claude Code integration layer (hooks, skills, CLAUDE.md) ([f3ae7be](https://github.com/coetzeevs/cerebro/commit/f3ae7be86721b50de9517e79e473d1ad8b1aa2db))
- Support recall --prime without query for session-start priming ([7c49dbb](https://github.com/coetzeevs/cerebro/commit/7c49dbb6eb29cc97574f51c7286752b01c0abff6))
- Implement GC eviction, fix hooks, stratified recall --prime ([4ab2e8d](https://github.com/coetzeevs/cerebro/commit/4ab2e8dbcf67e1c0307d3d6bb7206e8025be1c13))
- Implement graph expansion for composite scoring ([9d32b8e](https://github.com/coetzeevs/cerebro/commit/9d32b8e95ed4f796cf5a307259a7c8c9374c36b2))
- Global store, promote command, and dual-store recall ([99c8792](https://github.com/coetzeevs/cerebro/commit/99c879219c4790ac7c6b027df76f5b845a1b82b2))
- Implement export and import commands ([5603f20](https://github.com/coetzeevs/cerebro/commit/5603f2072572a07ffcc7f4108b1d4dc975199a7a))
- Cerebro init bootstraps Claude Code integration ([de036df](https://github.com/coetzeevs/cerebro/commit/de036df8f26bdbc5f27e99ce6f8b41d228f070e8))

## [0.1.0] - 2026-03-06

### Documentation
- Initial architecture design for Cerebro agent brain ([40baaed](https://github.com/coetzeevs/cerebro/commit/40baaed4f14ff9f085b79d806995efd658b9623c))
- Resolve open questions — Go, opportunistic triggers, scoping ([b77a2a1](https://github.com/coetzeevs/cerebro/commit/b77a2a1d6c14c9b610201de4fd1f3858ea212759))
- Add Claude Code integration pattern (ADR-006) and align all docs ([66724fc](https://github.com/coetzeevs/cerebro/commit/66724fc5973203c6a7d28c5a0edfbef62a4fb80c))
- Add work tracking approach and dogfooding note to CLAUDE.md ([0580ec1](https://github.com/coetzeevs/cerebro/commit/0580ec191efa871f8c4ab70d278d744a67404ed0))

### Features
- Add Go scaffold with store, brain API, embedding providers, and CLI ([a43747c](https://github.com/coetzeevs/cerebro/commit/a43747ce864a0f6f8f8ee11055a2ec154e50105c))
- Add CI, pre-commit hooks, golangci-lint, goreleaser, and tests ([babd68e](https://github.com/coetzeevs/cerebro/commit/babd68efc018a878f82e39627777fac55772ce34))

### Miscellaneous
- Move init doc to subfolder and add note ([79e7c6a](https://github.com/coetzeevs/cerebro/commit/79e7c6ae6dd57e8a76799ef842b8535044b0865a))

