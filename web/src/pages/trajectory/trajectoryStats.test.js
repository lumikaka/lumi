import assert from 'node:assert/strict'
import test from 'node:test'

import { formatTrajectoryDuration, formatTrajectoryTokens, trajectoryStatsGroups } from './trajectoryStats.js'

const messages = {
  'trajectory.stats.not_recorded': '未记录',
  'trajectory.stats.turn.one': '{count} 个 Turn',
  'trajectory.stats.turn.other': '{count} 个 Turn',
  'trajectory.stats.request.one': '{count} 次 Request',
  'trajectory.stats.request.other': '{count} 次 Request',
  'trajectory.stats.llm': 'LLM {duration}',
  'trajectory.stats.tool': '工具调用 {duration}',
  'trajectory.stats.tool_execution': '工具执行 {duration}',
  'trajectory.stats.user_wait': '等待用户 {duration}',
  'trajectory.stats.ttft': '首 token 平均 {duration}',
  'trajectory.stats.throughput': '{throughput} tok/s',
  'trajectory.stats.throughput_unrecorded': '吞吐率未记录',
  'trajectory.stats.cache_hit': '缓存命中 {percent}',
  'trajectory.stats.tokens': '输入 {input} tok · 输出 {output} tok',
}

function t(key, values = {}) {
  return Object.entries(values).reduce((text, [name, value]) => text.replace(`{${name}}`, String(value)), messages[key])
}

test('trajectory stats format compact durations and token counts', () => {
  assert.equal(formatTrajectoryDuration(14_629), '14.6s')
  assert.equal(formatTrajectoryDuration(162_000), '2m42s')
  assert.equal(formatTrajectoryTokens(9_946), '9.9K')
  assert.equal(formatTrajectoryTokens(689), '689')
})

test('trajectory stats reproduce the compact reference grouping from recorded facts', () => {
  const groups = trajectoryStatsGroups({
    turn_count: 1,
    model_request_count: 4,
    tool_count: 3,
    llm_duration_ms: 14_629,
    tool_duration_ms: 17_758,
    tool_execution_duration_ms: 7_758,
    user_wait_duration_ms: 10_000,
    cache_hit_percent: 22,
    input_tokens: 9_946,
    output_tokens: 689,
  }, t)
  assert.equal(groups.join(' | '), '1 个 Turn · 4 次 Request | LLM 14.6s · 工具执行 7.8s · 等待用户 10s | 首 token 平均 未记录 · 吞吐率未记录 | 缓存命中 22% | 输入 9.9K tok · 输出 689 tok')
})

test('trajectory stats keep unavailable timing and usage explicit', () => {
  const groups = trajectoryStatsGroups({ turn_count: 2, model_request_count: 1, tool_count: 1 }, t)
  assert.equal(groups.join(' | '), '2 个 Turn · 1 次 Request | LLM 未记录 · 工具调用 未记录 | 首 token 平均 未记录 · 吞吐率未记录 | 缓存命中 未记录 | 输入 未记录 tok · 输出 未记录 tok')
})
