#!/usr/bin/env bash
set -euo pipefail

# Verifies a tagged checkout as an immutable, published Go module release. This
# script only observes tags/releases; it never creates, moves, or deletes them.
# It always writes a release manifest, including diagnostic state on failure.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${ROOT_DIR}"

TAG="${1:-${GIT_TAG:-}}"
MANIFEST_PATH="${RELEASE_MANIFEST_PATH:-release-manifest.json}"
GO_CMD="${GO_CMD:-go}"
GIT_REMOTE="${GIT_REMOTE:-origin}"
RETRY_ATTEMPTS="${RELEASE_PROXY_RETRY_ATTEMPTS:-4}"
RETRY_INITIAL_DELAY_SECONDS="${RELEASE_PROXY_RETRY_INITIAL_DELAY_SECONDS:-2}"
RETRY_MAX_DELAY_SECONDS="${RELEASE_PROXY_RETRY_MAX_DELAY_SECONDS:-30}"
SLEEP_CMD="${RELEASE_PROXY_SLEEP_COMMAND:-sleep}"

VERIFICATION_STATUS='failed'
FAILURE_STAGE='initialization'
DOWNLOAD_ERROR=''
ATTEMPT_MODULE_CACHE_DIR=''

sha256_file() {
  local file="$1"
  if command -v sha256sum >/dev/null; then
    sha256sum "${file}" | awk '{print $1}'
  elif command -v shasum >/dev/null; then
    shasum -a 256 "${file}" | awk '{print $1}'
  elif command -v openssl >/dev/null; then
    openssl dgst -sha256 "${file}" | awk '{print $NF}'
  else
    echo 'sha256sum, shasum, or openssl is required' >&2
    return 1
  fi
}

sha256_stdin() {
  local file
  file="$(mktemp)"
  cat > "${file}"
  sha256_file "${file}"
  rm -f "${file}"
}

write_manifest() {
  cat > "${MANIFEST_PATH}" <<EOF
{
  "schema_version": 1,
  "verification_status": "${VERIFICATION_STATUS}",
  "failure_stage": "${FAILURE_STAGE}",
  "module": "${MODULE_PATH:-}",
  "version": "${TAG}",
  "tag_object": "${LOCAL_TAG_OBJECT:-}",
  "commit": "${LOCAL_COMMIT:-}",
  "remote": "${GIT_REMOTE}",
  "remote_tag_object": "${REMOTE_TAG_OBJECT:-}",
  "remote_commit": "${REMOTE_COMMIT:-}",
  "module_sum": "${MODULE_SUM:-}",
  "go_mod_sum": "${MODULE_GO_MOD_SUM:-}",
  "source_archive_sha256": "${ARCHIVE_SHA256:-}",
  "go_mod_sha256": "${GO_MOD_SHA256:-}"
}
EOF
}

remove_attempt_module_cache() {
  if [[ -n "${ATTEMPT_MODULE_CACHE_DIR}" ]]; then
    chmod -R u+w "${ATTEMPT_MODULE_CACHE_DIR}" 2>/dev/null || true
    rm -rf "${ATTEMPT_MODULE_CACHE_DIR}" || true
    ATTEMPT_MODULE_CACHE_DIR=''
  fi
}

cleanup() {
  [[ -z "${DOWNLOAD_ERROR}" ]] || rm -f "${DOWNLOAD_ERROR}"
  remove_attempt_module_cache
}

on_exit() {
  local status=$?
  trap - EXIT
  cleanup
  if (( status != 0 )); then
    VERIFICATION_STATUS='failed'
    write_manifest || echo "warning: unable to write failed manifest ${MANIFEST_PATH}" >&2
  fi
  exit "${status}"
}
trap on_exit EXIT

[[ "${TAG}" =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$ ]] || {
  echo "usage: $0 vMAJOR.MINOR.PATCH[-PRERELEASE]" >&2
  exit 2
}
[[ "${RETRY_ATTEMPTS}" =~ ^[1-9][0-9]*$ ]] &&
  [[ "${RETRY_INITIAL_DELAY_SECONDS}" =~ ^[0-9]+$ ]] &&
  [[ "${RETRY_MAX_DELAY_SECONDS}" =~ ^[0-9]+$ ]] || {
  echo 'invalid proxy retry settings' >&2
  exit 2
}

git remote get-url "${GIT_REMOTE}" >/dev/null
export GIT_TAG="${TAG}"
FAILURE_STAGE='version'
"${SCRIPT_DIR}/check_version.sh"

