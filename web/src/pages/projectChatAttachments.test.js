import assert from 'node:assert/strict'
import test from 'node:test'

import {
  MAX_PROJECT_CHAT_IMAGE_REFERENCES,
  canProjectChatAttachImages,
  readyProjectChatUploadUUIDs,
  removeProjectChatAttachment,
  selectProjectChatClipboardImages,
  selectProjectChatImageFiles,
} from './projectChatAttachments.js'

test('chat image attachments are limited to the two image scenes', () => {
  assert.equal(canProjectChatAttachImages('premise_asset_generation'), true)
  assert.equal(canProjectChatAttachImages('asset_reference'), true)
  assert.equal(canProjectChatAttachImages('storyboard_reference'), false)
})

test('image selection preserves order, rejects non-images, and caps at four', () => {
  const result = selectProjectChatImageFiles([
    { name: 'one.png', type: 'image/png' },
    { name: 'notes.txt', type: 'text/plain' },
    { name: 'animated.gif', type: 'image/gif' },
    { name: 'two.png', type: 'image/png' },
  ], 3)
  assert.deepEqual(result.acceptedFiles.map((file) => file.name), ['one.png'])
  assert.equal(result.rejectedNonImages, 2)
  assert.equal(result.exceededLimit, true)
  assert.equal(MAX_PROJECT_CHAT_IMAGE_REFERENCES, 4)
})

test('clipboard selection, removal, and ready upload extraction are stable', () => {
  const image = { name: 'paste.png', type: 'image/png' }
  const selected = selectProjectChatClipboardImages({ files: [image] }, 0)
  assert.equal(selected.hasImages, true)
  assert.deepEqual(selected.acceptedFiles, [image])
  const attachments = [
    { localId: 'one', status: 'ready', upload: { uuid: 'upload-one' } },
    { localId: 'two', status: 'uploading' },
  ]
  assert.deepEqual(readyProjectChatUploadUUIDs(attachments), ['upload-one'])
  assert.deepEqual(removeProjectChatAttachment(attachments, 'one').map((item) => item.localId), ['two'])
})
