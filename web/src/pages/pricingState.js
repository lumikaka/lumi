export function money(amount, currency = '') {
  if (amount == null) return '—'
  // Never convert monetary strings through Number (large totals lose precision).
  const value = String(amount).replace(/(\.\d*?[1-9])0+$/, '$1').replace(/\.0+$/, '.00')
  return `${currency} ${value}`.trim()
}
export const pricingMetrics = (mode) => mode === 'images'
  ? ['input_images', 'output_images']
  : mode === 'responses'
    ? ['input_tokens', 'cached_input_tokens', 'output_tokens', 'tool_input_tokens', 'cached_tool_input_tokens', 'image_input_tokens', 'cached_image_input_tokens', 'image_output_tokens']
    : ['input_tokens', 'cached_input_tokens', 'output_tokens']
export const blankRates = (mode) => pricingMetrics(mode).map((metric) => ({ metric, price: '', input_from: 0, input_to: null, size: '', quality: '', resolution: '' }))
export function dateBoundary(date, end = false) {
  if (!date) return ''
  const value = new Date(`${date}T00:00:00`)
  if (end) value.setDate(value.getDate() + 1)
  return value.toISOString()
}
export function blankPrice(provider, kind = 'text') {
  const mode = kind === 'text' ? 'tokens' : 'images'
  return { provider_uuid: provider.uuid, provider_type: provider.provider_type, model: kind === 'text' ? provider.default_model : provider.default_image_model, region: provider.region || '', request_type: kind, mode, currency: 'CNY', rates: blankRates(mode) }
}

export function localDateInput(instant, inclusiveEnd = false) {
  if (!instant) return ''
  const date = new Date(new Date(instant).getTime() - (inclusiveEnd ? 1 : 0))
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`
}
