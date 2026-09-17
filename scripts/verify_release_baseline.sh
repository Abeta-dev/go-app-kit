#!/usr/bin/env bash
set -euo pipefail

# Verify immutable published baseline evidence. This intentionally performs live
# network checks and is for local release investigation or scheduled/release
# workflows, not the pull-request fast path.

readonly MODULE='github.com/umesh0492/go-app-kit'
readonly PUBLISHED_VERSION='v0.2.1'
readonly AMBIGUOUS_VERSION='v0.3.0'
readonly EXPECTED_PUBLISHED_TAG_OBJECT='277ad0741cd723d8c2c445b24dec5608cf1a1c3b'
readonly EXPECTED_PUBLISHED_COMMIT='bbb29710fd5622023b9b30bd3c9151d4d5517832'
readonly EXPECTED_PUBLISHED_ZIP_SHA256='11c92c0eabf36fefe0b0a38eceebe8d3f311cda8b72e27edf479a3a64192d325'
readonly EXPECTED_PUBLISHED_MOD_SHA256='19bbf44cd0d6733695d4af577c1dbdf11edf1098e102e1333a4d284c3e96d29e'
readonly EXPECTED_PUBLISHED_SUM='h1:fIcEZxtHfIsYSfaoZPN5+xMGRh6WGltgBMNmkix+mx0='
readonly EXPECTED_PUBLISHED_GOMOD_SUM='h1:dOpJngefgYfoD4rogKC1X+O2Z1fYtQ5rT/hdfXMVfDI='
readonly EXPECTED_AMBIGUOUS_COMMIT='a4501b6ee165c475811e6e3c43dab9b20907d25e'
readonly EXPECTED_AMBIGUOUS_ZIP_SHA256='52036d15c805f2f10f795e868abbe32248f71e210c6f4213524f2a464c1806f4'
readonly EXPECTED_AMBIGUOUS_MOD_SHA256='17dae7887e7a58cbb4e3bbccc2a7c2aced25531d83835375f73dd92c7e9a6b5e'
readonly EXPECTED_AMBIGUOUS_SUM='h1:vxh+SnwkUot1nLkWKDUBuhLfEPI877ABFxjOfYYdTiY='
readonly EXPECTED_AMBIGUOUS_GOMOD_SUM='h1:iV+llhPWLrdj2q9LoKa8cg6gZ/5MelPvOkAfLDye+DU='
readonly GOLIBS_VERSION='v0.2.1'
readonly EXPECTED_GOLIBS_COMMIT='b75acc6d82e47189ef174a4ea80134fed8cd392f'
readonly EXPECTED_GOLIBS_ZIP_SHA256='d519b6624138503f1cbe8f3de071f0fe685b521ad5f1f9634a0a85a1992fc9db'
readonly EXPECTED_GOLIBS_MOD_SHA256='3625f2188bcc9a45a7bbf32241641dd317890d5a44c0bd32748cf12d6eb4ac47'
readonly EXPECTED_GOLIBS_SUM='h1:9Fzm2GZMkd+rnFAFOd5MURnOorqM98O4H/zcVUb6uoY='
readonly EXPECTED_GOLIBS_GOMOD_SUM='h1:R7gQaadUNwpnavd5P96ThNbhYyUUKyVbfKCR/mu29/o='
readonly FINTECH_VERSION='v0.2.3'
readonly EXPECTED_FINTECH_COMMIT='55888b8fb529bb6379eb96f9a04ce649b7a08b90'
readonly EXPECTED_FINTECH_ZIP_SHA256='17b5e66a4f1164cabca95e4bfef849b956162552f32db7e2e7ea6d57b30f8504'
readonly EXPECTED_FINTECH_MOD_SHA256='8352baf728c076a30ac48ea72091b8a313e1feb528302039de8854546c5150ff'
readonly EXPECTED_FINTECH_SUM='h1:f2QB8HdhsxaCRFvu0HWQmu55rqPtxA6/kliAYIYNqtU='
readonly EXPECTED_FINTECH_GOMOD_SUM='h1:UPKoeW7JKYkR0nQtY4rW1dmJ9oI20RzDOeE4s7fDrgc='

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"
GIT_REMOTE="${GIT_REMOTE:-origin}"

assert_equals() {
  local label="$1" want="$2" got="$3"
  [[ "${got}" == "${want}" ]] || {
    printf 'ERROR: %s mismatch\n  expected: %s\n  got:      %s\n' "${label}" "${want}" "${got}" >&2
    exit 1
  }
  printf 'OK: %s: %s\n' "${label}" "${got}"
}

sha256_file() {
  local file="$1"
  if command -v sha256sum >/dev/null; then sha256sum "${file}" | awk '{print $1}'
  elif command -v shasum >/dev/null; then shasum -a 256 "${file}" | awk '{print $1}'
  elif command -v openssl >/dev/null; then openssl dgst -sha256 "${file}" | awk '{print $NF}'
  else echo 'sha256sum, shasum, or openssl is required' >&2; return 1
  fi
}

module_field() {
  sed -n "s/^[[:space:]]*\"$2\": \"\\([^\"]*\\)\".*/\\1/p" <<<"$1" | head -n1
}

