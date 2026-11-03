---
title: Install the Codex plugin
description: Copy one installation prompt to install the Lumi picture-book plugin in Codex App on macOS, without an API key.
translationKey: codex-installation
slug: codex-installation
docs_group: start
weight: 14
keywords: [Codex, plugin, installation, prompt, macOS]
---

The Lumi picture-book plugin lets you create and export books in a Codex App chat. Writing and image generation use your Codex allowance without an extra API key. Book projects are saved locally.

## Before you install

- Use **Codex App on macOS** and sign in. Apple Silicon and Intel builds are available; Codex TUI and IDE extensions are not supported.
- Make sure you can access GitHub. Installation downloads files from a Lumi release.

## Install with one prompt

Click “Copy prompt”, paste the full text below into a Codex App chat, and send it:

{{< copy-prompt >}}
Read https://raw.githubusercontent.com/lumikaka/lumi/main/docs/features/codex-plugins/install.md and follow it to install the Lumi picture-book plugin for macOS Codex App. Report the actual codex executable and version, the installation result, and how to start creating in a new chat.
{{< /copy-prompt >}}

Codex will read the guide and perform the installation. Downloading files, writing the user installation directory and `~/.codex`, registering the plugin marketplace, and enabling the plugin may require approval. Follow the prompts shown in the app.

When installation finishes, check the result reported by Codex. The release plugin is named `lumi@lumi-local`.

## Start creating after installation

1. Choose a local folder for your picture-book project.
2. **Open a new Codex App chat** with that folder as its working directory. A chat started before installation will not automatically load the new plugin.
3. Tell Codex what you want to create with Lumi. See {{< doc-link "codex" >}}Create in Codex App{{< /doc-link >}} for an example.

After the book is initialized, trust the project yourself and open a new chat to check write protection. The installer does not set project trust for you, and creating a configuration file alone does not activate protection.

## Updates and installation issues

To upgrade, run the latest installer again as described in the [full installation guide](https://github.com/lumikaka/lumi/blob/main/docs/features/codex-plugins/install.md), then open a new chat. The guide also covers uninstallation, which preserves your books and Lumi application data.

If a download fails, check your network connection and whether the latest Release includes the Codex plugin installation assets. The installer needs those published files; source code in the repository alone is not enough.
