#!/usr/bin/env bash
set -euo pipefail

# ==============================================================================
# Strict Deprecation Window Gate (N + 2 Minor Releases Contract)
#
# Enforces that:
#   1. All non-test *.go files (! -name '*_test.go') are scanned for
#      `// Deprecated:` annotations.
#   2. Every `// Deprecated:` line strictly matches:
#      `// Deprecated: Since vX.Y.Z (remove in >= vA.B.C). Use <Replacement> instead.`
#   3. The removal target `vA.B.C` is at least 2 minor releases after `vX.Y.Z`:
#      (A > X) || (A == X && B >= Y + 2)
#   4. Against the previous release tag in git (if available), no tracked
#      `// Deprecated: Since vX.Y.Z (remove in >= vA.B.C)` annotation is
#      removed before the current version in CHANGELOG.md reaches `>= vA.B.C`.
#
# Bash 3.2 compatible (verified by ./scripts/test_bash_compat.sh).
# ==============================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${ROOT_DIR}"

semver_ge() {
  local v1="${1#v}"
  local v2="${2#v}"
  local maj1 min1 pat1 maj2 min2 pat2
  IFS=. read -r maj1 min1 pat1 <<< "${v1}"
  IFS=. read -r maj2 min2 pat2 <<< "${v2}"
  if (( maj1 > maj2 )); then return 0; fi
  if (( maj1 < maj2 )); then return 1; fi
  if (( min1 > min2 )); then return 0; fi
  if (( min1 < min2 )); then return 1; fi
  if (( pat1 >= pat2 )); then return 0; fi
  return 1
}

CURRENT_VERSION="$(grep -E '^## \[[0-9]+\.[0-9]+\.[0-9]+\]' CHANGELOG.md | head -n1 | sed -E 's/^## \[([0-9]+\.[0-9]+\.[0-9]+)\].*/\1/')"
if [ -z "${CURRENT_VERSION}" ]; then
  echo "❌ Error: could not parse current version from CHANGELOG.md" >&2
  exit 1
fi

echo "========================================================"
echo "⏳ Verifying Strict N+2 Minor-Release Deprecation Contract"
echo "   Current CHANGELOG version: v${CURRENT_VERSION}"
echo "========================================================"

FAILED_COUNT=0
CHECKED_ANNOTATIONS=0