FAILURE_STAGE='local_tag'
LOCAL_TAG_OBJECT="$(git rev-parse "refs/tags/${TAG}")"
LOCAL_COMMIT="$(git rev-parse "${TAG}^{}")"
[[ "${LOCAL_COMMIT}" == "$(git rev-parse HEAD)" ]] || {
  echo "release tag ${TAG} does not resolve to HEAD" >&2
  exit 1
}

FAILURE_STAGE='remote_tag'
REMOTE_TAG_OBJECT="$(git ls-remote --tags --refs "${GIT_REMOTE}" "refs/tags/${TAG}" | awk 'NR == 1 { print $1 }')"
REMOTE_COMMIT="$(git ls-remote "${GIT_REMOTE}" "refs/tags/${TAG}^{}" | awk 'NR == 1 { print $1 }')"
REMOTE_COMMIT="${REMOTE_COMMIT:-${REMOTE_TAG_OBJECT}}"
[[ -n "${REMOTE_TAG_OBJECT}" && "${REMOTE_TAG_OBJECT}" == "${LOCAL_TAG_OBJECT}" && "${REMOTE_COMMIT}" == "${LOCAL_COMMIT}" ]] || {
  echo "remote ${GIT_REMOTE} tag ${TAG} does not match the checked-out immutable release" >&2
  exit 1
}

FAILURE_STAGE='module_identity'
MODULE_PATH="$("${GO_CMD}" list -m -f '{{.Path}}')"

FAILURE_STAGE='module_proxy'
DOWNLOAD_ERROR="$(mktemp)"
delay_seconds="${RETRY_INITIAL_DELAY_SECONDS}"
for (( attempt = 1; attempt <= RETRY_ATTEMPTS; attempt++ )); do
  ATTEMPT_MODULE_CACHE_DIR="$(mktemp -d)"
  if DOWNLOAD_JSON="$(GOWORK=off GOMODCACHE="${ATTEMPT_MODULE_CACHE_DIR}" GOPROXY=https://proxy.golang.org "${GO_CMD}" mod download -json "${MODULE_PATH}@${TAG}" 2>"${DOWNLOAD_ERROR}")"; then
    remove_attempt_module_cache
    break
  fi
  remove_attempt_module_cache
  echo "proxy resolution attempt ${attempt}/${RETRY_ATTEMPTS} for ${MODULE_PATH}@${TAG} failed (fresh module cache):" >&2
  cat "${DOWNLOAD_ERROR}" >&2
  if (( attempt == RETRY_ATTEMPTS )); then
    echo "proxy resolution failed after ${RETRY_ATTEMPTS} attempts; final diagnostics shown above" >&2
    exit 1
  fi
  echo "retrying proxy resolution in ${delay_seconds}s" >&2
  "${SLEEP_CMD}" "${delay_seconds}"
  if (( delay_seconds < RETRY_MAX_DELAY_SECONDS )); then
    delay_seconds=$((delay_seconds * 2))
    (( delay_seconds <= RETRY_MAX_DELAY_SECONDS )) || delay_seconds="${RETRY_MAX_DELAY_SECONDS}"
  fi
done
rm -f "${DOWNLOAD_ERROR}"
DOWNLOAD_ERROR=''

MODULE_VERSION="$(sed -n 's/^[[:space:]]*"Version": "\([^"]*\)",/\1/p' <<<"${DOWNLOAD_JSON}" | head -n1)"
MODULE_SUM="$(sed -n 's/^[[:space:]]*"Sum": "\([^"]*\)",/\1/p' <<<"${DOWNLOAD_JSON}" | head -n1)"
MODULE_GO_MOD_SUM="$(sed -n 's/^[[:space:]]*"GoModSum": "\([^"]*\)".*/\1/p' <<<"${DOWNLOAD_JSON}" | head -n1)"
[[ "${MODULE_VERSION}" == "${TAG}" && -n "${MODULE_SUM}" && -n "${MODULE_GO_MOD_SUM}" ]] || {
  echo "module proxy did not resolve ${MODULE_PATH}@${TAG} with integrity sums" >&2
  exit 1
}

FAILURE_STAGE='source_hash'
ARCHIVE_SHA256="$(git archive --format=tar "${TAG}" | sha256_stdin)"
GO_MOD_SHA256="$(git show "${TAG}:go.mod" | sha256_stdin)"

VERIFICATION_STATUS='verified'
FAILURE_STAGE=''
write_manifest
echo "release manifest verified and written to ${MANIFEST_PATH}"
