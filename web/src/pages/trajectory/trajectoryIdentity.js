export function trajectoryKey(sourceKind, sourceUuid) {
  const kind = String(sourceKind || 'unknown').trim() || 'unknown'
  const uuid = String(sourceUuid || '').trim()
  if (!uuid) throw new Error('trajectory identity requires a source UUID')
  return `${kind}:${uuid}`
}

export function trajectorySummaryKey(summaryKind, ownerUuid, memberKeys = []) {
  const kind = String(summaryKind || 'summary').trim() || 'summary'
  const owner = String(ownerUuid || 'thread').trim() || 'thread'
  const members = memberKeys.map((value) => String(value || '').trim()).filter(Boolean)
  return `summary:${kind}:${owner}:${members.join('|') || 'empty'}`
}

export function trajectorySourceUuid(value) {
  return String(value?.sourceUuid || value?.source_uuid || value?.uuid || '').trim()
}

export function isSameTrajectoryIdentity(left, right) {
  return Boolean(left?.key && right?.key && left.key === right.key)
}
