import assert from 'node:assert/strict'
import test from 'node:test'
import { blankPrice, dateBoundary, money, pricingMetrics } from './pricingState.js'
import { costFilter } from '../api/pricing.js'

test('money formatting retains nano precision and values larger than safe integers', () => {
  assert.equal(money('9007199.254740993', 'USD'), 'USD 9007199.254740993')
  assert.equal(money('0.000000001', 'CNY'), 'CNY 0.000000001')
  assert.equal(money('2.400000000', 'CNY'), 'CNY 2.4')
  assert.equal(money('0.000000000', 'CNY'), 'CNY 0.00')
  assert.equal(money(null, 'CNY'), '—')
})
test('inclusive date end uses next local midnight', () => {
  const from = new Date(dateBoundary('2026-09-05'))
  const to = new Date(dateBoundary('2026-09-05', true))
  assert.equal(from.getDate(), 5)
  assert.equal(to.getDate(), 6)
  assert.equal(to.getHours(), 0)
})
test('backfill and summary filters preserve the same resource constraints', () => {
  assert.deepEqual(costFilter({ providerUuid: 'provider', requestType: 'text', model: 'exact/model', keyword: '', from: '2026-09-05T00:00:00Z', scope: 'project' }), { provider_uuid: 'provider', request_type: 'text', model: 'exact/model', from: '2026-09-05T00:00:00Z', scope: 'project' })
})
test('new prices require explicit rates and correct billing units', () => {
  const form = blankPrice({ uuid: 'provider', provider_type: 'aliyun_bailian', region: 'cn-beijing', default_image_model: 'qwen-image-3.0' }, 'image')
  assert.equal(form.region, 'cn-beijing')
  assert.equal(form.model, 'qwen-image-3.0')
  assert.deepEqual(form.rates.map((rate) => rate.metric), ['input_images', 'output_images'])
  assert.ok(form.rates.every((rate) => rate.price === ''))
  assert.equal(pricingMetrics('responses').length, 8)
})
