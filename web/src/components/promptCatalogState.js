const placeholderPattern = /\{\{([^{}]*)\}\}/g

export const promptIdentity = (definition) => `${definition.prompt_group}/${definition.prompt_key}`
export const normalizedPrompt = (value) => String(value || '').trim()

export function promptPlaceholders(value) {
  const found = new Set()
  for (const match of String(value || '').matchAll(placeholderPattern)) found.add(match[1])
  return [...found].sort()
}

export function promptIssues(definition, draft) {
  const allowed = new Set(definition.placeholders || [])
  const actual = promptPlaceholders(draft)
  return {
    missing: [...allowed].filter((placeholder) => !actual.includes(placeholder)),
    unknown: actual.filter((placeholder) => !allowed.has(placeholder)),
  }
}

export function promptServerValues(catalog) {
  return Object.fromEntries(catalog.map((definition) => [promptIdentity(definition), definition.effective_value || '']))
}

export function reconcilePromptDrafts(current, previousServerValues, catalog) {
  const next = { ...current }
  for (const definition of catalog) {
    const identity = promptIdentity(definition)
    const hadBaseline = Object.hasOwn(previousServerValues, identity)
    const dirty = hadBaseline && normalizedPrompt(current[identity]) !== normalizedPrompt(previousServerValues[identity])
    if (!dirty) next[identity] = definition.effective_value || ''
  }
  return next
}

export function promptUpdatePayload(definition, draft) {
  return {
    prompt_group: definition.prompt_group,
    prompt_key: definition.prompt_key,
    prompt: normalizedPrompt(draft),
    expected_current_version: definition.current_version?.version_no || 0,
  }
}

export function resetPromptGroupDrafts(current, definitions) {
  return {
    ...current,
    ...Object.fromEntries(definitions.map((definition) => [promptIdentity(definition), definition.default_value || ''])),
  }
}

export function applyOverallStylePresetDraft(current, preset, target) {
  return {
    ...current,
    [promptIdentity(target)]: current[promptIdentity(preset)] ?? preset.effective_value ?? '',
  }
}
