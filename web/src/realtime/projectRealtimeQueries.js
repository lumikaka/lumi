const chatDetailKeys = ['chat-items', 'chat-turns', 'chat-follow-ups', 'chat-input-requests', 'chat-events']
const workflowDetailKeys = ['workflow', 'workflow-runs', 'workflow-events']
const comicKeys = ['comic-sections', 'comic-state', 'comic-storyboards', 'comic-images', 'comic-snapshots', 'comic-snapshot', 'comic-exports']
const premiseKeys = ['premise', 'premise-sources', 'premise-settings', 'premise-assets', 'premise-variants']

function isAssetMaintenanceKind(kind) {
  return typeof kind === 'string' && kind.startsWith('asset_')
}

export function uniqueQueryKeys(queryKeys) {
  const seen = new Set()
  return queryKeys.filter((queryKey) => {
    const encoded = JSON.stringify(queryKey)
    if (seen.has(encoded)) return false
    seen.add(encoded)
    return true
  })
}

export function projectRealtimeInvalidation(projectUuid, event, payload = {}) {
  if (!projectUuid || !event || event.startsWith('phx_')) return { queryKeys: [], invalidateAll: false }

  const queryKeys = []
  const add = (name, ...parts) => queryKeys.push([name, projectUuid, ...parts])
  const addChat = () => {
    add('chat-threads')
    if (payload.thread_uuid) {
      add('chat-thread', payload.thread_uuid)
      chatDetailKeys.forEach((name) => add(name, payload.thread_uuid))
    }
  }
  const addWorkflow = () => {
    add('workflows')
    if (payload.workflow_uuid) workflowDetailKeys.forEach((name) => add(name, payload.workflow_uuid))
  }
  const addComic = () => comicKeys.forEach((name) => add(name))
  const addPremise = () => premiseKeys.forEach((name) => add(name))

  let matched = true
  if (event.startsWith('chat:')) {
    addChat()
    if (payload.workflow_uuid) addWorkflow()
  } else if (event.startsWith('workflow:')) {
    addWorkflow()
    if (payload.thread_uuid) addChat()
  } else if (event.startsWith('task:')) {
    if (isAssetMaintenanceKind(payload.kind)) {
      add('asset-maintenance-tasks')
      add('asset-scans')
    } else if (payload.kind) {
      add('story-tasks')
    } else {
      add('story-tasks')
      add('asset-maintenance-tasks')
    }
  } else if (event.startsWith('story:')) {
    add('story-tasks')
    add('story-project')
    add('story-chapters')
    if (payload.chapter_uuid || payload.resource_uuid) {
      const chapterUuid = payload.chapter_uuid || payload.resource_uuid
      add('story-chapter', chapterUuid)
      add('story-chapter-history', chapterUuid)
    }
    if (event === 'story:profile_changed') {
      add('story-profile')
      add('story-profile-history')
    }
  } else if (event.startsWith('production_task:')) {
    add('production-tasks')
    if (payload.task_uuid) {
      add('production-task', payload.task_uuid)
      add('comic-export-operation', payload.task_uuid)
    }
    add('comic-exports')
  } else if (event.startsWith('production:')) {
    add('production-tasks')
    addComic()
    addPremise()
  } else if (event.startsWith('comic:')) {
    addComic()
    add('production-tasks')
  } else if (event.startsWith('premise:')) {
    addPremise()
    addComic()
    add('production-tasks')
    add('story-project')
  } else if (event.startsWith('asset/') || event.startsWith('upload/') || event.startsWith('thumbnail/')) {
    add('assets')
    add('asset-maintenance-tasks')
  } else if (event.startsWith('integrity_scan/')) {
    add('asset-scans')
    add('asset-maintenance-tasks')
    add('assets')
  } else if (event === 'llm_log:changed') {
    add('project-llm-logs')
    if (payload.log_uuid) add('project-llm-log', payload.log_uuid)
    else add('project-llm-log')
    add('workflow-llm-logs')
  } else if (event === 'project:model_settings_changed') {
    add('project-model-settings')
    add('project-image-generation-preflight')
  } else {
    matched = false
  }

  return { queryKeys: uniqueQueryKeys(queryKeys), invalidateAll: !matched }
}
