#!/usr/bin/env bash

set -euo pipefail

build_dir="${1:-site/public}"

if [[ ! -d "${build_dir}" ]]; then
  echo "Site build directory does not exist: ${build_dir}" >&2
  exit 1
fi

docs_slugs=(
  installation
  providers
  first-picture-book
  story-and-chapters
  premise-assets
  storyboards-and-images
  preview-and-export
  local-projects
  troubleshooting
)

required_files=(
  index.html
  404.html
  search.json
  robots.txt
  sitemap.xml
  docs/index.html
  en/index.html
  en/404.html
  en/search.json
  en/docs/index.html
)

for slug in "${docs_slugs[@]}"; do
  required_files+=("docs/${slug}/index.html")
  required_files+=("en/docs/${slug}/index.html")
done

for relative_file in "${required_files[@]}"; do
  if [[ ! -f "${build_dir}/${relative_file}" ]]; then
    echo "Missing generated route or asset: ${relative_file}" >&2
    exit 1
  fi
done

for index_file in "${build_dir}/search.json" "${build_dir}/en/search.json"; do
  jq -e '
    length == 9 and
    all(.[];
      (.title | type == "string" and length > 0) and
      (.content | type == "string") and
      (.url | type == "string" and length > 0 and (startswith("/") | not))
    )
  ' "${index_file}" >/dev/null
done

grep -q 'rel=canonical' "${build_dir}/index.html"
grep -q 'hreflang=en-US' "${build_dir}/index.html"
grep -q 'hreflang=zh-CN' "${build_dir}/en/index.html"

if grep -R -E --include='*.html' '(href|src)=("|'"'"')?/(css|fonts|images|js)/' "${build_dir}" >/dev/null; then
  echo "Found a root-absolute runtime asset URL in generated HTML." >&2
  exit 1
fi

if grep -R -E --include='*.css' 'url\(("|'"'"')?/' "${build_dir}" >/dev/null; then
  echo "Found a root-absolute runtime asset URL in generated CSS." >&2
  exit 1
fi

echo "Verified ${#required_files[@]} generated routes/assets and both language search indexes."
