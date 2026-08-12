export const INHERIT_MODEL_VALUE = '__inherit__'

export function modelSelectionValue(selection) {
  if (!selection?.provider_uuid || !selection?.model) return INHERIT_MODEL_VALUE
  return JSON.stringify([selection.provider_uuid, selection.model])
}

export function parseModelSelection(value) {
  if (value === INHERIT_MODEL_VALUE) return null
  try {
    const [providerUuid, model] = JSON.parse(value)
    if (typeof providerUuid !== 'string' || !providerUuid || typeof model !== 'string' || !model) return null
    return { provider_uuid: providerUuid, model }
  } catch {
    return null
  }
}

export function modelOptionsForSetting(settings, setting) {
  if (!settings || !setting) return []
  const options = setting.kind === 'image' ? settings.options?.image_models || [] : settings.options?.text_models || []
  return options.filter((option) => option.ready)
}
