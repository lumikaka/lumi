export function testModelOptions(settings, providers, kind) {
  const configured = new Set((providers?.items || []).filter((provider) => provider.configured).map((provider) => provider.uuid))
  return (settings?.options?.[`${kind}_models`] || []).filter((option) => configured.has(option.provider_uuid))
}

export function testModelAvailable(options, selection) {
  return Boolean(selection && options.some((option) => option.provider_uuid === selection.provider_uuid && option.model === selection.model))
}

export function initialTestSelection(test, settings, providers, kind) {
  const saved = test?.[`${kind}_model`]
  if (saved?.provider_uuid && saved?.model) return { provider_uuid: saved.provider_uuid, model: saved.model }
  const defaults = settings?.settings?.[`project_${kind}`]?.effective
  if (!testModelAvailable(testModelOptions(settings, providers, kind), defaults)) return null
  return { provider_uuid: defaults.provider_uuid, model: defaults.model }
}

export function repeatTestSelection(test) {
  return Object.fromEntries(['text', 'image'].map((kind) => {
    const model = test[`${kind}_model`]
    return [`${kind}_model`, { provider_uuid: model.provider_uuid, model: model.model }]
  }))
}

export function modelTestPage(value) {
  const page = Number(value)
  return Number.isSafeInteger(page) && page > 0 && page <= 1000000 ? page : 1
}
