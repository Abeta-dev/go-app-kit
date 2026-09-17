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
| `github.com/umesh0492/go-libs` | `v0.2.1` | annotated (verified in its baseline) | `b75acc6d82e47189ef174a4ea80134fed8cd392f` | `d519b6624138503f1cbe8f3de071f0fe685b521ad5f1f9634a0a85a1992fc9db` | `h1:9Fzm2GZMkd+rnFAFOd5MURnOorqM98O4H/zcVUb6uoY=` |
| `github.com/umesh0492/go-fintech-india` | `v0.2.3` | lightweight (verified in its baseline) | `55888b8fb529bb6379eb96f9a04ce649b7a08b90` | `17b5e66a4f1164cabca95e4bfef849b956162552f32db7e2e7ea6d57b30f8504` | `h1:f2QB8HdhsxaCRFvu0HWQmu55rqPtxA6/kliAYIYNqtU=` |

The `v0.3.0` tag is a legitimate immutable published artifact, but it points to
a4501b6, which predates main's `v0.2.0` and annotated `v0.2.1` commits. It is
not a release of the current main-line source. Consumers of the source checkout
must not be told to use it, and a release must not overwrite it.

## Dependency baseline

The current source requires only verified published dependencies:

- `github.com/umesh0492/go-libs v0.2.1`
- `github.com/umesh0492/go-fintech-india v0.2.3`

There are no `replace` directives in `go.mod`. Use a caller-owned `go.work` file
for local companion development rather than editing the published module graph.

## Verification

Run the following from the repository root:

```bash
./scripts/check_version.sh
./scripts/verify_release_baseline.sh
```

The second command validates immutable remote tags, proxy zip and `go.mod`
SHA-256 values, and Go checksum database identities for the published baseline.
It also creates an isolated consumer with `GOWORK=off` and builds imports from
the module proxy, so neither a checkout nor a local replacement can satisfy the
check.

For a future release, invoke the same script with the new tag after it has been
pushed and the module proxy has resolved it. The release workflow validates
source, tag identity, proxy resolution, and the isolated consumer before it
creates a missing GitHub release. Existing releases are left unchanged.

## Next release recommendation

The source baseline removes an absolute local replacement, upgrades the direct
fintech dependency, and adds release verification. It must be published as a
new version; do not reuse `v0.2.1` or `v0.3.0`. Because `v0.3.0` already exists,
the next monotonic version is **`v0.3.1` or later**. A `v0.3.1` patch release is
appropriate if compatibility checks remain green.
