# Release Procedure & Guardrails

This document establishes the mandatory, step-by-step release procedure for `go-app-kit`.

---

## 1. Immutable Release Principles

Go modules rely on public proxy and checksum services (`proxy.golang.org` and `sum.golang.org`).

- **Never Overwrite or Move Tags**: Once a Git tag is pushed to remote, Go module proxies immediately cache the source archive and its cryptographic checksum (`h1:...`). Retagging the same version name to a different commit will break downstream builds with checksum mismatch errors.
- **Monotonic Releases Only**: If a release fails or needs corrections (such as the historical `v0.3.1` incident where a tag was pushed before docs were updated), a new monotonic version (e.g. `v0.3.2`) must be published.
- **Annotated Tags Only**: All release tags must be created as annotated Git tags (`git tag -a vX.Y.Z -m "Release vX.Y.Z"`). Never use lightweight tags for official releases.

---

## 2. Release Guardrails & Automated Checks

To prevent releasing unverified code or untracked changes, multiple layers of defense are active:

1. **Pre-Tag Verification Script** (`scripts/check_tag_readiness.sh`):
   - Verifies the Git working tree is completely clean.
   - Validates that `TAG` argument is valid SemVer (e.g. `v0.3.2`).
   - Ensures `CHANGELOG.md` has an entry for the version as its top released section.
   - Ensures `README.md` published version and installation snippet match.
   - Runs `GIT_TAG=$TAG ./scripts/check_version.sh` (validates package count, DDL, docs, symbol inventory, and coverage).
   - Runs `./scripts/test_bash_compat.sh` (Bash 3.2 macOS compatibility).
   - Runs `go test -race ./...` (concurrency race detector).

2. **Git Pre-Push Hook** (`scripts/git-hooks/pre-push`):
   - Intercepts any `git push` attempting to push a tag `refs/tags/v*`.
   - Executes `GIT_TAG=$tag ./scripts/check_version.sh`.
   - Rejects the push if version gates or test checks fail.

3. **Makefile Targets**:
   - `make install-hooks`: Configures Git to use `scripts/git-hooks`.
   - `make check-release-readiness TAG=vX.Y.Z`: Runs pre-tag validation.
   - `make tag-release TAG=vX.Y.Z`: Runs readiness validation and creates the annotated tag only if all checks pass.

---

## 3. Step-by-Step Release Checklist

### Step 1: Install Git Hooks (One-Time Setup)
```bash
make install-hooks
```

### Step 2: Prepare Release PR on Feature Branch
1. Create a release branch from `main`:
   ```bash
   git checkout -b release/vX.Y.Z
   ```
2. Update documentation and release metadata:
   - **`CHANGELOG.md`**: Add `## [X.Y.Z] - YYYY-MM-DD` at the top of released versions. Document all changes under appropriate categories (`Changed`, `Added`, `Fixed`).
   - **`README.md`**: Update current published release header and `go get github.com/umesh0492/go-app-kit@vX.Y.Z` snippet.
   - **`SECURITY.md`**: Verify that the version series is marked **Active / Current**.
   - **`docs/RELEASE_BASELINE.md`**: Document the release baseline and history.
   - **`scripts/check_version.sh`**: Ensure dependency versions and expectations match `go.mod`.
3. Verify changes locally:
   ```bash
   bash scripts/test_bash_compat.sh
   bash scripts/check_version.sh
   go test -race ./...
   ```
4. Commit and push the branch:
   ```bash
   git add -A
   git commit -m "chore(release): prepare vX.Y.Z release"
   git push origin release/vX.Y.Z
   ```
5. Open a Pull Request into `main` and wait for all CI checks to pass.

### Step 3: Merge PR to `main`
- Merge the Release PR into `main` (Squash or Rebase).

### Step 4: Tag the Release on `main`
1. Switch to `main` and ensure your local branch is synchronized:
   ```bash
   git checkout main
   git pull origin main
   ```
2. Execute the automated release tagging target:
   ```bash
   make tag-release TAG=vX.Y.Z
   ```
   *Note: This command runs `scripts/check_tag_readiness.sh`. It will fail if your working tree is dirty or any version mismatch exists.*

### Step 5: Push the Tag
```bash
git push origin vX.Y.Z
```
*Note: The `pre-push` hook will run a final validation before the tag leaves your machine.*

### Step 6: Verify CI Release Workflow
- Monitor the GitHub Actions **Release** workflow run.
- Ensure that the release manifest and assets are created and published on GitHub.
