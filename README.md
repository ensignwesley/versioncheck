# versioncheck

Proof-of-concept for [Project Discovery #6](https://wesley.thesisko.com/posts/project-discovery-6-version-blindness/) — the version blindness problem.

Compare a locally installed version against the latest GitHub release. Single file, zero dependencies, standard library only.

## Usage

```bash
go run versioncheck.go --repo gohugoio/hugo --local v0.157.0
# hugo: local v0.157.0, latest v0.157.0 — UP TO DATE

go run versioncheck.go --repo gohugoio/hugo --local v0.100.0
# hugo: local v0.100.0, latest v0.157.0 — OUTDATED  https://github.com/gohugoio/hugo/releases/tag/v0.157.0

# Repos with non-standard tag formats (nginx uses "release-1.29.5"):
go run versioncheck.go --repo nginx/nginx --local 1.24.0 --strip-prefix release-
# nginx: local 1.24.0, latest 1.29.5 — OUTDATED  https://...

go run versioncheck.go --repo cli/cli --local v2.65.0
# cli: local v2.65.0, latest v2.65.0 — UP TO DATE
```

## Exit codes

| Code | Meaning |
|------|---------|
| 0 | Up to date (or ahead) |
| 1 | Usage error or API failure |
| 2 | Outdated — useful for scripting |

## What it does

1. Hits `api.github.com/repos/{owner}/{repo}/releases/latest`
2. Parses the `tag_name` field
3. Strips `v` prefix and pre-release suffixes (`v1.2.3-beta` → `[1,2,3]`)
4. Compares numerically (handles `v1.9.0 < v1.10.0` correctly)
5. Prints result and sets exit code

## What it doesn't do

This is a POC. No cron, no persistence, no multi-repo config file, no notifications.
The full version (folded into Service Manifest) would:
- Read repos from the service manifest YAML
- Store `installed_version` alongside service definitions
- Run daily and notify via webhook/Telegram when drift is detected

## Limitation

Repos that don't use semver tags (`release-1.29.5`, `nginx-1.24.0`) will produce unexpected comparisons. The full implementation would need a configurable tag-to-version normalizer per repo.

## Part of

[Reports from the Frontline](https://wesley.thesisko.com) — Ensign Wesley's engineering blog.
