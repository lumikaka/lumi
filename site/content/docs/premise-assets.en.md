---
title: Build character and setting references
description: Create, upload, and manage characters, places, and props so later picture-book frames share a visual foundation.
translationKey: premise-assets
slug: premise-assets
docs_group: create
weight: 50
keywords: [premise, character, setting, place, prop, reference image, upload, batch generation]
---

The Premise workspace stores the shared visual foundation of the picture book. A premise item may be a character, place, prop, or other reference, with descriptive data, tags, and multiple image versions for later frame generation.

## Establish the overall visual style

Open Premise → AI batch generation and enter the project’s default visual style—for example, “children’s watercolor picture book, soft paper texture, warm backlight, simple outlines.” Describe traits that should remain stable instead of actions needed by only one frame.

## Use references when creating a project

When you create a project from Home with a one-sentence brief, you may attach one or more images. Every attached image is automatically used for visual generation in send order; there is no role, name, instruction, or participation setting to choose. Remove any unwanted image from the attachment strip before sending. After sending, Project Setup shows read-only thumbnails and an automatic-use notice rather than editable reference parameters.

After you finalize and start generation, all attached images are used for the premise setting image and become reusable premise items in the project. References affect visual setup by default. If an image should also change the plot, say so directly in the one-sentence story brief.

If a step reports a missing or damaged reference, or says the selected model does not support image input, correct that image or model configuration and retry the current step. You do not need to recreate the project, and Lumi will not silently continue with text only.

## Generate premise items in a batch

1. Start a batch and describe the main characters, places, key props, and visual traits that must stay consistent.
2. Importing `STORY.md` quickly adds story context but overwrites the current input. Save any custom requirements first.
3. Generate the premise overview image. Check character count, clothing, color, and setting against the story.
4. For a good overview, choose “Confirm and break down.” Lumi turns it into separate, searchable, reusable premise items.
5. Generate another overview candidate when needed. A new attempt does not automatically remove earlier candidates.

{{< product-demo kind="premise" >}}

## Upload existing reference art

If you already have character or setting designs, use “Upload premise items”:

1. Select, drop, or paste one or more images.
2. Enter the type, name, summary, and tags for each image.
3. Upload and wait for validation. Lumi verifies the real file type and image content before finalizing the project asset.

Upload only art you have the right to use. Avoid private information, external watermarks, or material with unclear licensing.

## Manage one premise item

- Open Details to change the title, summary, type, and tags.
- Upload a new image version or generate one with AI without deleting the older versions.
- Choose “Restore as current” in version history to make a specific version current for later work.
- Reference an item in ChatArea to discuss it or generate another version in context.

## Trash and permanent deletion

Restore premise items after moving them to trash. Permanent deletion applies only to trashed records that active work no longer uses. Images referenced by historical snapshots may remain to protect earlier project states.

{{< callout type="note" title="Consistency comes from deliberate references" >}}
Before generating a storyboard image, check each current premise version and choose the characters, places, and props actually relevant to that frame. Generation still works without references, but consistency may be lower.
{{< /callout >}}

## Next step

With the main characters and key places ready, turn the chapter prose into visual sections and write or generate a storyboard for each one.
