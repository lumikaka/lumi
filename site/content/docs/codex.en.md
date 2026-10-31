---
title: Create in Codex App
description: Install the Lumi plugin with one prompt, then turn an idea into a local picture book in Codex App without an API key.
translationKey: codex
slug: codex
docs_group: start
weight: 15
keywords: [Codex, plugin, installation, picture book, local, images]
---

The Lumi plugin brings stories, character references, page planning, artwork, review, and export into a Codex App chat. It uses your Codex allowance without an API key and saves projects locally. Text and reference images needed for generation are still sent to Codex.

The first version supports **Codex App on macOS only**, with Apple Silicon and Intel CLI builds. Codex TUI and IDE extensions are not supported. Installation becomes available with the first Lumi Release containing Codex assets. Real App installation and autonomous book creation still require manual acceptance; approval-free installation is not promised.

## Install with one prompt

Paste this into Codex App while signed in:

> Read https://raw.githubusercontent.com/lumikaka/lumi/main/docs/features/codex-plugins/install.md and follow it to install the Lumi picture-book plugin for macOS Codex App. Report the actual codex executable and version, the installation result, and how to start creating in a new chat.

Downloading binaries, writing the user installation directory and `~/.codex`, registering the marketplace, and enabling the plugin may require host approval. The number of approvals depends on your permissions. The installer does not request an API key or trust a project on your behalf.

**Open a new Codex App chat after installation.** Use a local folder for each book as the chat's working directory. After initialization, trust the project yourself and open a new chat to check write protection; creating a configuration file alone does not activate protection.

## Start with an idea

In the new chat, try:

> Use Lumi to make an eight-page landscape watercolor picture book for a five-year-old: a little fox helps a lost star return to the sky. Decide the details yourself, finish the whole book, and export a PDF plus a ZIP containing all the text.

You can request review after the story or after each image. Without a preference, the skill pauses before finalizing the book specifications and after the story, references, and a representative page are ready.

Generated images are imported immediately. Character and scene reference images help maintain consistency. Small aspect-ratio differences can be kept, cropped, or padded. The PDF uses page images; Markdown or ZIP preserves editable text.

## Resume, upgrade, or uninstall

If generation stops or your allowance runs out, open a new chat in the same folder and ask to continue. Lumi reads saved progress before filling gaps. Avoid editing the project's internal data files.

Run the latest installer again to upgrade, then open a new chat. Uninstallation preserves your books and Lumi application data. See the [installation guide and acceptance status](https://github.com/lumikaka/lumi/blob/main/docs/features/codex-plugins/install.md) for commands and troubleshooting.
