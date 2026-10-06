#!/bin/sh
set -eu
repo=''
version=''
install_dir="${TOYS_INSTALL_DIR:-$HOME/.local/bin}"
while [ "$#" -gt 0 ]; do
  case "$1" in
    --repo) repo="$2"; shift 2 ;;
    --version) version="$2"; shift 2 ;;
    --dir) install_dir="$2"; shift 2 ;;
    *) echo "Usage: sh install.sh --repo OWNER/REPO --version v0.1.0 [--dir PATH]" >&2; exit 2 ;;
  esac
done
printf '%s' "$repo" | LC_ALL=C grep -Eq '^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$' || { echo 'Specify --repo OWNER/REPO' >&2; exit 2; }
printf '%s' "$version" | LC_ALL=C grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+$' || { echo 'Specify a stable --version such as v0.1.0' >&2; exit 2; }
case "$(uname -s)" in Darwin) os=macos ;; Linux) os=linux ;; *) echo 'Use install.ps1 on Windows' >&2; exit 2 ;; esac
case "$(uname -m)" in arm64|aarch64) arch=arm64 ;; x86_64|amd64) arch=x64 ;; *) echo 'Unsupported architecture' >&2; exit 2 ;; esac
asset="supabase-toys_${version}_${os}_${arch}.tar.gz"
base="https://github.com/${repo}/releases/download/${version}"
work_dir=$(mktemp -d)
trap 'rm -rf "$work_dir"' EXIT HUP INT TERM
curl -fSL --proto '=https' --tlsv1.2 "$base/$asset" -o "$work_dir/$asset"
curl -fSL --proto '=https' --tlsv1.2 "$base/SHA256SUMS" -o "$work_dir/SHA256SUMS"
expected=$(awk -v name="$asset" '$2 == name {print $1}' "$work_dir/SHA256SUMS")
[ "${#expected}" -eq 64 ] || { echo 'Missing or invalid release checksum' >&2; exit 1; }
if command -v sha256sum >/dev/null 2>&1; then actual=$(sha256sum "$work_dir/$asset" | awk '{print $1}'); else actual=$(shasum -a 256 "$work_dir/$asset" | awk '{print $1}'); fi
[ "$actual" = "$expected" ] || { echo 'Checksum verification failed; nothing installed' >&2; exit 1; }
tar -xzf "$work_dir/$asset" -C "$work_dir" supabase-toys
mkdir -p "$install_dir"
install -m 755 "$work_dir/supabase-toys" "$install_dir/supabase-toys"
echo "Installed $version to $install_dir/supabase-toys"
echo "Add $install_dir to PATH if needed. Run supabase-toys --help."

