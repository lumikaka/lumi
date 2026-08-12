import assert from 'node:assert/strict'
import test from 'node:test'

import {
  comicImageDimensions,
  comicImageFileSize,
  comicImageModelLabel,
  comicImageTitle,
} from './comicImagePresentation.js'

test('comic image presentation uses compact metadata labels', () => {
  const variant = {
    version_no: 4,
    generation: { provider_type: 'aliyun_bailian', model: 'qwen-image-3.0' },
    asset: { original_filename: 'generated-section.png', width: 768, height: 2304 },
  }
  assert.equal(comicImageTitle(variant), 'generated-section.png')
  assert.equal(comicImageDimensions(variant.asset), '768x2304')
  assert.equal(comicImageModelLabel(variant), 'aliyun_bailian/qwen-image-3.0')
  assert.equal(comicImageModelLabel({ generation: { provider_type: 'openai', model: 'openai/gpt-image-2' } }), 'openai/gpt-image-2')
})

test('comic image file sizes use binary units', () => {
  const formatNumber = (value, options) => new Intl.NumberFormat('en', options).format(value)
  assert.equal(comicImageFileSize(0, formatNumber), '0 B')
  assert.equal(comicImageFileSize(1024, formatNumber), '1 KiB')
  assert.equal(comicImageFileSize(2.45 * 1024 * 1024, formatNumber), '2.5 MiB')
  assert.equal(comicImageFileSize(undefined, formatNumber), '')
})
