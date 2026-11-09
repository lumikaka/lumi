#!/bin/bash
# macOS system Bash 3.2 + curl, shasum, tar, plutil; no package manager/API key.
set -euo pipefail

usage() {
  cat <<'EOF'
Install or upgrade Lumi for macOS Codex App (not TUI/IDE).
Usage: bash install-codex.sh [--version VERSION] [--install-dir ABS_PATH] [--cache-dir ABS_PATH]
       bash install-codex.sh --from-dir RELEASE_ASSETS [--install-dir ABS_PATH]
       bash install-codex.sh --uninstall [--install-dir ABS_PATH]
CODEX_HOME defaults to ~/.codex. Default install dir: ~/.local/share/lumi-codex
Downloads use https://github.com/lumikaka/lumi/releases (SHA256 verified).
Verified downloads are reused from ~/Library/Caches/lumi-codex (or --cache-dir).
Progress goes to stderr; successful installation prints a JSON result to stdout.
The App-bundled codex is preferred; PATH codex is the fallback.
No API keys, sudo, shell profile edits, project trust changes or hooks.
EOF
}
stage=preflight
die() { printf 'Lumi [+%ss] [%s]: %s\n' "$SECONDS" "$stage" "$*" >&2; exit 1; }
log() { printf 'Lumi [+%ss] [%s]: %s\n' "$SECONDS" "$stage" "$*" >&2; }
json_get() {
  local value
  if value="$(/usr/bin/plutil -extract "$2" raw -o - "$1" 2>&1)"; then
    printf '%s\n' "$value"
  else
    printf '%s\n' "$value" >&2
    return 1
  fi
}
version=''
from_dir=''
install_dir="${HOME}/.local/share/lumi-codex"
cache_dir="${HOME}/Library/Caches/lumi-codex"
uninstall=false
while [ "$#" -gt 0 ]; do
  case "$1" in
    --version|--install-dir|--from-dir|--cache-dir)
      [ "$#" -ge 2 ] && [ -n "$2" ] || die "$1 requires a value"
      case "$1" in
        --version) version="${2#v}" ;;
        --install-dir) install_dir="$2" ;;
        --from-dir) from_dir="$2" ;;
        --cache-dir) cache_dir="$2" ;;
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
case "$cache_dir" in /*) ;; *) die '--cache-dir must be absolute.' ;; esac
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
"$codex_bin" plugin list --help >/dev/null

selector='lumi@lumi-local'
marker='.lumi-codex-install'
if [ -e "$install_dir" ]; then
  [ ! -L "$install_dir" ] && [ -d "$install_dir" ] || die 'Installation root must be a real directory.'
  [ ! -e "$install_dir/.install-lock" ] || die 'Another installation is running (or .install-lock needs manual recovery).'
  [ ! -e "$install_dir/.previous" ] || die 'Interrupted update found (.previous). See install.md recovery instructions.'
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

# Unverified downloads and transaction files are private and disposable.
umask 077
scratch="$(mktemp -d "${TMPDIR:-/tmp}/lumi-codex-install.XXXXXX")"
locked=false
switched=false
committed=false
registration_changed=false
market_added=false
marker_created=false
cleanup() {
  result=$?
  trap - EXIT
  set +e
  if $switched && ! $committed; then
    rm -rf "$install_dir/current"
    if [ -d "$install_dir/.previous" ]; then
      mv "$install_dir/.previous" "$install_dir/current"
      # Restore only Lumi's registration, never replace a user's config file.
      if $registration_changed; then
        "$codex_bin" plugin marketplace add "$install_dir/current/marketplace" --json >&2
        "$codex_bin" plugin add "$selector" --json >&2
      fi
    elif $registration_changed; then
      "$codex_bin" plugin remove "$selector" --json >&2 || true
      if $market_added; then "$codex_bin" plugin marketplace remove lumi-local --json >&2; fi
    fi
    log 'Installation did not complete; previous payload restored when available. See the error above.' >&2
  fi
  if [ "$result" -ne 0 ] && $marker_created; then rm -f "$install_dir/$marker"; fi
  if $locked; then rmdir "$install_dir/.install-lock"; fi
  rm -rf "$scratch"
  exit "$result"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

mkdir -p "$install_dir"
install_dir="$(cd "$install_dir" && pwd -P)"
mkdir "$install_dir/.install-lock" 2>/dev/null || die 'Another installation is running (or .install-lock needs manual recovery).'
locked=true
[ ! -e "$install_dir/.previous" ] || die 'Interrupted update found (.previous). See install.md recovery instructions.'
market_registered=false
check_market_root() {
  local market_root="$1"
  [ "$market_root" = "$install_dir/current/marketplace" ] || die "lumi-local already belongs to $market_root; use its original install directory or uninstall it first."
  market_registered=true
}

# A broken unrelated marketplace must not block Lumi. A broken Lumi marketplace
# still fails here, before any network request; never interpret that as absence.
log 'Checking the Lumi marketplace before downloading assets.'
"$codex_bin" plugin list --marketplace lumi-local --available --json > "$scratch/plugins-before.json" || die 'Cannot inspect lumi-local. Repair its reported source/configuration before retrying.'
for collection in installed available; do
  count="$(json_get "$scratch/plugins-before.json" "$collection")"
  index=0
  while [ "$index" -lt "$count" ]; do
    [ "$(json_get "$scratch/plugins-before.json" "$collection.$index.marketplaceName")" = lumi-local ] || die 'Unexpected marketplace in the scoped plugin list.'
    [ "$(json_get "$scratch/plugins-before.json" "$collection.$index.marketplaceSource.sourceType")" = local ] || die 'lumi-local belongs to a non-local source.'
    check_market_root "$(json_get "$scratch/plugins-before.json" "$collection.$index.marketplaceSource.source")"
    index=$((index + 1))
  done
done
if ! $market_registered; then
  # An empty scoped list can mean no market OR an empty existing catalog. Use
  # global metadata when available. Otherwise marketplace add's own collision
  # guard protects installation; uninstall must prove ownership before removal.
  if "$codex_bin" plugin marketplace list --json > "$scratch/marketplaces.json" 2> "$scratch/marketplaces.err"; then
    index=0
    while name="$(json_get "$scratch/marketplaces.json" "marketplaces.$index.name" 2>/dev/null)"; do
      if [ "$name" = lumi-local ]; then
        check_market_root "$(json_get "$scratch/marketplaces.json" "marketplaces.$index.root")"
      fi
      index=$((index + 1))
    done
  elif $uninstall; then
    cat "$scratch/marketplaces.err" >&2
    die 'Cannot verify ownership of an empty Lumi marketplace for uninstall.'
  else
    log 'No Lumi plugin entries. Other marketplaces could not be listed; Codex will enforce source ownership during registration.'
  fi
fi
if $uninstall; then
  stage=uninstall
  "$codex_bin" plugin remove "$selector" --json
  if $market_registered; then "$codex_bin" plugin marketplace remove lumi-local --json; fi
  rm -rf "$install_dir/current"
  rm "$install_dir/$marker"
  log 'Uninstalled Lumi plugin and managed binaries. Projects, application data and download cache were preserved.'
  log 'Close old App chats; installed tools may remain in those chats until they end.'
  exit 0
fi

if ! $uninstall; then
  stage=download
  base='https://github.com/lumikaka/lumi/releases'
  download() {
    log "Downloading ${1##*/}"
    curl --fail --location --silent --show-error --retry 2 --connect-timeout 20 --max-time 300 --proto '=https' --proto-redir '=https' \
      --write-out '%{size_download} bytes in %{time_total}s\n' "$1" -o "$2" >&2
  }
  if [ -n "$from_dir" ]; then
    from_dir="$(cd "$from_dir" && pwd -P)"
    [ -n "$version" ] || version="$(cat "$from_dir/lumi-codex-version.txt")"
  elif [ -z "$version" ]; then
    download "$base/latest/download/lumi-codex-version.txt" "$scratch/version"
    version="$(cat "$scratch/version")"
  fi
  [[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z]+([.-][0-9A-Za-z]+)*)?$ ]] || die 'Invalid release version.'
  log "Release: $version; architecture: $arch"
  # Resolve latest once, then fetch every artifact from that immutable tag.
  fetch() {
    if [ -n "$from_dir" ]; then cp "$from_dir/$1" "$scratch/$1";
    else download "$base/download/v$version/$1" "$scratch/$1"; fi
  }
  verify() {
    local asset="$1" expected actual cached
    fetch "$asset.sha256"
    expected="$(awk 'NR==1 {print $1}' "$scratch/$asset.sha256")"
    [[ "$expected" =~ ^[a-fA-F0-9]{64}$ ]] || die "Invalid SHA256 for $asset"
    expected="$(printf '%s' "$expected" | tr 'A-F' 'a-f')"
    cached="$cache_dir/$version/$arch/$expected/$asset"
    # --from-dir always validates the supplied files, even if a good network
    # cache exists. It is also used by release acceptance and corruption tests.
    if [ -z "$from_dir" ] && [ -f "$cached" ]; then
      cp "$cached" "$scratch/$asset"
      actual="$(shasum -a 256 "$scratch/$asset" | awk '{print $1}')"
      if [ "$actual" = "$expected" ]; then
        log "Cache hit (SHA256 verified): $asset"
        return
      fi
      log "Cached SHA256 mismatch; downloading $asset again."
    fi
    fetch "$asset"
    actual="$(shasum -a 256 "$scratch/$asset" | awk '{print $1}')"
    [ "$expected" = "$actual" ] || die "SHA256 mismatch: $asset"
    log "SHA256 verified: $asset ($actual)"
    if [ -z "$from_dir" ]; then
      mkdir -p "$(dirname "$cached")"
      # Atomic publication also permits concurrent installs with separate roots.
      local cache_tmp
      cache_tmp="$(mktemp "$(dirname "$cached")/.download.XXXXXX")"
      cp "$scratch/$asset" "$cache_tmp"
      mv -f "$cache_tmp" "$cached"
    fi
  }
  verify "lumi-darwin-$arch"
  verify 'lumi-codex-plugin.tar.gz'
  stage=validate
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

