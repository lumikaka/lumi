#!/usr/bin/env bash
set -euo pipefail

main() {
  root_dir="$(cd "$(dirname "$0")" && pwd)"
  project_dir="${root_dir}/../.."
  tauri_dir="${root_dir}/src-tauri"
  backend_dir="${tauri_dir}/backend-darwin"
  app_path="${tauri_dir}/target/aarch64-apple-darwin/release/bundle/macos/Lumi.app"
  dmg_dir="${tauri_dir}/target/aarch64-apple-darwin/release/bundle/dmg"
  target="aarch64-apple-darwin"
  command="${1:-build}"

  if [ $# -gt 0 ]; then
    shift
  fi

  require_apple_silicon

  tauri_args=()
  while [ $# -gt 0 ]; do
    case "$1" in
      --target)
        if [ $# -lt 2 ]; then
          echo "--target requires a value." >&2
          exit 1
        fi
        target="$2"
        shift 2
        ;;
      --target=*)
        target="${1#--target=}"
        shift
        ;;
      *)
        tauri_args+=("$1")
        shift
        ;;
    esac
  done

  if [ "$target" != "aarch64-apple-darwin" ]; then
    echo "Unsupported target: ${target}. Use aarch64-apple-darwin." >&2
    exit 1
  fi

  if [ "${#tauri_args[@]}" -gt 0 ]; then
    set -- "${tauri_args[@]}"
  else
    set --
  fi

  case "$command" in
    build)
      build_backend
      build_tauri "$@"
      ;;
    app)
      build_backend
      build_tauri --bundles app "$@"
      open -W "$app_path"
      ;;
    check)
      build_backend
      node --test "$root_dir/generate-updater-manifest.test.mjs"
      (
        cd "$tauri_dir"
        cargo fmt --check
        cargo test --target "$target"
        cargo test --features desktop-updater --target "$target"
      )
      build_tauri "$@"
      verify_bundle
      ;;
    verify)
      verify_bundle
      ;;
    *)
      pnpm --dir "$root_dir" exec tauri "$command" "$@"
      ;;
  esac
}

require_apple_silicon() {
  if [ "$(uname -s)" != "Darwin" ]; then
    echo "Lumi desktop packaging currently supports only macOS Apple Silicon." >&2
    exit 1
  fi

  if [ "$(uname -m)" != "arm64" ]; then
    echo "Lumi desktop packaging currently supports only arm64 Apple Silicon hosts." >&2
    exit 1
  fi
}

build_backend() {
  pnpm --dir "${project_dir}/web" install --frozen-lockfile
  pnpm --dir "${project_dir}/web" run build

  mkdir -p "$backend_dir"
  (
    cd "$project_dir"
    CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build \
      -trimpath \
      -tags embed_frontend \
      -ldflags="-s -w" \
      -o "${backend_dir}/lumi_web" \
      ./cmd/lumi_web
  )
}

