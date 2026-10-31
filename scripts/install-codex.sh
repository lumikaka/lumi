#!/bin/bash
# macOS system Bash 3.2 + curl, shasum, tar, plutil; no package manager/API key.
set -euo pipefail

usage() {
  cat <<'EOF'
Install or upgrade Lumi for macOS Codex App (not TUI/IDE).
Usage: bash install-codex.sh [--version VERSION] [--install-dir ABS_PATH]
       bash install-codex.sh --from-dir RELEASE_ASSETS [--install-dir ABS_PATH]
       bash install-codex.sh --uninstall [--install-dir ABS_PATH]
CODEX_HOME defaults to ~/.codex. Default install dir: ~/.local/share/lumi-codex
Downloads use https://github.com/lumikaka/lumi/releases (SHA256 verified).
The App-bundled codex is preferred; PATH codex is the fallback.
No API keys, sudo, shell profile edits, project trust changes or hooks.
EOF
}
die() { printf 'Lumi: %s\n' "$*" >&2; exit 1; }
log() { printf 'Lumi: %s\n' "$*"; }
version=''
from_dir=''
install_dir="${HOME}/.local/share/lumi-codex"
uninstall=false
while [ "$#" -gt 0 ]; do
  case "$1" in
    --version|--install-dir|--from-dir)
      [ "$#" -ge 2 ] && [ -n "$2" ] || die "$1 requires a value"
      case "$1" in
        --version) version="${2#v}" ;;
        --install-dir) install_dir="$2" ;;
        --from-dir) from_dir="$2" ;;
      esac
      shift 2 ;;
    --uninstall) uninstall=true; shift ;;
    --help|-h) usage; exit 0 ;;
    *) die "Unknown option: $1" ;;
  esac
