import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'

import {
  aspectRatioMismatch,
  defaultPictureBookDraft,
  draftForPictureBookFormat,
  pictureBookDraftIsValid,
  pictureBookPayload,
  reducedRatioValue,
} from './pictureBookProfile.js'

test('picture-book creation defaults to classic landscape with minimal text disabled', () => {
  const draft = defaultPictureBookDraft()
  assert.deepEqual(pictureBookPayload(draft), {
    format: 'classic_picture_book',
    aspect_ratio: { mode: 'landscape' },
    large_image_minimal_text: false,
  })
})

test('picture-book payloads include only fields relevant to the selected format', () => {
  assert.deepEqual(pictureBookPayload(draftForPictureBookFormat('wordless_picture_book')), {
    format: 'wordless_picture_book',
    aspect_ratio: { mode: 'landscape' },
  })
  assert.deepEqual(pictureBookPayload(draftForPictureBookFormat('interactive_picture_book')), {
    format: 'interactive_picture_book',
    interaction_mode: 'find_it',
  })
  assert.deepEqual(pictureBookPayload(draftForPictureBookFormat('comic_story')), {
    format: 'comic_story',
    aspect_ratio: { mode: 'landscape' },
    comic_layout: 'page_comic',
  })
  assert.deepEqual(pictureBookPayload(draftForPictureBookFormat('vertical_strip')), {
    format: 'vertical_strip',
  })
})

test('every picture-book format card has a distinct semantic icon', () => {
  const source = readFileSync(new URL('../components/PictureBookProfileFields.jsx', import.meta.url), 'utf8')
  for (const [format, icon] of [
    ['classic_picture_book', 'BookOpenText'],
    ['wordless_picture_book', 'Image'],
    ['interactive_picture_book', 'MousePointerClick'],
    ['comic_story', 'PanelsTopLeft'],
    ['vertical_strip', 'GalleryVertical'],
  ]) {
    assert.match(source, new RegExp(`${format}: ${icon}`))
  }
  assert.match(source, /<FormatIcon className="picture-book-format-card__icon"[^>]+aria-hidden="true"/)
})

test('picture-book card hover keeps readable text and selected-state feedback', () => {
  const styles = readFileSync(new URL('../styles/projects.sass', import.meta.url), 'utf8')
  const cardStyles = styles.slice(styles.indexOf('.picture-book-format-cards'), styles.indexOf('.picture-book-options'))
  const hoverStart = cardStyles.indexOf('  button:hover,')
  const selectedHoverStart = cardStyles.indexOf('  button[aria-pressed="true"]:hover,')
  const hoverBlock = cardStyles.slice(hoverStart, cardStyles.indexOf('  button[aria-pressed="true"]', hoverStart))
  assert.match(hoverBlock, /color: \$color-text/)
  assert.match(hoverBlock, /> svg\n\s+color: \$color-accent/)
  assert.ok(selectedHoverStart > hoverStart)
})

test('picture-book choice hover overrides the global button background', () => {
  const styles = readFileSync(new URL('../styles/projects.sass', import.meta.url), 'utf8')
  const choiceStyles = styles.slice(styles.indexOf('.picture-book-choice-row'), styles.indexOf('.picture-book-custom-ratio'))
  const hoverStart = choiceStyles.indexOf('  button:hover,')
  const selectedStart = choiceStyles.indexOf('  button[aria-pressed="true"]', hoverStart)
  const selectedHoverStart = choiceStyles.indexOf('  button[aria-pressed="true"]:hover,')
  const hoverBlock = choiceStyles.slice(hoverStart, selectedStart)
  assert.match(hoverBlock, /background: \$color-surface-subtle/)
  assert.ok(selectedHoverStart > selectedStart)
})

test('custom ratios enforce integer bounds and the 1:3 through 3:1 range', () => {
  assert.equal(pictureBookDraftIsValid({ format: 'classic_picture_book', aspectMode: 'custom', customWidth: 100, customHeight: 100 }), true)
  assert.equal(pictureBookDraftIsValid({ format: 'classic_picture_book', aspectMode: 'custom', customWidth: 1, customHeight: 3 }), true)
  assert.equal(pictureBookDraftIsValid({ format: 'classic_picture_book', aspectMode: 'custom', customWidth: 3, customHeight: 1 }), true)
  assert.equal(pictureBookDraftIsValid({ format: 'classic_picture_book', aspectMode: 'custom', customWidth: 1, customHeight: 4 }), false)
  assert.equal(pictureBookDraftIsValid({ format: 'classic_picture_book', aspectMode: 'custom', customWidth: 2.5, customHeight: 3 }), false)
  assert.equal(pictureBookDraftIsValid({ format: 'classic_picture_book', aspectMode: 'custom', customWidth: 101, customHeight: 50 }), false)
})

test('ratio helpers preserve exact ratios and detect replacement mismatches without cropping', () => {
  const profile = { aspect_ratio: { mode: 'landscape', width: 4, height: 3 } }
  assert.equal(reducedRatioValue(1536, 1152), '4:3')
  assert.equal(aspectRatioMismatch(1536, 1152, profile), false)
  assert.equal(aspectRatioMismatch(1024, 1024, profile), true)
})

test('chapter preview keeps continuous vertical strips and adds single-page keyboard pagination', () => {
  const source = readFileSync(new URL('./ChapterComicPreviewPage.jsx', import.meta.url), 'utf8')
  assert.match(source, /verticalStrip \? \(/)
  assert.match(source, /chapter-preview__pager/)
  assert.match(source, /event\.key === 'ArrowLeft'/)
  assert.match(source, /event\.key === 'ArrowRight'/)
  assert.match(source, /<ImageRatioNotice pictureBook=\{pictureBook\}/)
})
