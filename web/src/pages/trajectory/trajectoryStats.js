function recordedNumber(value) {
  if (value == null || value === '') return null
  const number = Number(value)
  return Number.isFinite(number) && number >= 0 ? number : null
}

export function formatTrajectoryDuration(value, fallback = '—') {
  const milliseconds = recordedNumber(value)
  if (milliseconds == null) return fallback
  const seconds = milliseconds / 1000
  if (seconds < 60) return `${Math.round(seconds * 10) / 10}s`
  const wholeSeconds = Math.round(seconds)
  return `${Math.floor(wholeSeconds / 60)}m${wholeSeconds % 60}s`
}

export function formatTrajectoryTokens(value, fallback = '—') {
  const tokens = recordedNumber(value)
  if (tokens == null) return fallback
  if (tokens < 1000) return String(Math.round(tokens))
  const compact = (scaled) => scaled >= 100 ? String(Math.round(scaled)) : String(Math.round(scaled * 10) / 10)
  if (tokens < 1_000_000) return `${compact(tokens / 1000)}K`
  return `${compact(tokens / 1_000_000)}M`
}

export function formatTrajectoryThroughput(value, fallback = '—') {
  const throughput = recordedNumber(value)
  if (throughput == null) return fallback
  return throughput < 100 ? throughput.toFixed(1) : String(Math.round(throughput))
}

export function trajectoryStatsGroups(overview = {}, t) {
  const turnCount = recordedNumber(overview.turn_count) || 0
  const requestCount = recordedNumber(overview.model_request_count) || 0
  const toolCount = recordedNumber(overview.tool_count) || 0
  const notRecorded = t('trajectory.stats.not_recorded')
  const groups = []

  if (turnCount > 0 || requestCount > 0) {
    const turns = t(turnCount === 1 ? 'trajectory.stats.turn.one' : 'trajectory.stats.turn.other', { count: turnCount })
    const requests = t(requestCount === 1 ? 'trajectory.stats.request.one' : 'trajectory.stats.request.other', { count: requestCount })
    groups.push(`${turns} · ${requests}`)
  }
  if (requestCount > 0) {
    const durations = [t('trajectory.stats.llm', { duration: formatTrajectoryDuration(overview.llm_duration_ms, notRecorded) })]
    if (toolCount > 0) durations.push(t('trajectory.stats.tool', { duration: formatTrajectoryDuration(overview.tool_duration_ms, notRecorded) }))
    groups.push(durations.join(' · '))
    const throughput = recordedNumber(overview.output_tokens_per_second)
    groups.push([
      t('trajectory.stats.ttft', { duration: formatTrajectoryDuration(overview.average_ttft_ms, notRecorded) }),
      throughput == null
        ? t('trajectory.stats.throughput_unrecorded')
        : t('trajectory.stats.throughput', { throughput: formatTrajectoryThroughput(throughput, notRecorded) }),
    ].join(' · '))
    const cacheHit = recordedNumber(overview.cache_hit_percent)
    groups.push(t('trajectory.stats.cache_hit', { percent: cacheHit == null ? notRecorded : `${Math.round(cacheHit)}%` }))
    groups.push(t('trajectory.stats.tokens', {
      input: formatTrajectoryTokens(overview.input_tokens, notRecorded),
      output: formatTrajectoryTokens(overview.output_tokens, notRecorded),
    }))
  }
  return groups
}
