import assert from 'node:assert/strict'
import test from 'node:test'

import {
  DEFAULT_LOCALE,
  LANGUAGE_OPTIONS,
  LOCALE_STORAGE_KEY,
  MESSAGE_BUNDLES,
  RESOURCES,
  SUPPORTED_LOCALES,
} from './messages/index.js'
import { interpolationNames } from './runtime.js'

test('interface i18n defaults to persistent Simplified Chinese and supports English', () => {
  assert.equal(DEFAULT_LOCALE, 'zh-Hans')
  assert.deepEqual(SUPPORTED_LOCALES, ['zh-Hans', 'en'])
  assert.equal(LOCALE_STORAGE_KEY, 'lumi.locale')
  assert.deepEqual(LANGUAGE_OPTIONS.map((option) => option.value), ['zh-Hans', 'en'])
})

test('message bundles have unique, complete locale keys and matching interpolation variables', () => {
  const owners = new Map()
  for (const [bundleName, bundle] of MESSAGE_BUNDLES) {
    const zhKeys = Object.keys(bundle['zh-Hans']).sort()
    const enKeys = Object.keys(bundle.en).sort()
    assert.deepEqual(enKeys, zhKeys, `${bundleName} locale keys differ`)
    for (const key of zhKeys) {
      assert.equal(owners.has(key), false, `${key} is duplicated by ${owners.get(key)} and ${bundleName}`)
      owners.set(key, bundleName)
      assert.notEqual(bundle['zh-Hans'][key].trim(), '', `${key} has an empty zh-Hans translation`)
      assert.notEqual(bundle.en[key].trim(), '', `${key} has an empty English translation`)
      assert.deepEqual(
        interpolationNames(bundle.en[key]),
        interpolationNames(bundle['zh-Hans'][key]),
        `${key} interpolation variables differ`,
      )
    }
  }
  assert.deepEqual(Object.keys(RESOURCES.en).sort(), Object.keys(RESOURCES['zh-Hans']).sort())
})

test('plural message families provide both one and other branches', () => {
  for (const locale of SUPPORTED_LOCALES) {
    for (const key of Object.keys(RESOURCES[locale])) {
      if (!key.endsWith('.one')) continue
      assert.equal(Object.hasOwn(RESOURCES[locale], `${key.slice(0, -4)}.other`), true, `${locale} ${key} has no other branch`)
    }
  }
})

test('every language option points to a translated label', () => {
  for (const option of LANGUAGE_OPTIONS) {
    for (const locale of SUPPORTED_LOCALES) assert.equal(typeof RESOURCES[locale][option.labelKey], 'string')
  }
})
