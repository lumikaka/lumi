import assert from 'node:assert/strict'
import test from 'node:test'

import { RESOURCES } from './messages/index.js'
import {
  formatCountForLocale,
  formatDateTimeForLocale,
  formatNumberForLocale,
  normalizeLocale,
  readLocalePreference,
  translateMessage,
  writeLocalePreference,
} from './runtime.js'

test('normalizes supported language variants and falls back to Simplified Chinese', () => {
  assert.equal(normalizeLocale('en-GB'), 'en')
  assert.equal(normalizeLocale('zh_CN'), 'zh-Hans')
  assert.equal(normalizeLocale('fr'), 'zh-Hans')
  assert.equal(normalizeLocale(null), 'zh-Hans')
})

test('restores and persists normalized locale preferences', () => {
  const values = new Map([['lumi.locale', 'en-US']])
  const storage = { getItem: (key) => values.get(key), setItem: (key, value) => values.set(key, value) }
  assert.equal(readLocalePreference(storage), 'en')
  assert.equal(writeLocalePreference(storage, 'zh-CN'), 'zh-Hans')
  assert.equal(values.get('lumi.locale'), 'zh-Hans')
  assert.equal(readLocalePreference({ getItem: () => { throw new Error('blocked') } }), 'zh-Hans')
})

test('translates immediately from the selected resource without changing user values', () => {
  assert.equal(translateMessage(RESOURCES, 'zh-Hans', 'chat.reference.remove', { title: 'Moonlight Post Office' }), '移除引用“Moonlight Post Office”')
  assert.equal(translateMessage(RESOURCES, 'en', 'chat.reference.remove', { title: '月亮邮局' }), 'Remove reference “月亮邮局”')
  assert.equal(translateMessage(RESOURCES, 'en', 'missing.key'), 'missing.key')
})

test('formats dates, numbers, and English singular/plural counts with Intl', () => {
  const date = new Date('2026-08-10T12:34:00Z')
  assert.notEqual(
    formatDateTimeForLocale('zh-Hans', date, { dateStyle: 'medium', timeZone: 'UTC' }),
    formatDateTimeForLocale('en', date, { dateStyle: 'medium', timeZone: 'UTC' }),
  )
  assert.equal(formatNumberForLocale('en', 12345.6), new Intl.NumberFormat('en-US').format(12345.6))
  assert.equal(formatCountForLocale(RESOURCES, 'en', 'common.count.items', 1), '1 item')
  assert.equal(formatCountForLocale(RESOURCES, 'en', 'common.count.items', 2), '2 items')
  assert.equal(formatCountForLocale(RESOURCES, 'zh-Hans', 'common.count.items', 2), '2 项')
})