verify_proxy_module() {
  local module="$1" version="$2" expected_commit="$3" expected_zip_sha256="$4" expected_mod_sha256="$5" expected_sum="$6" expected_gomod_sum="$7" label="$8"
  local info json
  info="$(curl --fail --silent --show-error --location "https://proxy.golang.org/${module}/@v/${version}.info")"
  assert_equals "${label} proxy commit" "${expected_commit}" "$(sed -n 's/.*"Hash":"\([^"]*\)".*/\1/p' <<<"${info}")"
  curl --fail --silent --show-error --location "https://proxy.golang.org/${module}/@v/${version}.zip" -o "${tmp_dir}/${label}.zip"
  curl --fail --silent --show-error --location "https://proxy.golang.org/${module}/@v/${version}.mod" -o "${tmp_dir}/${label}.mod"
  assert_equals "${label} proxy zip SHA-256" "${expected_zip_sha256}" "$(sha256_file "${tmp_dir}/${label}.zip")"
  assert_equals "${label} proxy module SHA-256" "${expected_mod_sha256}" "$(sha256_file "${tmp_dir}/${label}.mod")"
  json="$(GOWORK=off go mod download -json "${module}@${version}")"
  assert_equals "${label} Go module checksum" "${expected_sum}" "$(module_field "${json}" Sum)"
  assert_equals "${label} Go module file checksum" "${expected_gomod_sum}" "$(module_field "${json}" GoModSum)"
}

for command in go curl git; do command -v "${command}" >/dev/null || { echo "${command} must be on PATH" >&2; exit 1; }; done
git remote get-url "${GIT_REMOTE}" >/dev/null
tmp_dir="$(mktemp -d)"
trap 'rm -rf "${tmp_dir}"' EXIT

published_tag_object="$(git ls-remote --tags --refs "${GIT_REMOTE}" "refs/tags/${PUBLISHED_VERSION}" | awk 'NR == 1 { print $1 }')"
published_commit="$(git ls-remote "${GIT_REMOTE}" "refs/tags/${PUBLISHED_VERSION}^{}" | awk 'NR == 1 { print $1 }')"
assert_equals "${PUBLISHED_VERSION} remote tag object" "${EXPECTED_PUBLISHED_TAG_OBJECT}" "${published_tag_object}"
assert_equals "${PUBLISHED_VERSION} remote tagged commit" "${EXPECTED_PUBLISHED_COMMIT}" "${published_commit}"
ambiguous_tag_object="$(git ls-remote --tags --refs "${GIT_REMOTE}" "refs/tags/${AMBIGUOUS_VERSION}" | awk 'NR == 1 { print $1 }')"
assert_equals "${AMBIGUOUS_VERSION} lightweight tag object" "${EXPECTED_AMBIGUOUS_COMMIT}" "${ambiguous_tag_object}"
if git ls-remote "${GIT_REMOTE}" "refs/tags/${AMBIGUOUS_VERSION}^{}" | grep -q .; then
  echo "${AMBIGUOUS_VERSION} must remain a lightweight tag" >&2
  exit 1
fi

verify_proxy_module "${MODULE}" "${PUBLISHED_VERSION}" "${EXPECTED_PUBLISHED_COMMIT}" "${EXPECTED_PUBLISHED_ZIP_SHA256}" "${EXPECTED_PUBLISHED_MOD_SHA256}" "${EXPECTED_PUBLISHED_SUM}" "${EXPECTED_PUBLISHED_GOMOD_SUM}" app-kit-v0.2.1
verify_proxy_module "${MODULE}" "${AMBIGUOUS_VERSION}" "${EXPECTED_AMBIGUOUS_COMMIT}" "${EXPECTED_AMBIGUOUS_ZIP_SHA256}" "${EXPECTED_AMBIGUOUS_MOD_SHA256}" "${EXPECTED_AMBIGUOUS_SUM}" "${EXPECTED_AMBIGUOUS_GOMOD_SUM}" app-kit-v0.3.0
verify_proxy_module github.com/umesh0492/go-libs "${GOLIBS_VERSION}" "${EXPECTED_GOLIBS_COMMIT}" "${EXPECTED_GOLIBS_ZIP_SHA256}" "${EXPECTED_GOLIBS_MOD_SHA256}" "${EXPECTED_GOLIBS_SUM}" "${EXPECTED_GOLIBS_GOMOD_SUM}" go-libs-v0.2.1
verify_proxy_module github.com/umesh0492/go-fintech-india "${FINTECH_VERSION}" "${EXPECTED_FINTECH_COMMIT}" "${EXPECTED_FINTECH_ZIP_SHA256}" "${EXPECTED_FINTECH_MOD_SHA256}" "${EXPECTED_FINTECH_SUM}" "${EXPECTED_FINTECH_GOMOD_SUM}" go-fintech-india-v0.2.3

scratch_dir="$(mktemp -d)"
trap 'rm -rf "${tmp_dir}" "${scratch_dir}"' EXIT
(
  cd "${scratch_dir}"
  GOWORK=off go mod init example.com/go-app-kit-baseline-consumer >/dev/null
  GOPROXY=https://proxy.golang.org GOWORK=off go get "${MODULE}/export@${PUBLISHED_VERSION}"
  cat > main.go <<'EOF'
package main

import (
	"bytes"
	"fmt"

	kitexport "github.com/umesh0492/go-app-kit/export"
)

func main() {
	var output bytes.Buffer
	writer := kitexport.NewCSVStreamer[struct{}](&output, nil)
	fmt.Println(writer.Close())
}
EOF
  GOPROXY=https://proxy.golang.org GOWORK=off go build .
  if grep -q "${ROOT_DIR}" go.mod go.sum; then
    echo 'isolated consumer references source checkout' >&2
    exit 1
  fi
)

echo "Release baseline verification passed for ${MODULE} and published dependencies."
