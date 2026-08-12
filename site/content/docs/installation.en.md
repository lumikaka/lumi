---
title: Install Lumi
description: Download and install the Apple Silicon macOS or Windows x64 desktop build and handle first-launch prompts safely.
translationKey: installation
slug: installation
docs_group: start
weight: 10
keywords: [download, install, macOS, Windows, SmartScreen, checksum]
---

## Choose the right package

Open [Lumi Releases](https://github.com/lumikaka/lumi/releases) and download the file for your operating system from the latest stable release:

| System | File | Notes |
|---|---|---|
| Apple Silicon macOS | `Lumi-macos-aarch64.app.zip` | For M1, M2, M3, M4, and later Apple-chip Macs |
| Windows x64 | `Lumi-windows-x64-setup.exe` | For 64-bit Windows 10 or 11 |

There is currently no desktop package for older Intel Macs. Do not treat the outer artifact downloaded from GitHub Actions as the application; the Release page provides the direct delivery files.

## Install on macOS

1. Open the ZIP to obtain `Lumi.app`.
2. Move `Lumi.app` to Applications, or keep it in another location you trust.
3. Open Lumi from Finder. The launcher starts the Lumi service on this Mac and opens the workspace in your default browser.
4. If macOS blocks the app because it is not notarized yet, verify that it came from this repository’s Release page and compare its SHA-256 with the file shipped alongside the release. Then open System Settings → Privacy & Security and choose “Open Anyway” only after confirming the source.

{{< callout type="warning" title="Do not share the access URL" >}}
“Copy Access URL” in the tray menu includes a temporary access token for the running instance. Treat the complete URL like a password and never post or send it publicly.
{{< /callout >}}

## Install on Windows

1. Run `Lumi-windows-x64-setup.exe`. It installs for the current user and normally does not require administrator access.
2. If Microsoft Defender SmartScreen says that Windows protected your PC, confirm that the file came from a Lumi Release and compare its hash with the release’s `.sha256` file.
3. Continue through “More info” only after you trust the source. Open Lumi from the Start menu when installation completes.
4. Lumi starts a local service and opens the workspace in your default browser.

The Windows installer is not Authenticode-signed yet, so a SmartScreen prompt does not by itself mean that the file is corrupted. It is also not a reason to skip source and hash verification.

## Confirm the installation

A successful first launch shows the “Connect an AI model service” page. Configure that service before creating a project.

Closing the browser tab may leave the desktop launcher running in the menu bar or system tray. Reopen Lumi from its tray menu; choose Quit to stop the local service as well.
