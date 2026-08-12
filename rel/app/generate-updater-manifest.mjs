#!/usr/bin/env node

import { readFileSync, writeFileSync } from "node:fs";
import { pathToFileURL } from "node:url";

const STABLE_VERSION_PATTERN = /^\d+\.\d+\.\d+$/;
const REPOSITORY_PATTERN = /^[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+$/;

export function generateUpdaterManifest({
  version,
  repository,
  pubDate,
  macosSignature,
  windowsSignature,
}) {
  if (!STABLE_VERSION_PATTERN.test(version)) {
    throw new Error(
      "version must be a stable semantic version without a leading v",
    );
  }
  if (!REPOSITORY_PATTERN.test(repository)) {
    throw new Error("repository must use the owner/name form");
  }
  if (Number.isNaN(Date.parse(pubDate))) {
    throw new Error("pub-date must be an RFC 3339 timestamp");
  }

  const signatures = {
    macos: macosSignature.trim(),
    windows: windowsSignature.trim(),
  };
  if (!signatures.macos || !signatures.windows) {
    throw new Error("both updater signatures must be non-empty");
  }

  const tag = `v${version}`;
  const releaseBase = `https://github.com/${repository}/releases/download/${tag}`;
  return {
    version,
    notes: `Desktop builds for ${tag}.`,
    pub_date: pubDate,
    platforms: {
      "darwin-aarch64": {
        signature: signatures.macos,
        url: `${releaseBase}/Lumi-macos-aarch64.app.tar.gz`,
      },
      "windows-x86_64": {
        signature: signatures.windows,
        url: `${releaseBase}/Lumi-windows-x64-setup.exe`,
      },
    },
  };
}

function parseArguments(argumentsList) {
  const values = new Map();
  for (let index = 0; index < argumentsList.length; index += 2) {
    const name = argumentsList[index];
    const value = argumentsList[index + 1];
    if (!name?.startsWith("--") || value === undefined) {
      throw new Error(`invalid argument near ${name ?? "<end>"}`);
    }
    values.set(name.slice(2), value);
  }

  for (const name of [
    "version",
    "repository",
    "pub-date",
    "macos-signature",
    "windows-signature",
    "output",
  ]) {
    if (!values.has(name)) {
      throw new Error(`missing --${name}`);
    }
  }
  return values;
}

export function run(argumentsList) {
  const values = parseArguments(argumentsList);
  const manifest = generateUpdaterManifest({
    version: values.get("version"),
    repository: values.get("repository"),
    pubDate: values.get("pub-date"),
    macosSignature: readFileSync(values.get("macos-signature"), "utf8"),
    windowsSignature: readFileSync(values.get("windows-signature"), "utf8"),
  });
  writeFileSync(values.get("output"), `${JSON.stringify(manifest, null, 2)}\n`);
}

if (
  process.argv[1] &&
  import.meta.url === pathToFileURL(process.argv[1]).href
) {
  try {
    run(process.argv.slice(2));
  } catch (error) {
    console.error(error.message);
    process.exitCode = 1;
  }
}
