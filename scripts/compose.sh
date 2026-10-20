#!/usr/bin/env bash
# Run compose with podman or docker, whichever is installed.
set -euo pipefail
if command -v podman >/dev/null 2>&1; then
  if command -v podman-compose >/dev/null 2>&1; then exec podman-compose "$@"; fi
  exec podman compose "$@"
elif command -v docker >/dev/null 2>&1; then
  exec docker compose "$@"
fi
echo "error: neither podman nor docker found. Install one: sudo pacman -S podman podman-compose" >&2
exit 1
