#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd -- "${script_dir}/../../.." && pwd)"

entrypoint="${SCANDRIX_CLI_ENTRYPOINT:-${repo_root}/bin/scandrix}"
api_url="${SCANDRIX_API_URL:-http://localhost:8080}"
verbose="${SCANDRIX_VERBOSE:-1}"

print_help() {
  cat <<EOF
Run the local ScanDrix CLI build from this repository.

Usage:
  $(basename "$0") <command> [args...]

Environment:
  SCANDRIX_API_URL         Defaults to http://localhost:8080
  SCANDRIX_VERBOSE         Defaults to 1
  SCANDRIX_CLI_ENTRYPOINT  Defaults to <repo>/bin/scandrix

Examples:
  $(basename "$0") --help
  $(basename "$0") auth status
  $(basename "$0") review --prompt-only
EOF
}

if [[ "${1:-}" == "--help" || "${1:-}" == "-h" ]]; then
  print_help
  exit 0
fi

if [[ $# -eq 0 ]]; then
  print_help
  exit 1
fi

if [[ ! -f "$entrypoint" ]]; then
  echo "Building local ScanDrix CLI..."
  go build -o "${entrypoint}" "${repo_root}/cmd/cli"
fi

exec env \
  SCANDRIX_API_URL="$api_url" \
  SCANDRIX_VERBOSE="$verbose" \
  "$entrypoint" "$@"
