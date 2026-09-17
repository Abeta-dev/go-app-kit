#!/usr/bin/env bash
set -euo pipefail

# Static guard for the Bash 3.2 minimum supported by macOS. These scripts use
# indexed arrays only; reject Bash 4+ conveniences before they reach CI.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${ROOT_DIR}"

for script in scripts/*.sh; do
  bash -n "${script}"
done

# Match executable-looking leading syntax so the test's own diagnostic text and
# explanatory comments do not trigger a false positive.
if grep -nE '^[[:space:]]*(mapfile|readarray|declare[[:space:]]+-A|typeset[[:space:]]+-A)([[:space:]]|$)' scripts/*.sh; then
  echo 'Bash 4+ constructs found; release scripts must remain Bash 3.2 compatible.' >&2
  exit 1
fi
if grep -nE '\*\*/' scripts/*.sh | grep -v '^scripts/test_bash_compat.sh:'; then
  echo 'Bash globstar found; release scripts must remain Bash 3.2 compatible.' >&2
  exit 1
fi

echo 'Bash 3.2 static compatibility check passed.'
