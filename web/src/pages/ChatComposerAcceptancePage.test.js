import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

import { CHAT_COMPOSER_ACCEPTANCE_STATES } from './chatComposerAcceptanceStates.js'

const pageSource = readFileSync(new URL('./ChatComposerAcceptancePage.jsx', import.meta.url), 'utf8')
const appSource = readFileSync(new URL('../App.jsx', import.meta.url), 'utf8')
const chatSource = readFileSync(new URL('../components/ChatArea.jsx', import.meta.url), 'utf8')
const chatStyles = readFileSync(new URL('../styles/chat.sass', import.meta.url), 'utf8')

test('composer acceptance enumerates every approved state exactly once', () => {
  assert.deepEqual(CHAT_COMPOSER_ACCEPTANCE_STATES.map((state) => state.id), [
    'idle', 'focused', 'draft', 'multiline', 'attachment_uploading', 'attachment_ready',
    'attachment_error', 'sending', 'running_stop', 'running_queue', 'waiting_input', 'stopping',
  ])
  assert.equal(new Set(CHAT_COMPOSER_ACCEPTANCE_STATES.map((state) => state.id)).size, 12)
})

test('acceptance page renders the production composer and has a direct route', () => {
  assert.match(pageSource, /import \{ ChatComposer \} from '\.\.\/components\/ChatArea\.jsx'/)
  assert.match(pageSource, /CHAT_COMPOSER_ACCEPTANCE_STATES\.map/)
  assert.match(pageSource, /<ChatComposer/)
  assert.match(appSource, /path="\/acceptance\/chat-composer"/)
})

test('production composer uses the Figma SVG icon set and geometry', () => {
  assert.match(chatSource, /attachment\.svg/)
  assert.match(chatSource, /attachment-file\.svg/)
  assert.match(chatSource, /attachment-remove\.svg/)
  assert.match(chatSource, /composer-stop\.svg/)
  assert.match(chatSource, /send\.svg/)
  assert.doesNotMatch(chatSource, /figma\/workspace\/[^'"\n]+\.png/)
  assert.match(chatStyles, /\.chat-composer[\s\S]*?margin: 20px[\s\S]*?padding: 16px[\s\S]*?border-radius: 12px/)
  assert.match(chatStyles, /\.chat-composer__send[\s\S]*?width: 28px[\s\S]*?height: 28px/)
  assert.match(chatStyles, /\.chat-composer__action-slot[\s\S]*?width: 28px[\s\S]*?min-width: 28px/)
  assert.doesNotMatch(chatSource, /chat-composer__send--text/)
  assert.doesNotMatch(chatSource, /chat-composer__status/)
})
