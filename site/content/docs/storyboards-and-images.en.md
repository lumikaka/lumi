---
title: Make storyboards and picture-book frames
description: Break a chapter into editable visual beats, select premise references, generate image candidates, and manage current frames and snapshots.
translationKey: storyboards-and-images
slug: storyboards-and-images
docs_group: create
weight: 60
keywords: [storyboard, comic, section, image generation, candidate, reference, snapshot, restore]
---

The Comic workspace breaks a chapter into visual sections. Each section has a current storyboard and current image while preserving candidate versions. You can remake one illustration without regenerating the whole chapter.

## Generate a storyboard from a chapter

1. Open the target chapter and choose comic-storyboard generation.
2. Describe the desired frame count, pacing, dialogue density, and emotional focus.
3. Set the maximum section count. The current range is 1–24 and the default is 6. More frames are not automatically better; picture books need room for page turns and repeated reading.
4. Submit and wait. The task uses the chapter, prompt, and model settings frozen when it was created; a retry does not silently switch inputs.

If the project has no usable premise items, Lumi shows a warning. Storyboard and image generation remain available, but characters, props, and places may be less consistent.

## Edit each visual section

Review every generated section:

- Does this frame advance the story instead of repeating the previous one?
- Which characters, places, and props truly need to appear?
- Are framing, direction of movement, and emotion clear?
- Does dialogue belong in the illustration, or should it become narration?

Adjust the title, order, storyboard text, action, and dialogue, then save another storyboard version.

{{< product-demo kind="storyboard" >}}

## Generate image candidates

1. Choose one section, or select several sections in the comic list for batch generation.
2. Review the suggested premise references. Keep only the characters, places, and props relevant to the frame so that references do not compete.
3. Check the image prompt and aspect ratio, then create the task.
4. Open image details when generation completes to inspect source, model, premise references, and file state.
5. Choose “Make current” for a good candidate. Earlier candidates remain available for comparison and restoration.

## Restore a chapter-comic snapshot

Changes to section structure and current images create restorable snapshots. Open the chapter snapshot list, inspect the saved section count, storyboards, and media availability, then restore the chosen version.

{{< callout type="warning" title="Restore changes the current chapter state" >}}
Confirm the version, section count, and available media in snapshot details. Lumi blocks restoration while related image work is running, preventing results from writing into the wrong state.
{{< /callout >}}

Restoration uses historical content to establish a new current state instead of physically overwriting all history. Review every current image again before opening the continuous preview.
