---
title: Preview and export the picture book
description: Review a chapter as a sequence, understand export readiness, and safely export one chapter or the whole project as ZIP.
translationKey: preview-and-export
slug: preview-and-export
docs_group: create
weight: 70
keywords: [preview, export, ZIP, missing images, download, readiness]
---

Review the sequence as a reader before exporting. One attractive image does not guarantee a good chapter; movement, character appearance, color, and information density must work across adjacent frames.

## Preview the chapter continuously

1. Open the chapter’s Comic workspace and enter continuous preview.
2. Inspect the current image of every active section in order.
3. Look for sudden costume changes, time-of-day jumps, contradictory movement, or repetitive framing.
4. Return to the relevant section to change its storyboard, references, or current candidate, then preview again.

Preview uses the current image of each active section. A generated candidate does not replace the current frame until you make it current.

{{< product-demo kind="preview" >}}

## Understand export readiness

Lumi checks the current snapshot before creating an export:

- **Every active section has a ready image**: create a complete export directly.
- **Some active sections have no image**: the dialog shows how many are missing; create a partial export only after explicitly allowing missing images.
- **No image is available**: export is blocked until at least one current frame is ready.

## Export one chapter

1. Open the export dialog from the current chapter.
2. Confirm the scope, ZIP filename, snapshot version, exportable section count, and missing-image count.
3. If images are missing, return to complete them or explicitly confirm a partial export.
4. Create the task and wait for progress to complete. You may cancel it while active; retry the original task after failure or cancellation where the dialog offers that control.
5. Choose “Download ZIP” after completion. Lumi does not automatically start a download when the task finishes.

## Export the whole project

Create a project-scoped export from Overview → Exports. It includes active chapters and frames that satisfy the current snapshot. Check readiness for each chapter before allowing any missing images.

Export history is paginated and saved by the service. Download a completed historical item directly instead of regenerating the same snapshot.

{{< callout type="note" title="An export ZIP is not a project backup" >}}
The ZIP delivers picture-book frames. It does not include every database record, version, and internal state required for continued editing. Back up the complete project folder instead.
{{< /callout >}}

## Check the downloaded result

Extract the ZIP into a new folder. Verify filenames, image count, reading order, and that every image opens correctly. Do not extract it inside your only project folder, where delivery files could be confused with project assets.
