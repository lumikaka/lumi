import { common } from './common.js'
import { projects } from './projects.js'
import { story } from './story.js'
import { premise } from './premise.js'
import { comic } from './comic.js'
import { chat } from './chat.js'
import { trajectory } from './trajectory.js'
import { settings } from './settings.js'
import { errors } from './errors.js'

export const DEFAULT_LOCALE = 'zh-Hans'
export const LOCALE_STORAGE_KEY = 'lumi.locale'
export const LANGUAGE_OPTIONS = Object.freeze([
  { value: 'zh-Hans', labelKey: 'common.language.zh_hans', nativeLabel: '简体中文' },
  { value: 'en', labelKey: 'common.language.en', nativeLabel: 'English' },
])

export const MESSAGE_BUNDLES = Object.freeze([
  ['common', common],
  ['projects', projects],
  ['story', story],
  ['premise', premise],
  ['comic', comic],
  ['chat', chat],
  ['trajectory', trajectory],
  ['settings', settings],
  ['errors', errors],
])

export const RESOURCES = Object.freeze({
  'zh-Hans': Object.freeze(Object.assign({}, ...MESSAGE_BUNDLES.map(([, bundle]) => bundle['zh-Hans']))),
  en: Object.freeze(Object.assign({}, ...MESSAGE_BUNDLES.map(([, bundle]) => bundle.en))),
})
export const SUPPORTED_LOCALES = Object.freeze(['zh-Hans', 'en'])
