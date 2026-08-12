---
title: Write the story and chapters
description: Maintain the story profile and chapter prose, import a draft, generate new versions, and resolve external STORY.md changes.
translationKey: story-and-chapters
slug: story-and-chapters
docs_group: create
weight: 40
keywords: [story, chapter, story profile, import, STORY.md, continuation, versions]
---

Lumi separates “what the whole book is about” from “what each chapter says.” The story profile carries characters, goals, conflict, and direction. Chapter prose is the text that later becomes a storyboard.

## Refine the story profile

1. Open Overview → Story profile.
2. Edit the current content or generate a story profile with AI. Adjust the requirements before generation and revise the result afterward.
3. Saving creates another content version rather than erasing all history. Restore an earlier candidate when you want to revisit an idea; the restore itself becomes a new version.
4. If chapters came first, Lumi can reconstruct a profile from existing chapters. Verify the characters and direction before treating it as current.

## Create and edit chapters

1. Open Chapters and create a chapter.
2. Enter its title and write in the body editor. Watch the save state and resolve a failure or conflict before leaving.
3. Switch to Preview to check the Markdown. Storyboarding benefits more from clear action, setting changes, and dialogue than from complex formatting.
4. For chapter generation, write explicit requirements and create a new version. Existing prose does not simply disappear when the task begins.

{{< product-demo kind="story" >}}

## Import an existing draft

Import TXT or Markdown from the chapter workspace. Before importing:

- Keep one clearly identifiable chapter or story segment per file.
- Use UTF-8 to avoid characters that the app cannot interpret.
- Keep the original file backed up until you verify the imported chapter structure.

Imported text joins the project’s version history and remains editable, previewable, and available for storyboarding.

## Plan several chapters or continue the story

Once the profile is stable, plan multiple chapters or generate the next one. State the chapter’s job in the overall story, required turn, approximate length, and desired child-friendly voice. Read the transition when the task completes before adopting or editing it.

## Resolve external STORY.md edits

`STORY.md` is a human-readable projection of the current story profile. You may inspect it in another editor, but Lumi does not silently overwrite an external change:

- Choose “Import as new version” to bring that edit into history.
- Choose “Regenerate from database version” to discard the external change and restore the current formal version.

{{< callout type="warning" title="Avoid editing continuously in two places" >}}
Saving from Lumi and an external editor at the same time can create conflicts. Use one editing surface at a time and confirm that the previous save completed before switching.
{{< /callout >}}

## Next step

When the direction and at least one chapter are stable, open Premise and turn recurring characters, places, and props into visual references.
