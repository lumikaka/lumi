---
title: Troubleshooting
description: Resolve common launch, provider, project, background-task, image, and export problems.
translationKey: troubleshooting
slug: troubleshooting
docs_group: manage
weight: 90
keywords: [troubleshooting, launch failure, connection failure, project lock, task failure, missing image, export]
---

Keep the visible error code and full message before troubleshooting, then change one condition at a time. Do not delete the project database, `.lumi/`, or asset files “to see if it helps.” That can turn a recoverable problem into data loss.

## Lumi did not open the workspace

1. Check whether Lumi is still present in the menu bar or system tray.
2. Choose Reopen from the tray menu instead of launching several instances repeatedly.
3. If the desktop program quit, start Lumi again. An old browser page no longer has a valid session and must be reopened by the new launcher.
4. If it still fails, open the log folder from the tray menu and record the most recent error. Remove access URLs, tokens, secrets, and personal paths before sharing anything.

## macOS or Windows blocked launch

Use only official files from GitHub Releases and verify SHA-256 first. Lack of macOS notarization or Windows Authenticode signing triggers system warnings; do not bypass one when you cannot verify the source. See [Install Lumi](../installation/) for the exact flow.

## Provider connection failed

- Confirm that the provider, region, workspace ID, or Account ID belong to the same account environment.
- Check whether the API key or token expired, was revoked, or lacks call permission.
- Make sure the current network can reach the provider and is not blocked by a proxy, firewall, or corporate policy.
- Save and rerun the connection check. Never paste a secret into logs or screenshots submitted for help.

## A project will not open

| Message | Action |
|---|---|
| Path unavailable | Use Relocate and choose the moved project’s complete root folder |
| No write permission | Move the project to a writable location or fix folder permissions |
| Path belongs to another project | Select the original project folder rather than substituting another one |
| Project locked | Close another Lumi instance using the project and retry |
| Lumi upgrade required | Install a newer Lumi version, keeping a complete project backup |

Before project migration, Lumi creates a consistent database backup in its internal backup folder. If migration fails, do not edit the database manually. Preserve the project and logs for diagnosis.

## Save reports a conflict

A chapter or `STORY.md` may have changed in another tool. Stop saving from the other editor, then use the interface to reload, import external content as a new version, or regenerate STORY.md from the database version. Copy any unsaved text to a temporary document before resolving the conflict.

## A background task failed or appears stuck

1. Open task status or the relevant workspace and read the error summary and current progress.
2. Confirm that the provider is still verified and check its console for quotas or request errors.
3. Use Retry when offered. A retry keeps the inputs and model configuration frozen when the task was created.
4. If you need different input, cancel the old task, change the prose, prompt, or references, and create a new task.

Refreshing is not retrying. The same task state returns after refresh; it does not create another model call.

## An image is missing or corrupted

Do not delete files directly from `assets/` or `.lumi/`. Open Premise → Assets, run a full integrity scan, and inspect missing, corrupt, pending, quarantined, or orphan findings. Unknown files are not automatically deleted. Use only the safe reconcile or restore operations offered by the interface.

## Export is unavailable

- No usable image: make at least one ready image current for an active section.
- Some images missing: complete those frames or explicitly allow a partial export in the dialog.
- Image work still active: wait for it to finish, then refresh readiness.
- A historical export is complete: download it directly instead of creating a duplicate task.

## Prepare a useful support report

When opening a [GitHub Issue](https://github.com/lumikaka/lumi/issues), include the Lumi version, operating system, reproduction steps, expected result, and error code. You may attach redacted screenshots or relevant log lines, but remove API keys, tokens, complete access URLs, personal directories, project UUIDs, and private story content.

{{< callout type="warning" title="Back up before attempting repairs" >}}
For a project that will not open, failed upgrade, damaged image, or filesystem problem, first copy the complete project folder. Never perform exploratory cleanup on the only copy.
{{< /callout >}}