stage=install
[ -f "$install_dir/$marker" ] || marker_created=true
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
"$codex_bin" plugin marketplace add "$install_dir/current/marketplace" --json > "$scratch/marketplace-add.json"
registration_changed=true
if [ "$(json_get "$scratch/marketplace-add.json" alreadyAdded)" = false ]; then market_added=true; fi
# add refreshes the installed plugin from the local source, including upgrades.
"$codex_bin" plugin add "$selector" --json > "$scratch/plugin-add.json"
stage=verify
"$codex_bin" plugin list --marketplace lumi-local --json > "$scratch/plugins-after.json"
verified=false
index=0
while plugin_id="$(json_get "$scratch/plugins-after.json" "installed.$index.pluginId" 2>/dev/null)"; do
  if [ "$plugin_id" = "$selector" ]; then
    [ "$(json_get "$scratch/plugins-after.json" "installed.$index.version")" = "$version" ] || die 'Installed plugin version does not match the release.'
    [ "$(json_get "$scratch/plugins-after.json" "installed.$index.installed")" = true ] || die 'Plugin is not installed.'
    [ "$(json_get "$scratch/plugins-after.json" "installed.$index.enabled")" = true ] || die 'Plugin is not enabled; check Codex policy.'
    verified=true
  fi
  index=$((index + 1))
