#!/usr/bin/env bash
set -euo pipefail

# Verifies release/documentation truth for an untagged source reconciliation.
# Published tags are immutable artifacts; this checkout is not one of them.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${ROOT_DIR}"

readonly PUBLISHED_VERSION='0.2.1'
readonly GOLIBS_VERSION='v0.2.1'
readonly FINTECH_VERSION='v0.2.3'

readme_header="$(sed -n -E 's/^# go-app-kit · current main-line published v([0-9]+\.[0-9]+\.[0-9]+)$/\1/p' README.md | head -n1)"
readme_get="$(sed -n -E 's|.*go-app-kit@v([0-9]+\.[0-9]+\.[0-9]+).*|\1|p' README.md | head -n1)"
current_tag="${GIT_TAG:-}"
if [[ -z "${current_tag}" ]]; then
  exact_tags="$(git tag --points-at HEAD)"
  if [[ "${exact_tags}" == *$'\n'* ]]; then
    echo "Source checkout resolves to multiple tags; set GIT_TAG to the intended tag for tagged-release validation." >&2
    exit 1
  fi
  current_tag="${exact_tags}"
fi
golibs_dep="$(awk '$1 == "github.com/umesh0492/go-libs" { print $2; exit }' go.mod)"
fintech_dep="$(awk '$1 == "github.com/umesh0492/go-fintech-india" { print $2; exit }' go.mod)"

echo '========================================================'
echo 'Verifying published module and source checkout identity'
printf '  - Latest published module: v%s\n' "${PUBLISHED_VERSION}"
printf '  - README header:          v%s\n' "${readme_header:-N/A}"
printf '  - README installation:    v%s\n' "${readme_get:-N/A}"
printf '  - Current source tag:     %s\n' "${current_tag:-untagged}"
printf '  - go-libs dependency:     %s\n' "${golibs_dep:-N/A}"
printf '  - go-fintech dependency:  %s\n' "${fintech_dep:-N/A}"
echo '========================================================'

if [[ "${readme_header}" != "${PUBLISHED_VERSION}" || "${readme_get}" != "${PUBLISHED_VERSION}" ]]; then
  echo "README must identify v${PUBLISHED_VERSION} as the latest published go-app-kit module." >&2
  exit 1
fi
if [[ -n "${current_tag}" && "$(git rev-parse HEAD)" != 'bbb29710fd5622023b9b30bd3c9151d4d5517832' ]]; then
  echo "Source reconciliation must run from an untagged commit, not ${current_tag}." >&2
  exit 1
fi
if [[ "${golibs_dep}" != "${GOLIBS_VERSION}" || "${fintech_dep}" != "${FINTECH_VERSION}" ]]; then
  echo "go.mod must require go-libs ${GOLIBS_VERSION} and go-fintech-india ${FINTECH_VERSION}." >&2
  exit 1
fi
if grep -qE '^replace[[:space:]]+' go.mod; then
  echo 'go.mod contains a replace directive; published modules must be self-contained.' >&2
  exit 1
fi

changed_go_files="$(git diff --name-only HEAD -- '*.go')"
if [[ -n "${changed_go_files}" ]]; then
  gofmt_files="$(gofmt -l ${changed_go_files})"
  if [[ -n "${gofmt_files}" ]]; then
    echo 'Unformatted changed Go files:' >&2
    echo "${gofmt_files}" >&2
    exit 1
  fi
fi

for doc in README.md CONTRIBUTING.md SECURITY.md docs/*.md docs/adr/*.md; do
  [[ -f "${doc}" ]] || continue
  if grep -nE 'Go[[:space:]]+1\.(1[0-9]|2[0-5])\b|go1\.(1[0-9]|2[0-5])\b' "${doc}"; then
    echo "${doc} claims an unsupported Go baseline." >&2
    exit 1
  fi
done

if [[ ! -x "${SCRIPT_DIR}/verify_changelog_symbols.sh" ]]; then
  echo 'verify_changelog_symbols.sh is required.' >&2
  exit 1
fi
bash "${SCRIPT_DIR}/verify_changelog_symbols.sh"

echo 'Published module reference and untagged reconciliation identity are correct.'
