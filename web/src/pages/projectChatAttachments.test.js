import assert from 'node:assert/strict'
import test from 'node:test'

import {
  MAX_PROJECT_CHAT_REFERENCES,
  appendProjectChatReference,
  consumeProjectChatReferenceQuery,
  readyProjectChatReferences,
  referenceKey,
  removeProjectChatAttachment,
  selectProjectChatClipboardImages,
  selectProjectChatImageFiles,
} from './projectChatAttachments.js'

test('entry Reference query parameters are consumed without removing the new-thread intent', () => {
  const current = new URLSearchParams('workspace_tab=assets&chat_new=1&chat_reference_type=premise_asset&chat_reference_uuid=asset-one&chat_reference_title=Hero')
  const next = consumeProjectChatReferenceQuery(current)

  assert.equal(next.get('workspace_tab'), 'assets')
  assert.equal(next.get('chat_new'), '1')
  assert.equal(next.has('chat_reference_type'), false)
  assert.equal(next.has('chat_reference_uuid'), false)
  assert.equal(next.has('chat_reference_title'), false)
  assert.equal(current.get('chat_reference_uuid'), 'asset-one')
})

test('image selection preserves order, rejects non-images, and shares the 16 Reference limit', () => {
  const result = selectProjectChatImageFiles([
    { name: 'one.png', type: 'image/png' },
    { name: 'notes.txt', type: 'text/plain' },
    { name: 'animated.gif', type: 'image/gif' },
    { name: 'two.png', type: 'image/png' },
  ], 15)
  assert.deepEqual(result.acceptedFiles.map((file) => file.name), ['one.png'])
  assert.equal(result.rejectedNonImages, 2)
  assert.equal(result.exceededLimit, true)
  assert.equal(MAX_PROJECT_CHAT_REFERENCES, 16)
})

test('clipboard selection, removal, and finalized Reference extraction are stable', () => {
  const image = { name: 'paste.png', type: 'image/png' }
  const selected = selectProjectChatClipboardImages({ files: [image] }, 0)
  assert.equal(selected.hasImages, true)
  assert.deepEqual(selected.acceptedFiles, [image])
  const attachments = [
    { localId: 'one', status: 'ready', resource_type: 'file', resource_uuid: 'file-one' },
    { localId: 'two', status: 'uploading' },
  ]
  assert.deepEqual(readyProjectChatReferences(attachments), [{ resource_type: 'file', resource_uuid: 'file-one' }])
  assert.deepEqual(removeProjectChatAttachment(attachments, 'one').map((item) => item.localId), ['two'])
})

test('domain References deduplicate by type and UUID while preserving order', () => {
  const asset = { resource_type: 'premise_asset', resource_uuid: 'asset-one', status: 'ready' }
  const section = { resource_type: 'comic_section', resource_uuid: 'section-one', status: 'ready' }
  const references = appendProjectChatReference(appendProjectChatReference([], asset), section)
  assert.deepEqual(references.map(referenceKey), ['premise_asset:asset-one', 'comic_section:section-one'])
  assert.equal(appendProjectChatReference(references, asset), references)
})