NON_TEST_GO_FILES=()
while IFS= read -r go_file; do
  NON_TEST_GO_FILES[${#NON_TEST_GO_FILES[@]}]="${go_file}"
done < <(find . -path './.git' -prune -o -name '*.go' ! -name '*_test.go' -type f -print | sort)

ANNOTATION_REGEX='^//[[:space:]]*Deprecated:[[:space:]]+Since[[:space:]]+v([0-9]+)\.([0-9]+)\.([0-9]+)[[:space:]]+\(remove[[:space:]]+in[[:space:]]+>=[[:space:]]+v([0-9]+)\.([0-9]+)\.([0-9]+)\)\.[[:space:]]+Use[[:space:]]+(.+)[[:space:]]+instead\.$'

for go_file in "${NON_TEST_GO_FILES[@]}"; do
  while IFS=: read -r lineno line_content; do
    [ -z "${lineno}" ] && continue
    CHECKED_ANNOTATIONS=$((CHECKED_ANNOTATIONS + 1))
    trimmed="$(printf '%s' "${line_content}" | sed -E 's/^[[:space:]]+//; s/[[:space:]]+$//')"

    if [[ ! "${trimmed}" =~ ${ANNOTATION_REGEX} ]]; then
      echo "❌ Error: ${go_file#./}:${lineno}: invalid '// Deprecated:' format:" >&2
      echo "   Found:    ${trimmed}" >&2
      echo "   Required: // Deprecated: Since vX.Y.Z (remove in >= vA.B.C). Use <Replacement> instead." >&2
      FAILED_COUNT=$((FAILED_COUNT + 1))
      continue
    fi

    since_maj="${BASH_REMATCH[1]}"
    since_min="${BASH_REMATCH[2]}"
    since_pat="${BASH_REMATCH[3]}"
    rem_maj="${BASH_REMATCH[4]}"
    rem_min="${BASH_REMATCH[5]}"
    rem_pat="${BASH_REMATCH[6]}"
    replacement="${BASH_REMATCH[7]}"

    if ! (( rem_maj > since_maj || (rem_maj == since_maj && rem_min >= since_min + 2) )); then
      echo "❌ Error: ${go_file#./}:${lineno}: deprecation window violates N+2 minor-release minimum:" >&2
      echo "   Since v${since_maj}.${since_min}.${since_pat} -> remove in >= v${rem_maj}.${rem_min}.${rem_pat}" >&2
      FAILED_COUNT=$((FAILED_COUNT + 1))
      continue
    fi

    echo "   - ${go_file#./}:${lineno}: Since v${since_maj}.${since_min}.${since_pat} (remove in >= v${rem_maj}.${rem_min}.${rem_pat}) -> Use ${replacement} ✅"
  done < <(grep -nE '^[[:space:]]*//[[:space:]]*Deprecated:' "${go_file}" || true)
done

if [ "${CHECKED_ANNOTATIONS}" -eq 0 ]; then
  echo "   - No active '// Deprecated:' annotations in non-test Go files ✅"
fi

# ------------------------------------------------------------------------------
# Step 4: Verify no deprecation annotation from previous release tag was removed
#         prior to its declared `remove in >= vA.B.C` target version.
# ------------------------------------------------------------------------------
PREV_TAG=""
LATEST_TAG="$(git tag --list 'v[0-9]*.[0-9]*.[0-9]*' --sort=-v:refname 2>/dev/null | head -n1 || true)"
if [ -n "${LATEST_TAG}" ]; then
  LATEST_COMMIT="$(git rev-parse "${LATEST_TAG}^{}" 2>/dev/null || true)"
  HEAD_COMMIT="$(git rev-parse HEAD 2>/dev/null || true)"
  if [ "${LATEST_COMMIT}" != "${HEAD_COMMIT}" ]; then
    PREV_TAG="${LATEST_TAG}"
  else
    PREV_TAG="$(git tag --list 'v[0-9]*.[0-9]*.[0-9]*' --sort=-v:refname 2>/dev/null | sed -n '2p' || true)"
  fi
fi

if [ -n "${PREV_TAG}" ]; then
  echo "🔍 Checking deprecation continuity against previous release tag (${PREV_TAG})..."
  while IFS= read -r prev_entry; do
    [ -z "${prev_entry}" ] && continue
    # Format: <tag>:<path>:<lineno>:<content>
    prev_rest="${prev_entry#"${PREV_TAG}":}"
    prev_path="${prev_rest%%:*}"
    prev_after_path="${prev_rest#*:}"
    prev_content="${prev_after_path#*:}"
    prev_trimmed="$(printf '%s' "${prev_content}" | sed -E 's/^[[:space:]]+//; s/[[:space:]]+$//')"

    if [[ "${prev_trimmed}" =~ ${ANNOTATION_REGEX} ]]; then
      since_ver="${BASH_REMATCH[1]}.${BASH_REMATCH[2]}.${BASH_REMATCH[3]}"
      remove_ver="${BASH_REMATCH[4]}.${BASH_REMATCH[5]}.${BASH_REMATCH[6]}"

      # Check if this annotation still exists in the working tree
      if [ ! -f "${prev_path}" ] || ! grep -Fq "${prev_trimmed}" "${prev_path}"; then
        if ! semver_ge "${CURRENT_VERSION}" "${remove_ver}"; then
          echo "❌ Error: premature removal of deprecated symbol in ${prev_path} (from ${PREV_TAG}):" >&2
          echo "   Annotation: ${prev_trimmed}" >&2
          echo "   Current CHANGELOG version v${CURRENT_VERSION} is earlier than allowed removal v${remove_ver}" >&2
          FAILED_COUNT=$((FAILED_COUNT + 1))
        else
          echo "   - Completed deprecation window for ${prev_path} (Since v${since_ver}, removed at v${CURRENT_VERSION} >= v${remove_ver}) ✅"
        fi
      fi
    fi
  done < <(git grep -n '// Deprecated: Since v' "${PREV_TAG}" -- '*.go' ':!*_test.go' 2>/dev/null || true)
fi

echo "========================================================"
if [ "${FAILED_COUNT}" -ne 0 ]; then
  echo "❌ Error: ${FAILED_COUNT} deprecation policy violation(s) detected!" >&2
  exit 1
fi

echo "✅ Strict N+2 Minor-Release Deprecation Window Gate passed (${CHECKED_ANNOTATIONS} active annotation(s) verified)!"
echo "========================================================"
