#!/usr/bin/env bash
set -euo pipefail

# Version and documentation truth gate. It verifies the current published
# documentation on normal commits and the exact release version when GIT_TAG,
# a GitHub tag ref, or an exact local tag identifies a release checkout.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${ROOT_DIR}"

readonly SEMVER_PATTERN='[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?'
readonly MODULE='github.com/umesh0492/go-app-kit'

fail() {
  echo "ERROR: $*" >&2
  exit 1
}

first_release_version() {
  grep -E "^## \[${SEMVER_PATTERN}\]" CHANGELOG.md | head -n1 |
    sed -E "s/^## \[(${SEMVER_PATTERN})\].*/\1/"
}

readme_current_version() {
  sed -n -E "s/^> Current main-line published release: \`v(${SEMVER_PATTERN})\`\..*/\1/p" README.md | head -n1
}

readme_install_version() {
  sed -n -E "s|.*${MODULE}@v(${SEMVER_PATTERN}).*|\1|p" README.md | head -n1
}

resolve_tag() {
  if [[ -n "${GIT_TAG:-}" ]]; then
    printf '%s\n' "${GIT_TAG}"
  elif [[ "${GITHUB_REF_TYPE:-}" == 'tag' && -n "${GITHUB_REF_NAME:-}" ]]; then
    printf '%s\n' "${GITHUB_REF_NAME}"
  else
    git describe --tags --exact-match 2>/dev/null || true
  fi
}

expected_version="$(first_release_version)"
[[ -n "${expected_version}" ]] || fail 'could not determine the latest released version from CHANGELOG.md'
resolved_tag="$(resolve_tag)"
if [[ -n "${resolved_tag}" ]]; then
  [[ "${resolved_tag}" =~ ^v${SEMVER_PATTERN}$ ]] || fail "release tag must be valid SemVer: ${resolved_tag}"
  expected_version="${resolved_tag#v}"
fi

changelog_version="$(first_release_version)"
readme_version="$(readme_current_version)"
install_version="$(readme_install_version)"
golibs_version="$(awk '$1 == "github.com/umesh0492/go-libs" { print $2; exit }' go.mod)"
fintech_version="$(awk '$1 == "github.com/umesh0492/go-fintech-india" { print $2; exit }' go.mod)"

echo '========================================================'
echo 'Verifying version, documentation, and measured truth'
printf '  - Expected version:      v%s\n' "${expected_version}"
printf '  - CHANGELOG release:     v%s\n' "${changelog_version:-N/A}"
printf '  - README published:      v%s\n' "${readme_version:-N/A}"
printf '  - README installation:   v%s\n' "${install_version:-N/A}"
printf '  - Release tag:           %s\n' "${resolved_tag:-untagged}"
printf '  - go-libs dependency:    %s\n' "${golibs_version:-N/A}"
printf '  - go-fintech dependency: %s\n' "${fintech_version:-N/A}"
echo '========================================================'

[[ "${changelog_version}" == "${expected_version}" ]] || fail "CHANGELOG release v${changelog_version} does not match expected v${expected_version}"
[[ "${readme_version}" == "${expected_version}" ]] || fail "README published version v${readme_version} does not match v${expected_version}"
[[ "${install_version}" == "${expected_version}" ]] || fail "README installation version v${install_version} does not match v${expected_version}"
if [[ -n "${resolved_tag}" ]]; then
  [[ "${resolved_tag#v}" == "${changelog_version}" ]] || fail "tag ${resolved_tag} does not match CHANGELOG v${changelog_version}"
fi

if grep -qE '^[[:space:]]*replace[[:space:]]+' go.mod; then
  fail 'go.mod contains a replace directive; use a caller-owned go.work for local companion development'
fi
[[ "${golibs_version}" == 'v0.2.1' ]] || fail "go.mod must require go-libs v0.2.1, got ${golibs_version:-none}"
[[ "${fintech_version}" == 'v0.2.3' ]] || fail "go.mod must require go-fintech-india v0.2.3, got ${fintech_version:-none}"

# Bash 3.2 has indexed arrays but not mapfile/readarray. Populate arrays with
# newline-delimited paths; repository file names must not contain newlines.
go_files=()
while IFS= read -r go_file; do
  go_files[${#go_files[@]}]="${go_file}"
done < <(git ls-files '*.go')
if (( ${#go_files[@]} > 0 )); then
  unformatted=()
  while IFS= read -r go_file; do
    unformatted[${#unformatted[@]}]="${go_file}"
  done < <(gofmt -l "${go_files[@]}")
  (( ${#unformatted[@]} == 0 )) || fail "unformatted tracked Go files: ${unformatted[*]}"
fi

docs=()
while IFS= read -r doc; do
  docs[${#docs[@]}]="${doc}"
done < <(find . -path './.git' -prune -o -name '*.md' -type f -print | sort)
for doc in "${docs[@]}"; do
  if grep -nE 'Go[[:space:]]+1\.(1[0-9]|2[0-5])\b|go1\.(1[0-9]|2[0-5])\b' "${doc}"; then
    fail "${doc#./} claims an unsupported Go baseline"
  fi
done

# Only current-package installation examples must use the documented current
# release. Historical changelog/baseline evidence is intentionally immutable.
wrong_module_versions="$(grep -rnE "${MODULE}@v${SEMVER_PATTERN}" README.md CONTRIBUTING.md SECURITY.md docs audit export examples india notifications outbox pdf 2>/dev/null | grep -v "${MODULE}@v${expected_version}" || true)"
[[ -z "${wrong_module_versions}" ]] || fail "current documentation contains a mismatched ${MODULE} version:\n${wrong_module_versions}"

declare -a packages=(india pdf notifications outbox audit export)
actual_package_count=0
for package in "${packages[@]}"; do
  [[ -f "${package}/README.md" ]] || fail "missing ${package}/README.md"
  grep -qx "# \`${package}\`" "${package}/README.md" || fail "${package}/README.md title must be # \`${package}\`"
  [[ -n "$(git ls-files "${package}/*.go")" ]] || fail "${package} is documented but has no tracked Go files"
  ((actual_package_count += 1))
done
readme_package_headings="$(grep -Ec '^### [0-9]+\. `[^`]+`' README.md || true)"
[[ "${readme_package_headings}" == "${actual_package_count}" ]] || fail "README lists ${readme_package_headings} package headings; expected ${actual_package_count}"
for index in "${!packages[@]}"; do
  grep -q "^### $((index + 1))\. \`${packages[index]}\`" README.md || fail "README package heading for ${packages[index]} is missing or out of order"
done
wrong_package_claims="$(grep -nE '\b[0-9]+[[:space:]]+(core[[:space:]]+)?packages\b' README.md | grep -vE "\b${actual_package_count}[[:space:]]+(core[[:space:]]+)?packages\b" || true)"
[[ -z "${wrong_package_claims}" ]] || fail "README package-count claim is incorrect:\n${wrong_package_claims}"

if grep -RIn '/Users/' --include='*.md' . --exclude-dir=.git; then
  fail 'documentation contains a machine-specific /Users path'
fi
for path in audit/ddl/001_audit_logs.sql outbox/ddl/001_outbox_events.sql outbox/ddl/002_outbox_concurrency_index.sql; do
  [[ -f "${path}" ]] || fail "referenced DDL is missing: ${path}"
done

"${SCRIPT_DIR}/verify_changelog_symbols.sh"

# `check_coverage.sh` both enforces floors and compares every documented global
# and per-package value against the single measured test run.
"${SCRIPT_DIR}/check_coverage.sh"

echo 'Version, documentation, package inventory, and coverage truth checks passed.'
