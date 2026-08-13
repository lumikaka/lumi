#!/usr/bin/env bash
set -euo pipefail

main() {
  root_dir="$(cd "$(dirname "$0")" && pwd)"
  project_dir="${root_dir}/../.."
  tauri_dir="${root_dir}/src-tauri"
  backend_dir="${tauri_dir}/backend-darwin"
  app_path="${tauri_dir}/target/aarch64-apple-darwin/release/bundle/macos/Lumi.app"
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
      build_tauri "$@"
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
    --bundles app \
    "${updater_args[@]}" \
    "$@"
}

verify_bundle() {
  bundled_backend="${app_path}/Contents/Resources/backend/lumi_web"

  if [ ! -d "$app_path" ]; then
    echo "Tauri app bundle not found: ${app_path}" >&2
    exit 1
  fi
  if [ ! -x "$bundled_backend" ]; then
    echo "Bundled Lumi backend is missing or not executable: ${bundled_backend}" >&2
    exit 1
  fi

  codesign --verify --deep --strict "$app_path"
  echo "Verified ${app_path}"
}

main "$@"
