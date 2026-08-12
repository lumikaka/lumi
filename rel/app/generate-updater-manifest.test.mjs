import assert from "node:assert/strict";
import test from "node:test";

import { generateUpdaterManifest } from "./generate-updater-manifest.mjs";

const input = {
  version: "1.2.3",
  repository: "hxgdzyuyi/lumi",
  pubDate: "2026-08-12T03:00:00Z",
  macosSignature: "mac-signature\n",
  windowsSignature: "windows-signature\n",
};

test("generates the complete stable updater manifest", () => {
  const manifest = generateUpdaterManifest(input);

  assert.equal(manifest.version, "1.2.3");
  assert.equal(manifest.notes, "Desktop builds for v1.2.3.");
  assert.deepEqual(Object.keys(manifest.platforms), [
    "darwin-aarch64",
    "windows-x86_64",
  ]);
  assert.equal(manifest.platforms["darwin-aarch64"].signature, "mac-signature");
  assert.equal(
    manifest.platforms["darwin-aarch64"].url,
    "https://github.com/lumikaka/lumi/releases/download/v1.2.3/Lumi-macos-aarch64.app.tar.gz",
  );
  assert.equal(
    manifest.platforms["windows-x86_64"].signature,
    "windows-signature",
  );
});

test("rejects prerelease versions for the stable channel", () => {
  assert.throws(
    () => generateUpdaterManifest({ ...input, version: "1.2.3-beta.1" }),
    /stable semantic version/,
  );
});

test("requires both platform signatures", () => {
  assert.throws(
    () => generateUpdaterManifest({ ...input, windowsSignature: "  " }),
    /both updater signatures/,
  );
});
