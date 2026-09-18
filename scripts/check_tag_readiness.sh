#!/usr/bin/env bash
set -euo pipefail

# Validates repository, documentation, and test state before creating or pushing a release tag.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${ROOT_DIR}"

readonly SEMVER_PATTERN='[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?'

fail() {
  printf '\033[0;31mERROR: %s\033[0m\n' "$*" >&2
  exit 1
}

tag="${1:-}"
if [[ -z "${tag}" ]]; then
  fail "Usage: $0 <vX.Y.Z>"
fi

if [[ ! "${tag}" =~ ^v${SEMVER_PATTERN}$ ]]; then
  fail "Tag must be valid SemVer prefixed with 'v' (e.g. v0.3.2): ${tag}"
fi

version="${tag#v}"

echo "========================================================"
echo "Checking release tag readiness for: ${tag}"
echo "========================================================"

# 1. Validates that the working tree is clean.
echo "-> Checking working tree status..."
if [[ -n "$(git status --porcelain)" ]]; then
  git status --short >&2
  fail "Working tree has uncommitted or untracked changes. Commit or stash them before releasing."
fi

# 2. Validates that CHANGELOG.md has an entry for this exact version as its top released section.
echo "-> Validating CHANGELOG.md top released version..."
top_changelog_version="$(grep -E "^## \[${SEMVER_PATTERN}\]" CHANGELOG.md | head -n1 | sed -E "s/^## \[(${SEMVER_PATTERN})\].*/\1/")"
if [[ "${top_changelog_version}" != "${version}" ]]; then
  fail "Top CHANGELOG.md release entry is v${top_changelog_version:-none}, but expected v${version}"
fi

# 3. Validates that README.md has this exact version.
echo "-> Validating README.md version references..."
readme_pub="$(sed -n -E "s/^> Current main-line published release: \`v(${SEMVER_PATTERN})\`\..*/\1/p" README.md | head -n1)"
readme_inst="$(sed -n -E "s|.*github\.com/umesh0492/go-app-kit@v(${SEMVER_PATTERN}).*|\1|p" README.md | head -n1)"
if [[ "${readme_pub}" != "${version}" ]]; then
  fail "README.md published release header is v${readme_pub:-none}, but expected v${version}"
fi
if [[ "${readme_inst}" != "${version}" ]]; then
  fail "README.md installation snippet is v${readme_inst:-none}, but expected v${version}"
fi

# 4. Runs GIT_TAG=$1 ./scripts/check_version.sh.
echo "-> Running check_version.sh for ${tag}..."
GIT_TAG="${tag}" ./scripts/check_version.sh

# 5. Runs ./scripts/test_bash_compat.sh.
echo "-> Running test_bash_compat.sh..."
./scripts/test_bash_compat.sh

# 6. Runs go test -race ./...
echo "-> Running go test -race ./..."
go test -race ./...

echo "========================================================"
printf '\033[0;32m✅ Release readiness confirmed! Tag %s is ready to be created and pushed.\033[0m\n' "${tag}"
echo "========================================================"
