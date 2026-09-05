#!/usr/bin/env bash
set -euo pipefail

readonly protoc_bin="${PROTOC:-$(command -v protoc || true)}"
readonly protoc_version="libprotoc 35.1"
readonly plugin_version="v1.33.0"
readonly go_toolchain="${SCHEMA_CACHE_GO_TOOLCHAIN:-go1.25.9}"
readonly root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly proto_dir="${root}/internal/cli/schemacachepb"
readonly mode="${1:-generate}"

if [[ "${mode}" != "generate" && "${mode}" != "--check" ]]; then
  echo "usage: $0 [--check]" >&2
  exit 2
fi

if [[ ! -x "${protoc_bin}" ]]; then
  echo "protoc not found or not executable: ${protoc_bin}" >&2
  exit 1
fi
if [[ "$("${protoc_bin}" --version)" != "${protoc_version}" ]]; then
  echo "protoc version mismatch: want ${protoc_version}" >&2
  exit 1
fi

tool_dir="$(mktemp -d "${TMPDIR:-/tmp}/dws-protoc-gen-go.XXXXXX")"
trap 'rm -rf "${tool_dir}"' EXIT
if [[ "$(GOTOOLCHAIN="${go_toolchain}" go env GOVERSION)" != "${go_toolchain}" ]]; then
  echo "Go toolchain mismatch: want ${go_toolchain}" >&2
  exit 1
fi
GOTOOLCHAIN="${go_toolchain}" GOBIN="${tool_dir}" go install "google.golang.org/protobuf/cmd/protoc-gen-go@${plugin_version}"
if [[ "$("${tool_dir}/protoc-gen-go" --version)" != "protoc-gen-go ${plugin_version}" ]]; then
  echo "protoc-gen-go version mismatch: want ${plugin_version}" >&2
  exit 1
fi

output_dir="${proto_dir}"
if [[ "${mode}" == "--check" ]]; then
  output_dir="${tool_dir}/generated"
  mkdir "${output_dir}"
fi

"${protoc_bin}" \
  --plugin="protoc-gen-go=${tool_dir}/protoc-gen-go" \
  --proto_path="${proto_dir}" \
  --go_out="${output_dir}" \
  --go_opt=paths=source_relative \
  "${proto_dir}/schema_cache.proto"

if [[ "${mode}" == "--check" ]]; then
  cmp "${output_dir}/schema_cache.pb.go" "${proto_dir}/schema_cache.pb.go"
fi