build_tauri() {
  config_json='{"bundle":{"resources":{"backend-darwin/":"backend/"}}}'
  updater_args=()

  if [ -n "${LUMI_DESKTOP_VERSION:-}" ]; then
    if ! [[ "$LUMI_DESKTOP_VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z]+([.-][0-9A-Za-z]+)*)?$ ]]; then
      echo "LUMI_DESKTOP_VERSION must be a semantic version without a leading v." >&2
      exit 1
    fi
    config_json="{\"version\":\"${LUMI_DESKTOP_VERSION}\",\"bundle\":{\"resources\":{\"backend-darwin/\":\"backend/\"}}}"
  fi

  if [ "${LUMI_DESKTOP_UPDATER:-0}" = "1" ]; then
    if [ -z "${TAURI_SIGNING_PRIVATE_KEY:-}" ] || [ -z "${TAURI_SIGNING_PRIVATE_KEY_PASSWORD:-}" ]; then
      echo "TAURI_SIGNING_PRIVATE_KEY and TAURI_SIGNING_PRIVATE_KEY_PASSWORD are required for updater artifacts." >&2
      exit 1
    fi
    if [ -n "${LUMI_DESKTOP_VERSION:-}" ]; then
      config_json="{\"version\":\"${LUMI_DESKTOP_VERSION}\",\"bundle\":{\"createUpdaterArtifacts\":true,\"resources\":{\"backend-darwin/\":\"backend/\"}}}"
    else
      config_json='{"bundle":{"createUpdaterArtifacts":true,"resources":{"backend-darwin/":"backend/"}}}'
    fi
    updater_args=(--features desktop-updater)
  fi

  pnpm --dir "$root_dir" exec tauri build \
    --config "$config_json" \
    --target "$target" \
    "${updater_args[@]}" \
    "$@"
}

verify_bundle() {
  bundled_backend="${app_path}/Contents/Resources/backend/lumi_web"
  app_version=""

  if [ ! -d "$app_path" ]; then
    echo "Tauri app bundle not found: ${app_path}" >&2
    exit 1
  fi
  if [ ! -x "$bundled_backend" ]; then
    echo "Bundled Lumi backend is missing or not executable: ${bundled_backend}" >&2
    exit 1
  fi

  if [ "$(lipo -archs "$bundled_backend")" != "arm64" ]; then
    echo "Bundled Lumi backend is not arm64: ${bundled_backend}" >&2
    exit 1
  fi

  app_version="$(/usr/libexec/PlistBuddy -c 'Print :CFBundleShortVersionString' "${app_path}/Contents/Info.plist")"
  if [ -n "${LUMI_DESKTOP_VERSION:-}" ] && [ "$app_version" != "$LUMI_DESKTOP_VERSION" ]; then
    echo "Bundled Lumi version ${app_version} does not match ${LUMI_DESKTOP_VERSION}." >&2
    exit 1
  fi

  codesign --verify --deep --strict "$app_path"
  codesign -dv --verbose=2 "$app_path"

  if [ ! -d "$dmg_dir" ]; then
    echo "Tauri DMG output directory not found: ${dmg_dir}" >&2
    exit 1
  fi

  shopt -s nullglob
  dmg_paths=("${dmg_dir}"/*.dmg)
  shopt -u nullglob
  if [ "${#dmg_paths[@]}" -ne 1 ]; then
    echo "Expected one Tauri DMG, found ${#dmg_paths[@]} in ${dmg_dir}." >&2
    exit 1
  fi
  dmg_path="${dmg_paths[0]}"
  hdiutil verify "$dmg_path"

  mount_dir="$(mktemp -d "${TMPDIR:-/tmp}/lumi-dmg.XXXXXX")"
  cleanup_mounted_dmg() {
    hdiutil detach "$mount_dir" >/dev/null 2>&1 || true
    rmdir "$mount_dir" >/dev/null 2>&1 || true
  }
  trap cleanup_mounted_dmg EXIT
  trap 'exit 1' INT TERM

  hdiutil attach -nobrowse -readonly -mountpoint "$mount_dir" "$dmg_path" >/dev/null
  mounted_app="${mount_dir}/Lumi.app"
  mounted_backend="${mounted_app}/Contents/Resources/backend/lumi_web"

  if [ ! -d "$mounted_app" ]; then
    echo "Mounted DMG does not contain Lumi.app: ${dmg_path}" >&2
    exit 1
  fi
  if [ ! -L "${mount_dir}/Applications" ]; then
    echo "Mounted DMG does not contain the Applications shortcut: ${dmg_path}" >&2
    exit 1
  fi
  if [ ! -x "$mounted_backend" ]; then
    echo "Mounted Lumi backend is missing or not executable: ${mounted_backend}" >&2
    exit 1
  fi
  if [ "$(lipo -archs "$mounted_backend")" != "arm64" ]; then
    echo "Mounted Lumi backend is not arm64: ${mounted_backend}" >&2
    exit 1
  fi
  mounted_version="$(/usr/libexec/PlistBuddy -c 'Print :CFBundleShortVersionString' "${mounted_app}/Contents/Info.plist")"
  if [ "$mounted_version" != "$app_version" ]; then
    echo "Mounted Lumi version ${mounted_version} does not match built app version ${app_version}." >&2
    exit 1
  fi
  codesign --verify --deep --strict "$mounted_app"

  hdiutil detach "$mount_dir" >/dev/null
  if [ -d "$mount_dir" ]; then
    rmdir "$mount_dir"
  fi
  trap - EXIT INT TERM
  echo "Verified ${app_path} and ${dmg_path}"
}

main "$@"
