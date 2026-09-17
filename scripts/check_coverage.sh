#!/usr/bin/env bash
set -euo pipefail

# Enforces coverage floors and ensures README's coverage table matches one
# measured test run. It never reuses stale local coverage output.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${ROOT_DIR}"

readonly GLOBAL_FLOOR='88.0'
readonly PACKAGE_FLOOR='75.0'
readonly MODULE='github.com/umesh0492/go-app-kit'
coverage_file="${1:-coverage.out}"
summary_file="${COVERAGE_SUMMARY_FILE:-/tmp/gak_coverage_packages.txt}"

cleanup() {
  rm -f "${summary_file}"
}
trap cleanup EXIT

fail() {
  echo "ERROR: $*" >&2
  exit 1
}

coverage_value() {
  printf '%s' "$1" | tr -d '%'
}

is_below() {
  awk -v left="$1" -v right="$2" 'BEGIN { exit !(left + 0 < right + 0) }'
}

readme_global_coverage() {
  sed -n -E 's/.*Overall Repository Statement Coverage: ([0-9]+\.[0-9]+)%.*/\1/p' README.md | head -n1
}

readme_total_coverage() {
  grep -E '^\| \*\*Total Statement Coverage\*\* \|' README.md |
    sed -n -E 's/.*\*\*([0-9]+\.[0-9]+)%\*\*.*/\1/p' | head -n1
}

readme_package_coverage() {
  local package="$1"
  awk -F'|' -v package="\`$package\`" '
    $2 ~ package {
      value = $4
      gsub(/[^0-9.]/, "", value)
      print value
      exit
    }
  ' README.md
}

echo '========================================================'
echo 'Running statement coverage and documentation truth gate'
printf '  - Global floor:      >= %s%%\n' "${GLOBAL_FLOOR}"
printf '  - Per-package floor: >= %s%%\n' "${PACKAGE_FLOOR}"
echo '========================================================'

echo 'Generating coverage profile in one test suite run...'
go test -coverprofile="${coverage_file}" ./... | tee "${summary_file}"
global_coverage_str="$(go tool cover -func="${coverage_file}" | awk '/^total:/ { print $3 }')"
global_coverage="$(coverage_value "${global_coverage_str}")"
[[ -n "${global_coverage}" ]] || fail 'could not determine global coverage'

if is_below "${global_coverage}" "${GLOBAL_FLOOR}"; then
  fail "global coverage ${global_coverage}% is below ${GLOBAL_FLOOR}%"
fi

mapfile -t packages < <(go list ./... | sed "s|^${MODULE}/||" | grep -v "^${MODULE}$" | sort)
(( ${#packages[@]} > 0 )) || fail 'could not determine Go packages'

printf '\nMeasured package coverage:\n'
printf '| Package | Statement Coverage |\n| :--- | :--- |\n'
for package in "${packages[@]}"; do
  package_line="$(grep -E "${MODULE}/${package}[[:space:]].*coverage:" "${summary_file}" | head -n1 || true)"
  [[ -n "${package_line}" ]] || fail "coverage output has no entry for ${package}"
  package_coverage="$(sed -n -E 's/.*coverage: ([0-9]+\.[0-9]+)%.*/\1/p' <<<"${package_line}")"
  [[ -n "${package_coverage}" ]] || fail "could not parse coverage for ${package}"
  printf '| `%s` | **%s%%** |\n' "${package}" "${package_coverage}"
  if is_below "${package_coverage}" "${PACKAGE_FLOOR}"; then
    fail "package ${package} coverage ${package_coverage}% is below ${PACKAGE_FLOOR}%"
  fi

  documented_coverage="$(readme_package_coverage "${package}")"
  [[ -n "${documented_coverage}" ]] || fail "README coverage table has no ${package} row"
  if [[ "${documented_coverage}" != "${package_coverage}" ]]; then
    fail "README coverage for ${package} (${documented_coverage}%) differs from measured ${package_coverage}%"
  fi
done

readme_global="$(readme_global_coverage)"
readme_total="$(readme_total_coverage)"
[[ -n "${readme_global}" ]] || fail 'README global coverage callout is missing'
[[ -n "${readme_total}" ]] || fail 'README total coverage table row is missing'
[[ "${readme_global}" == "${global_coverage}" ]] || fail "README global coverage ${readme_global}% differs from measured ${global_coverage}%"
[[ "${readme_total}" == "${global_coverage}" ]] || fail "README total coverage ${readme_total}% differs from measured ${global_coverage}%"

mkdir -p .github/badges
badge_color='brightgreen'
if is_below "${global_coverage}" '90.0'; then badge_color='yellow'; fi
if is_below "${global_coverage}" '80.0'; then badge_color='red'; fi
cat > .github/badges/coverage.json <<EOF
{
  "schemaVersion": 1,
  "label": "coverage",
  "message": "${global_coverage_str}",
  "color": "${badge_color}"
}
EOF

if [[ -n "${GITHUB_STEP_SUMMARY:-}" ]]; then
  {
    echo '### Verified CI/CD Statement Coverage Gate'
    echo
    printf -- '- **Overall Repository Statement Coverage**: `%s` (gate: `>= %s%%`)\n' "${global_coverage_str}" "${GLOBAL_FLOOR}"
  } >> "${GITHUB_STEP_SUMMARY}"
fi

printf '\nCoverage truth check passed: %s global coverage.\n' "${global_coverage_str}"
