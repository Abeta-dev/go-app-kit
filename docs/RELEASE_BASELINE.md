# Release Baseline

This repository's source checkout and its published module releases are distinct
artifacts. Do not infer a release from a version-like tag name, and never move,
delete, or recreate an existing tag or GitHub release.

## Verified published artifacts

The following evidence was captured on 2026-09-17. `scripts/verify_release_baseline.sh`
rechecks it against the remote, Go module proxy, and checksum database without
altering local refs.

| Module | Version | Remote tag form | Tagged commit | Proxy archive SHA-256 | SumDB module checksum |
| --- | --- | --- | --- | --- | --- |
| `github.com/umesh0492/go-app-kit` | `v0.2.1` | annotated (`277ad0741cd723d8c2c445b24dec5608cf1a1c3b`) | `bbb29710fd5622023b9b30bd3c9151d4d5517832` | `11c92c0eabf36fefe0b0a38eceebe8d3f311cda8b72e27edf479a3a64192d325` | `h1:fIcEZxtHfIsYSfaoZPN5+xMGRh6WGltgBMNmkix+mx0=` |
| `github.com/umesh0492/go-app-kit` | `v0.3.0` | lightweight | `a4501b6ee165c475811e6e3c43dab9b20907d25e` | `52036d15c805f2f10f795e868abbe32248f71e210c6f4213524f2a464c1806f4` | `h1:vxh+SnwkUot1nLkWKDUBuhLfEPI877ABFxjOfYYdTiY=` |
| `github.com/umesh0492/go-app-kit` | `v0.3.1` | lightweight | `815823212936738de790eba35118c36acdecfea1` | cached proxy entry (release truth-gate failure) | cached sumdb entry |
| `github.com/umesh0492/go-libs` | `v0.3.0` | annotated (verified in its baseline) | `69e9b0857997380907ad53ec220d36baadbe53e3` | verified in go-libs baseline | `h1:B6rPK2M7OdXc5a3Q8xsxUvJawgf/C4nJVG1KvHwpyXg=` |
| `github.com/umesh0492/go-fintech-india` | `v0.2.3` | lightweight (verified in its baseline) | `55888b8fb529bb6379eb96f9a04ce649b7a08b90` | `17b5e66a4f1164cabca95e4bfef849b956162552f32db7e2e7ea6d57b30f8504` | `h1:f2QB8HdhsxaCRFvu0HWQmu55rqPtxA6/kliAYIYNqtU=` |

The `v0.3.0` tag is a legitimate immutable published artifact, but it points to
a4501b6, which predates main's `v0.2.0` and annotated `v0.2.1` commits. It is
not a release of the current main-line source. Consumers of the source checkout
must not be told to use it, and a release must not overwrite it.

The `v0.3.1` tag is an immutable remote artifact pointing to commit `8158232`. It tripped
the release workflow truth gate because commit 8158232 was tagged before CHANGELOG.md
and README.md were updated to v0.3.1. Because Go module proxies cache tags immutably,
v0.3.1 cannot be re-pointed or overwritten; it remains an immutable entry, and
reconciliation is published monotonically as `v0.3.2`.

## Dependency baseline

The current source requires only verified published dependencies:

- `github.com/umesh0492/go-libs v0.3.0`
- `github.com/umesh0492/go-fintech-india v0.2.3`

There are no `replace` directives in `go.mod`. Use a caller-owned `go.work` file
for local companion development rather than editing the published module graph.

## Verification

Run the following from the repository root:

```bash
./scripts/check_version.sh
./scripts/verify_release_baseline.sh
```

`check_version.sh` is deterministic and suitable for pull requests: it checks
version/documentation/package/coverage truth against the checkout. The second
command performs live immutable remote-tag, proxy zip and `go.mod` SHA-256, and
Go checksum database verification. It also creates an isolated `GOWORK=off`
consumer importing the stable `export` package, so neither a checkout nor a
local replacement can satisfy the check. It is intentionally run only on the
scheduled/manual baseline workflow and during release investigations, not on
every pull request.

For a future release, invoke `./scripts/verify_release.sh vX.Y.Z` after the tag
has been pushed and the module proxy has resolved it. The release workflow uses
that script's bounded, fresh-cache proxy retries and uploads its manifest even
on failure. It creates a missing GitHub release or replaces only the
`release-manifest.json` asset on an existing release; tags and other assets are
never changed.

## Current release baseline (v0.3.2)

Following the immutable remote caching of `v0.3.1` at commit `8158232`, this repository
reconciled its release baseline to **`v0.3.2`**.
The release includes:
- Upgrading `github.com/umesh0492/go-libs` to `v0.3.0`.
- Implementing `TypeMap() *pgtype.Map` on `*mockRows` in `outbox/outbox_test.go` for pgx/v5 5.11.0 compatibility.
- Introducing structural release guardrails: `scripts/check_tag_readiness.sh`, `scripts/git-hooks/pre-push`, and `make tag-release` to prevent untracked or unverified tags from being cut.
