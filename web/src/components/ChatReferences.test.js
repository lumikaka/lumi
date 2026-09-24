import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const source = readFileSync(new URL('./ChatReferences.jsx', import.meta.url), 'utf8')
const chatAreaSource = readFileSync(new URL('./ChatArea.jsx', import.meta.url), 'utf8')
const stylesSource = readFileSync(new URL('../styles/chat.sass', import.meta.url), 'utf8')
const messagesSource = readFileSync(new URL('../i18n/messages/chat.js', import.meta.url), 'utf8')

test('compact reference images open an accessible shared dialog preview', () => {
  assert.match(source, /import LumiDialog from '\.\/LumiDialog\.jsx'/)
  assert.match(source, /compact && previewImages[\s\S]*?<button className="chat-reference-chip__image"[\s\S]*?aria-haspopup="dialog"[\s\S]*?setImagePreview/)
  assert.equal((chatAreaSource.match(/<ReferenceStrip[^>]*compact previewImages/g) || []).length, 2)
  assert.match(source, /<LumiDialog className="chat-reference-preview-dialog"[\s\S]*?onClose=\{\(\) => setImagePreview\(null\)\}/)
  assert.match(source, /chat-reference-preview-dialog__media[\s\S]*?<img src=\{imagePreview\.imageUrl\} alt=\{imagePreview\.title\}/)
  assert.match(stylesSource, /\.lumi-dialog\.chat-reference-preview-dialog[\s\S]*?max-height: calc\(100vh - 32px\)/)
  assert.match(messagesSource, /'chat\.reference\.preview_image': \['查看引用图片“\{title\}”', 'Preview reference image “\{title\}”'\]/)
})
