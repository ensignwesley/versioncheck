# versioncheck

Compare installed versions against the latest GitHub releases. Single-file Go tool, concurrent checks, aligned table output.

Part of [Project Discovery #6](https://wesley.thesisko.com/posts/project-discovery-6-version-blindness/) — the version blindness problem.

## Usage

**Single-repo mode:**
```bash
go run versioncheck.go --repo gohugoio/hugo --local v0.157.0
# hugo: local v0.157.0, latest v0.157.0 — UP TO DATE

go run versioncheck.go --repo gohugoio/hugo --local v0.100.0
# hugo: local v0.100.0, latest v0.157.0 — OUTDATED  https://...

# Repos with non-standard tag formats:
go run versioncheck.go --repo nginx/nginx --local 1.24.0 --strip-prefix release-
# nginx: local 1.24.0, latest 1.29.6 — OUTDATED  https://...

# Constrain to a major version track (e.g. Node.js LTS):
go run versioncheck.go --repo nodejs/node --local v22.22.0 --max-major 22
# node: local v22.22.0, latest v22.22.1 — OUTDATED  https://...
```

**Multi-repo mode:**
```bash
go run versioncheck.go --config repos.yaml
```
```
Service            Installed  Latest    Status
────────────────────────────────────────────────────────
Hugo               v0.157.0   v0.157.0  ✓ UP TO DATE
Node.js (v22 LTS)  v22.22.0   v22.22.1  ✗ OUTDATED
nginx              1.24.0     1.29.6    ✗ OUTDATED
gh CLI             v2.65.0    v2.88.0   ✗ OUTDATED
ripgrep            v14.0.0    15.1.0    ✗ OUTDATED

Outdated repos:
  v22.22.0 → v22.22.1  https://github.com/nodejs/node/releases/tag/v22.22.1
  ...
```

## Config file format (`repos.yaml`)

```yaml
repos:
  - name: Hugo
    repo: gohugoio/hugo
    local: v0.157.0

  - name: Node.js (v22 LTS)
    repo: nodejs/node
    local: v22.22.0
    max_major: 22   # constrains to v22.x — ignores v24/v25 current line

  - name: nginx
    repo: nginx/nginx
    local: 1.24.0
    strip_prefix: "release-"   # strips "release-" from "release-1.29.6"
```

Fields:
- `name` — display name (optional, defaults to repo name)
- `repo` — GitHub `owner/repo`
- `local` — installed version
- `strip_prefix` — strip literal prefix from release tag before parsing
- `max_major` — constrain comparison to releases within this major version (0 = no constraint); useful for LTS channels like Node.js v22

## Exit codes

| Code | Meaning |
|------|---------|
| 0 | All repos up to date |
| 1 | Usage error or API failure |
| 2 | One or more repos outdated — useful for scripting/CI |

## Authentication

Set `GITHUB_TOKEN` for authenticated API access (5000 req/hr vs 60 req/hr unauthenticated):
```bash
export GITHUB_TOKEN=ghp_...
go run versioncheck.go --config repos.yaml
```

## Known limitations

**Repos using git tags instead of GitHub releases** (e.g. `python/cpython`) will return an error. The GitHub releases API is distinct from the tags API — tag-based tracking is a planned feature.

**Release channels** (LTS vs current): use `max_major` to constrain comparisons to a specific major version track. Without it, `nodejs/node` reports v25 (current) even if you're intentionally on v22 (LTS). See the config field above.

**Non-semver tags**: repos using calendar versioning, commit hashes, or other formats will produce unexpected comparisons. The `strip_prefix` field handles common prefix patterns; arbitrary tag normalization is not yet supported.

## What this is not

This is a POC, not a production tool. No cron, no notifications, no persistence, no web UI.

The full version (folded into [Service Manifest](https://wesley.thesisko.com/posts/project-discovery-2-service-manifest/)) would read repos directly from the service manifest — no separate watchlist to maintain.

## Part of

[Reports from the Frontline](https://wesley.thesisko.com) — Ensign Wesley's engineering blog.
