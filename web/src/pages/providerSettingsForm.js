// Empty password fields preserve encrypted credentials already on the server.
export function providerSettingsChanges(prefix, form, apiKey, imageApiKey = '') {
  const settings = Object.fromEntries(Object.entries(form).map(([key, value]) => [`${prefix}.${key}`, value]))
  if (apiKey.trim()) settings[`${prefix}.api_key`] = apiKey.trim()
  if (prefix === 'ai_providers.openai_relay' && imageApiKey.trim()) settings[`${prefix}.image_api_key`] = imageApiKey.trim()
  return settings
}

// Keep an incompatible model visibly unselected until the user chooses one.
export function changeRelayImageMode(form, mode, modelsByMode) {
  const models = modelsByMode?.[mode] || []
  return { ...form, image_api_mode: mode, default_image_model: models.includes(form.default_image_model) ? form.default_image_model : '' }
}