done
[ "$(uname -s)" = Darwin ] || die 'Only macOS Codex App is supported.'
case "$(uname -m)" in arm64) arch=arm64 ;; x86_64) arch=amd64 ;; *) die 'Unsupported Mac architecture.' ;; esac
case "$install_dir" in /*) ;; *) die '--install-dir must be absolute.' ;; esac
[ "$install_dir" != / ] && [ "$install_dir" != "$HOME" ] || die 'Use a dedicated installation directory.'
export CODEX_HOME="${CODEX_HOME:-$HOME/.codex}"
case "$CODEX_HOME" in /*) ;; *) die 'CODEX_HOME must be absolute.' ;; esac

codex_bin='/Applications/ChatGPT.app/Contents/Resources/codex-cli/bin/codex'
if [ ! -x "$codex_bin" ]; then
  codex_bin="$(command -v codex || true)"
  [ -n "$codex_bin" ] && [ -x "$codex_bin" ] || die 'Install Codex App first; no bundled or PATH codex was found.'
  case "$codex_bin" in /*) ;; *) codex_bin="$(cd "$(dirname "$codex_bin")" && pwd -P)/$(basename "$codex_bin")" ;; esac
  log 'App-bundled CLI not found; using PATH fallback. A working Codex App is still required.'
fi
log "Codex executable: $codex_bin"
codex_version="$("$codex_bin" --version)"
log "Codex version: $codex_version"
log "CODEX_HOME: $CODEX_HOME"
"$codex_bin" plugin add --help >/dev/null
"$codex_bin" plugin marketplace add --help >/dev/null

selector='lumi@lumi-local'
marker='.lumi-codex-install'
if [ -e "$install_dir" ]; then
  [ ! -L "$install_dir" ] && [ -d "$install_dir" ] || die 'Installation root must be a real directory.'
  if [ -f "$install_dir/$marker" ]; then
    [ "$(cat "$install_dir/$marker")" = "$CODEX_HOME" ] || die 'This installation belongs to a different CODEX_HOME.'
  elif [ -n "$(ls -A "$install_dir")" ]; then
    die 'Refusing to overwrite a non-empty directory not owned by this installer.'
  fi
fi
if $uninstall && [ ! -f "$install_dir/$marker" ]; then
  log 'Already uninstalled; nothing to remove.'
  exit 0
fi

# All staged downloads are private. Only current/ and our marker are managed.
scratch="$(mktemp -d "${TMPDIR:-/tmp}/lumi-codex-install.XXXXXX")"
locked=false
switched=false
committed=false
registration_changed=false
cleanup() {
  result=$?
  trap - EXIT
  if $switched && ! $committed; then
    rm -rf "$install_dir/current"
    if [ -d "$install_dir/.previous" ]; then
      mv "$install_dir/.previous" "$install_dir/current"
      # Restore only Lumi's registration, never replace a user's config file.
      "$codex_bin" plugin marketplace add "$install_dir/current/marketplace" --json >&2 || true
      "$codex_bin" plugin add "$selector" --json >&2 || true
    elif $registration_changed; then
      "$codex_bin" plugin remove "$selector" --json >&2 || true
      "$codex_bin" plugin marketplace remove lumi-local --json >&2 || true
    fi
    log 'Installation did not complete; previous payload restored when available. See the error above.' >&2
  fi
  if $locked; then rmdir "$install_dir/.install-lock"; fi
  rm -rf "$scratch"
  exit "$result"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

if ! $uninstall; then
  base='https://github.com/lumikaka/lumi/releases'
  download() { curl --fail --location --silent --show-error --retry 2 --connect-timeout 20 --max-time 300 --proto '=https' --proto-redir '=https' "$1" -o "$2"; }
  if [ -n "$from_dir" ]; then
    from_dir="$(cd "$from_dir" && pwd -P)"
    [ -n "$version" ] || version="$(cat "$from_dir/lumi-codex-version.txt")"
  elif [ -z "$version" ]; then
    download "$base/latest/download/lumi-codex-version.txt" "$scratch/version"
    version="$(cat "$scratch/version")"
  fi
  [[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z]+([.-][0-9A-Za-z]+)*)?$ ]] || die 'Invalid release version.'
  # Resolve latest once, then fetch every artifact from that immutable tag.
  fetch() {
    if [ -n "$from_dir" ]; then cp "$from_dir/$1" "$scratch/$1";
    else download "$base/download/v$version/$1" "$scratch/$1"; fi
  }
  verify() {
    asset="$1"
    fetch "$asset"
    fetch "$asset.sha256"
    expected="$(awk 'NR==1 {print $1}' "$scratch/$asset.sha256")"
    [[ "$expected" =~ ^[a-fA-F0-9]{64}$ ]] || die "Invalid SHA256 for $asset"
    actual="$(shasum -a 256 "$scratch/$asset" | awk '{print $1}')"
    [ "$(printf '%s' "$expected" | tr 'A-F' 'a-f')" = "$actual" ] || die "SHA256 mismatch: $asset"
    log "SHA256 verified: $asset ($actual)"
  }
  verify "lumi-darwin-$arch"
  verify 'lumi-codex-plugin.tar.gz'
  # Check members before extraction; plugin packages contain only regular files/directories.
  tar -tzf "$scratch/lumi-codex-plugin.tar.gz" > "$scratch/members"
  while IFS= read -r member; do
    case "$member" in lumi/|lumi/*) ;; *) die 'Unexpected plugin archive root.' ;; esac
    case "/${member%/}/" in */../*|*/./*|*//* ) die 'Unsafe plugin archive path.' ;; esac
  done < "$scratch/members"
  tar -tvzf "$scratch/lumi-codex-plugin.tar.gz" > "$scratch/types"
  LC_ALL=C awk 'substr($0,1,1)!="-" && substr($0,1,1)!="d" {exit 1}' "$scratch/types" || die 'Plugin archive contains links or special files.'
  mkdir -p "$scratch/payload/bin" "$scratch/payload/marketplace/plugins" "$scratch/payload/marketplace/.agents/plugins"
  tar -xzf "$scratch/lumi-codex-plugin.tar.gz" -C "$scratch/payload/marketplace/plugins"
  plugin="$scratch/payload/marketplace/plugins/lumi"
  /usr/bin/plutil -convert json -o /dev/null "$plugin/.codex-plugin/plugin.json"
  /usr/bin/plutil -convert json -o /dev/null "$plugin/.mcp.json"
  [ "$(/usr/bin/plutil -extract name raw -o - "$plugin/.codex-plugin/plugin.json")" = lumi ] || die 'Unexpected plugin identity.'
  [ "$(/usr/bin/plutil -extract version raw -o - "$plugin/.codex-plugin/plugin.json")" = "$version" ] || die 'Plugin and binary release versions differ.'
  mv "$scratch/lumi-darwin-$arch" "$scratch/payload/bin/lumi"
  chmod 755 "$scratch/payload/bin/lumi"
  "$scratch/payload/bin/lumi" --help >/dev/null
fi

mkdir -p "$install_dir"
install_dir="$(cd "$install_dir" && pwd -P)"
mkdir "$install_dir/.install-lock" 2>/dev/null || die 'Another installation is running (or .install-lock needs manual recovery).'
locked=true
[ ! -e "$install_dir/.previous" ] || die 'Interrupted update found (.previous). See install.md recovery instructions.'
"$codex_bin" plugin marketplace list --json > "$scratch/marketplaces.json"
market_registered=false
index=0
while name="$(/usr/bin/plutil -extract "marketplaces.$index.name" raw -o - "$scratch/marketplaces.json" 2>/dev/null)"; do
  if [ "$name" = lumi-local ]; then
    market_root="$(/usr/bin/plutil -extract "marketplaces.$index.root" raw -o - "$scratch/marketplaces.json")"
    [ "$market_root" = "$install_dir/current/marketplace" ] || die "lumi-local already belongs to $market_root; use its original install directory or uninstall it first."
    market_registered=true
  fi
  index=$((index + 1))
done
if $uninstall; then
  "$codex_bin" plugin remove "$selector" --json
  if $market_registered; then "$codex_bin" plugin marketplace remove lumi-local --json; fi
  rm -rf "$install_dir/current"
  rm "$install_dir/$marker"
  log 'Uninstalled Lumi plugin and managed binaries. Projects and Lumi application data were preserved.'
  log 'Close old App chats; installed tools may remain in those chats until they end.'
  exit 0
fi
printf '%s\n' "$CODEX_HOME" > "$install_dir/$marker"
binary="$install_dir/current/bin/lumi"
# No cwd: Codex starts this stdio server in the chat's project directory.
/usr/bin/plutil -replace mcpServers.lumi.command -string "$binary" "$plugin/.mcp.json"
cat > "$scratch/payload/marketplace/.agents/plugins/marketplace.json" <<'EOF'
{
  "name": "lumi-local",
  "interface": {"displayName": "Lumi Local"},
  "plugins": [{
    "name": "lumi",
    "source": {"source": "local", "path": "./plugins/lumi"},
    "policy": {"installation": "AVAILABLE", "authentication": "ON_INSTALL"},
    "category": "Productivity"
  }]
}
EOF
printf '%s\n' "$version" > "$scratch/payload/version.txt"
# Keep the installer for offline removal, even when the original was temporary.
cp "$0" "$scratch/payload/install-codex.sh"
if [ -d "$install_dir/current" ]; then mv "$install_dir/current" "$install_dir/.previous"; fi
switched=true
mv "$scratch/payload" "$install_dir/current"
log "Installing/upgrading Lumi $version at $binary"
"$codex_bin" plugin marketplace add "$install_dir/current/marketplace" --json
registration_changed=true
# add refreshes the installed plugin from the local source, including upgrades.
"$codex_bin" plugin add "$selector" --json
committed=true
rm -rf "$install_dir/.previous"
log "Installed $selector ($version). CLI: $binary"
log 'Open a NEW chat in Codex App to load the plugin. Trust the project to activate its write-protection configuration.'
log 'Existing Lumi CLI servers keep running; if cli_upgrade_required appears, finish active work, run lumi stop, then open a new chat.'
log "Uninstall: bash \"$install_dir/current/install-codex.sh\" --uninstall --install-dir \"$install_dir\" (use the same CODEX_HOME)"
