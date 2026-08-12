---
title: Manage local projects and backups
description: Understand the Lumi project folder, move or relocate it correctly, and build a safe complete-project backup routine.
translationKey: local-projects
slug: local-projects
docs_group: manage
weight: 80
keywords: [local project, folder, backup, move, project.sqlite, STORY.md, privacy]
---

Lumi is local first. Global application data holds provider configuration, interface preferences, and recent project locations. Each picture book keeps its long-lived content in its own project folder.

You can keep multiple projects open in different tabs. Each tab derives its project from the URL: opening B does not close A or interrupt A's generation work, and revisiting an old A link reopens it automatically. A project is released independently only after it has had no open page, request, or queued/running work for five minutes.

## Understand the project folder

A normal project looks approximately like this:

```text
Moonlight-Mail/
├── README.md
├── STORY.md
├── project.sqlite
├── assets/
└── .lumi/
    ├── cache/
    ├── thumbnails/
    ├── tmp/
    ├── quarantine/
    └── backups/
```

- `STORY.md` is a human-readable projection of the current story profile.
- `project.sqlite` is the source of truth for structured project data. Backing up only STORY.md is not enough.
- `assets/` stores finalized project images and related assets.
- `.lumi/` contains internal caches, thumbnails, temporary files, quarantine, and upgrade backups.

{{< product-demo kind="local" >}}

## Back up the complete project

1. Wait for saves and generation tasks to finish.
2. Quit Lumi so background work cannot write files during the copy. Merely switching to another project does not close the original project.
3. Copy the entire project root to a backup drive, controlled cloud storage, or a versioned backup tool.
4. Preserve the folder structure and hidden `.lumi/` directory instead of selecting only familiar-looking files.
5. Test backups periodically: copy one to a temporary location and use “Open existing” to confirm that Lumi can read it.

## Move or rename a project

Quit any Lumi instance using the project, then move or rename the complete root folder. On the next launch, its recent card may report an unavailable path. Use Relocate and choose the new location. Do not rename or separate internal files during the move.

Two Lumi processes cannot write the same project simultaneously. If Lumi reports that the project is locked, check whether another instance is running. Do not delete internal files to bypass the lock.

## Cache versus finalized assets

`.lumi/cache/` and thumbnails can normally be regenerated, but that does not mean the entire `.lumi/` directory is disposable. Do not manually clean internal project directories unless the interface or documentation explicitly instructs you to do so.

## What leaves the device

Lumi does not upload the complete project to a Lumi cloud. When you use an AI feature, the text, prompts, and reference images required for that generation are sent to your chosen model provider. The exact scope depends on the task and references you select.

{{< callout type="warning" title="Sync is not the same as backup" >}}
A sync service can propagate accidental deletion or corruption. Keep at least one independent backup with version history or on offline media for important projects.
{{< /callout >}}
