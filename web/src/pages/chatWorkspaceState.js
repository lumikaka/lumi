export const ACTIVE_CHAT_STATUSES = new Set(['busy', 'waiting_for_input'])
export const ACTIVE_WORKFLOW_STATUSES = new Set(['queued', 'running'])

export const PROJECT_AGENT_EVENTS = [
  'chat:turn_queued', 'chat:run_status', 'chat:tool_call', 'chat:tool_result',
  'chat:follow_up_changed', 'chat:steering_queued', 'chat:turn_cancelled',
  'chat:user_input_requested', 'chat:user_input_answered', 'chat:user_input_cancelled',
  'workflow:queued', 'workflow:step_changed', 'workflow:failed', 'workflow:cancelled',
  'story:chapter_changed', 'premise:changed', 'premise:source_created',
  'premise:setting_image_changed', 'premise:asset_changed',
  'comic:section_changed', 'comic:sections_reordered', 'comic:snapshot_restored',
]

export function workflowControls(workflow) {
  return {
    canCancel: Boolean(workflow && ACTIVE_WORKFLOW_STATUSES.has(workflow.status)),
    canRetry: Boolean(workflow && ['failed', 'interrupted', 'cancelled'].includes(workflow.status)),
  }
}

export function agentQueryKeysForEvent(projectUuid, payload = {}) {
  const keys = []
  if (payload.thread_uuid) {
    keys.push(['chat-threads', projectUuid])
    for (const name of ['chat-items', 'chat-turns', 'chat-follow-ups', 'chat-input-requests', 'chat-events']) {
      keys.push([name, projectUuid, payload.thread_uuid])
    }
    keys.push(['chat-thread', projectUuid, payload.thread_uuid])
  }
  if (payload.workflow_uuid) {
    keys.push(['workflows', projectUuid])
    keys.push(['workflow', projectUuid, payload.workflow_uuid])
    keys.push(['workflow-runs', projectUuid, payload.workflow_uuid])
    keys.push(['workflow-events', projectUuid, payload.workflow_uuid])
    keys.push(['workflow-llm-logs', projectUuid, payload.workflow_uuid])
  }
  return keys
}

export function shouldPollAgentState(threads = [], workflows = []) {
  return threads.some((item) => ACTIVE_CHAT_STATUSES.has(item.status)) || workflows.some((item) => ACTIVE_WORKFLOW_STATUSES.has(item.status))
}
