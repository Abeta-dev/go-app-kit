#!/usr/bin/env bash
set -euo pipefail

# ==============================================================================
# Leaf Package Import Isolation & Minimal Footprint Gate
#
# Guarantees that:
#   1. Gate 1 — Zero Cross-Package Sibling Coupling:
#      None of the 5 core packages (audit, export, notifications, outbox, pdf)
#      import any sibling github.com/abeta-dev/go-app-kit/* package across
#      production, internal test, or external test files (only
#      examples/invoice_service composes multiple go-app-kit packages).
#   2. Gate 2 — Heavy Dependency & Domain Isolation per Package:
#      Verified via both `go list -deps` and isolated temporary `go.mod`
#      consumer sandboxes:
#        - export, pdf, notifications never pull github.com/jackc/pgx,
#          github.com/gin-gonic/gin, or github.com/abeta-dev/go-fintech-india
#          into a consumer's dependency graph or go.sum.
#        - audit, outbox never pull github.com/gin-gonic/gin or
#          github.com/abeta-dev/go-fintech-india into a consumer's dependency
#          graph or go.sum.
#
# Bash 3.2 compatible (verified by ./scripts/test_bash_compat.sh).
# ==============================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${ROOT_DIR}"

readonly MODULE="github.com/abeta-dev/go-app-kit"
CORE_PACKAGES=("audit" "export" "notifications" "outbox" "pdf")

echo "========================================================"
echo "🍃 Verifying Leaf-Package Import & Dependency Isolation"
echo "   Checking ${#CORE_PACKAGES[@]} core packages: ${CORE_PACKAGES[*]}"
echo "========================================================"

FAILED_COUNT=0

# ------------------------------------------------------------------------------
# Gate 1: Zero Cross-Package Sibling Coupling Across All 5 Core Packages
# ------------------------------------------------------------------------------
echo "🔍 Gate 1: Verifying zero cross-package sibling coupling..."
for pkg in "${CORE_PACKAGES[@]}"; do
  ALL_IMPORTS="$(go list -f '{{join .Imports "\n"}}{{"\n"}}{{join .TestImports "\n"}}{{"\n"}}{{join .XTestImports "\n"}}' "./${pkg}")"
  SIBLING_IMPORTS="$(printf '%s\n' "${ALL_IMPORTS}" \
    | grep -E "^${MODULE}/" \
    | grep -vE "^${MODULE}/${pkg}(/|$)" \
    | sort -u || true)"

  if [ -n "${SIBLING_IMPORTS}" ]; then
    echo "❌ Error: '${pkg}' imports sibling go-app-kit package(s): ${SIBLING_IMPORTS}" >&2
    FAILED_COUNT=$((FAILED_COUNT + 1))
  else
    echo "   - ${pkg}: zero sibling go-app-kit imports ✅"
  fi
done

# ------------------------------------------------------------------------------
# Gate 2: Heavy Dependency & Domain Isolation per Package
# ------------------------------------------------------------------------------
echo "🔍 Gate 2: Verifying transitive dependency & isolated go.mod sandbox resolution..."

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "${TMP_DIR}"' EXIT

for pkg in "${CORE_PACKAGES[@]}"; do
  case "${pkg}" in
    export|pdf|notifications)
      FORBIDDEN_PATTERN='github\.com/jackc/pgx|github\.com/gin-gonic/gin|github\.com/abeta-dev/go-fintech-india'
      FORBIDDEN_DESC='pgx, gin, go-fintech-india'
      ;;
    audit|outbox)
      FORBIDDEN_PATTERN='github\.com/gin-gonic/gin|github\.com/abeta-dev/go-fintech-india'
      FORBIDDEN_DESC='gin, go-fintech-india'
      ;;
    *)
      echo "❌ Error: unknown core package '${pkg}'" >&2
      exit 1
      ;;
  esac

  # 2a. Direct repository dependency graph check
  DIRECT_LEAK="$(go list -deps "./${pkg}" | grep -E "${FORBIDDEN_PATTERN}" | sort -u | tr '\n' ' ' || true)"
  if [ -n "${DIRECT_LEAK}" ]; then
    echo "❌ Error: '${pkg}' dependency graph leaked forbidden packages (${FORBIDDEN_DESC}): ${DIRECT_LEAK}" >&2
    FAILED_COUNT=$((FAILED_COUNT + 1))
    continue
  fi

  # 2b. Isolated temporary go.mod consumer sandbox check
  PKG_DIR="${TMP_DIR}/${pkg}"
  mkdir -p "${PKG_DIR}"
  (
    cd "${PKG_DIR}"
    GOWORK=off go mod init "verify_leaf_${pkg}" >/dev/null 2>&1
    cat << EOF > main.go
package main

import (
	_ "${MODULE}/${pkg}"
)

func main() {}
EOF
    GOWORK=off go mod edit -replace "${MODULE}=${ROOT_DIR}" >/dev/null 2>&1
    if ! TIDY_ERR="$(GOWORK=off go mod tidy 2>&1)"; then
      echo "❌ Error: 'go mod tidy' failed for '${pkg}': ${TIDY_ERR}" >&2
      exit 1
    fi

    SANDBOX_DEPS_LEAK="$(GOWORK=off go list -deps . | grep -E "${FORBIDDEN_PATTERN}" | sort -u | tr '\n' ' ' || true)"
    if [ -n "${SANDBOX_DEPS_LEAK}" ]; then
      echo "❌ Error: isolated consumer of '${pkg}' pulled forbidden packages (${FORBIDDEN_DESC}) into dependency graph: ${SANDBOX_DEPS_LEAK}" >&2
      exit 1
    fi

    if [ -f go.sum ] && grep -E "${FORBIDDEN_PATTERN}" go.sum >/dev/null 2>&1; then
      SUM_LEAK="$(grep -E "${FORBIDDEN_PATTERN}" go.sum | awk '{print $1}' | sort -u | tr '\n' ' ')"
      echo "❌ Error: isolated consumer of '${pkg}' leaked forbidden modules (${FORBIDDEN_DESC}) into go.sum: ${SUM_LEAK}" >&2
      exit 1
    fi

    echo "   - ${pkg}: clean isolated consumer import ✅ (0 forbidden deps: ${FORBIDDEN_DESC})"
  ) || FAILED_COUNT=$((FAILED_COUNT + 1))
done

echo "========================================================"
if [ "${FAILED_COUNT}" -ne 0 ]; then
  echo "❌ Error: ${FAILED_COUNT} leaf-import isolation check(s) failed!" >&2
  exit 1
fi

echo "✅ All ${#CORE_PACKAGES[@]} core packages satisfy zero sibling coupling and minimal-footprint dependency isolation!"
echo "========================================================"
