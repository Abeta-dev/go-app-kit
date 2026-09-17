#!/usr/bin/env bash
set -euo pipefail

# Verify a candidate release after its immutable tag is available from the Go
# module proxy. This script does not create, delete, or move tags or releases.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${ROOT_DIR}"

TAG="${1:-${GIT_TAG:-}}"
MANIFEST_PATH="${RELEASE_MANIFEST_PATH:-release-manifest.json}"

if [[ ! "${TAG}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "usage: $0 vMAJOR.MINOR.PATCH" >&2
  exit 2
fi

module_path="$(sed -n -E 's/^module[[:space:]]+([^[:space:]]+).*/\1/p' go.mod | head -n1)"
if [[ -z "${module_path}" ]]; then
  echo 'unable to determine module path from go.mod' >&2
  exit 1
fi

local_tag_object="$(git rev-parse "refs/tags/${TAG}")"
local_commit="$(git rev-parse "${TAG}^{}")"
head_commit="$(git rev-parse HEAD)"
if [[ "${local_commit}" != "${head_commit}" ]]; then
  echo "release tag ${TAG} resolves to ${local_commit}, not HEAD ${head_commit}" >&2
  exit 1
fi

remote_tag_object="$(git ls-remote --tags --refs origin "refs/tags/${TAG}" | awk 'NR == 1 { print $1 }')"
remote_commit="$(git ls-remote origin "refs/tags/${TAG}^{}" | awk 'NR == 1 { print $1 }')"
remote_commit="${remote_commit:-${remote_tag_object}}"
if [[ -z "${remote_tag_object}" || "${remote_tag_object}" != "${local_tag_object}" || "${remote_commit}" != "${local_commit}" ]]; then
  echo "remote tag ${TAG} does not match the checked-out immutable release" >&2
  exit 1
fi

download_json="$(GOWORK=off go mod download -json "${module_path}@${TAG}")"
module_version="$(printf '%s\n' "${download_json}" | sed -n 's/^[[:space:]]*"Version": "\([^"]*\)",/\1/p' | head -n1)"
module_sum="$(printf '%s\n' "${download_json}" | sed -n 's/^[[:space:]]*"Sum": "\([^"]*\)",/\1/p' | head -n1)"
gomod_sum="$(printf '%s\n' "${download_json}" | sed -n 's/^[[:space:]]*"GoModSum": "\([^"]*\)".*/\1/p' | head -n1)"
if [[ "${module_version}" != "${TAG}" || -z "${module_sum}" || -z "${gomod_sum}" ]]; then
  echo "module proxy did not resolve ${module_path}@${TAG} with integrity sums" >&2
  exit 1
fi

source_archive_sha256="$(git archive --format=tar "${TAG}" | sha256sum | awk '{print $1}')"
gomod_sha256="$(git show "${TAG}:go.mod" | sha256sum | awk '{print $1}')"
cat >"${MANIFEST_PATH}" <<EOF
{
  "schema_version": 1,
  "module": "${module_path}",
  "version": "${TAG}",
  "tag_object": "${local_tag_object}",
  "commit": "${local_commit}",
  "remote_tag_object": "${remote_tag_object}",
  "remote_commit": "${remote_commit}",
  "module_sum": "${module_sum}",
  "go_mod_sum": "${gomod_sum}",
  "source_archive_sha256": "${source_archive_sha256}",
  "go_mod_sha256": "${gomod_sha256}"
}
EOF

echo "Release manifest verified and written to ${MANIFEST_PATH}."