done
$verified || die 'Lumi is missing from the installed plugin list.'
installed_path="$(json_get "$scratch/plugin-add.json" installedPath)"
case "$installed_path" in /*) ;; *) die 'Codex returned a non-absolute plugin cache path.' ;; esac
cached_mcp="$installed_path/.mcp.json"
[ "$(json_get "$installed_path/.codex-plugin/plugin.json" version)" = "$version" ] || die 'Cached plugin version does not match.'
[ "$(json_get "$cached_mcp" mcpServers.lumi.command)" = "$binary" ] && [ -x "$binary" ] || die 'Cached MCP command does not point to the installed CLI.'
# Current plugins pass --host codex; a bare "mcp" is still accepted for older packages.
mcp_args="$(json_get "$cached_mcp" mcpServers.lumi.args)"
[ "$(json_get "$cached_mcp" mcpServers.lumi.args.0)" = mcp ] && { [ "$mcp_args" = 1 ] || { [ "$mcp_args" = 3 ] && [ "$(json_get "$cached_mcp" mcpServers.lumi.args.1)" = --host ] && [ "$(json_get "$cached_mcp" mcpServers.lumi.args.2)" = codex ]; }; } || die 'Unexpected cached MCP arguments.'
if /usr/bin/plutil -extract mcpServers.lumi.cwd json -o /dev/null "$cached_mcp" >/dev/null 2>&1; then die 'Cached MCP must not override the chat directory.'; fi
[ -f "$installed_path/skills/create-picture-book/SKILL.md" ] || die 'Installed creation skill is missing.'
printf '%s\n' '{"success":true,"data":{"installed":true,"enabled":true}}' > "$scratch/result.json"
result_string() { /usr/bin/plutil -insert "data.$1" -string "$2" "$scratch/result.json"; }
result_string plugin_id "$selector"
result_string lumi_version "$version"
result_string codex_path "$codex_bin"
result_string codex_version "$codex_version"
result_string codex_home "$CODEX_HOME"
result_string cli_path "$binary"
result_string installed_path "$installed_path"
committed=true
rm -rf "$install_dir/.previous"
/usr/bin/plutil -convert json -r -o - "$scratch/result.json"
printf '\n'
stage=complete
log "Installed $selector ($version). CLI: $binary"
log 'Open a NEW chat in Codex App to load the plugin. Trust the project to activate its write-protection configuration.'
log 'Existing Lumi CLI servers keep running; if cli_upgrade_required appears, finish active work, run lumi stop, then open a new chat.'
log "Uninstall: bash \"$install_dir/current/install-codex.sh\" --uninstall --install-dir \"$install_dir\" (use the same CODEX_HOME)"
