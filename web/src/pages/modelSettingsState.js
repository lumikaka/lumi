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

// Use the effective model when inheriting. Changing an image option pins
// that model as a project override, just like selecting it in the model menu.
function imageOptionSelection(settings, setting, capability) {
  if (setting?.kind !== 'image' || setting.override_status === 'invalid') return null
  const selection = setting.override || setting.effective
  if (!selection) return null
  const option = settings.options?.image_models?.find((item) => item.ready && item.provider_uuid === selection.provider_uuid && item.model === selection.model)
  return option?.[capability] ? selection : null
}

export function imageThinkingSelection(settings, setting) {
  return imageOptionSelection(settings, setting, 'supports_thinking')
}

export function imagePromptExtendSelection(settings, setting) {
  return imageOptionSelection(settings, setting, 'supports_prompt_extend')
}

export function imageThinkingEnabled(selection) {
  return Boolean(selection) && selection.prompt_extend !== false && selection.enable_thinking !== false
}
