export const DEFAULT_VISIBLE_THREAD_COUNT = 5

export function conversationProject(projects = [], activeProjectUuid = '') {
  const available = projects.filter((project) => project?.uuid && (project.open || project.available))
  return available.find((project) => project.uuid === activeProjectUuid)
    || available.find((project) => project.open)
    || available[0]
    || null
}

export function newConversationPath(projects = [], activeProjectUuid = '') {
  const project = conversationProject(projects, activeProjectUuid)
  if (!project) return '/?create_project=1&continue=new_conversation'
  return `/projects/${encodeURIComponent(project.uuid)}/premise?chat_scope=project&chat_new=1`
}

export function orderedThreads(threads = [], pinnedThreadUuids = []) {
  const pinned = new Set(pinnedThreadUuids)
  return threads
    .map((thread, index) => ({ thread, index }))
    .sort((left, right) => Number(pinned.has(right.thread.uuid)) - Number(pinned.has(left.thread.uuid)) || left.index - right.index)
    .map(({ thread }) => thread)
}

export function orderedProjects(projects = [], pinnedProjectUuids = []) {
  const pinned = new Set(pinnedProjectUuids)
  return projects
    .map((project, index) => ({ project, index }))
    .sort((left, right) => Number(pinned.has(right.project.uuid)) - Number(pinned.has(left.project.uuid)) || left.index - right.index)
    .map(({ project }) => project)
}

export function matchingThreads(threads = [], search = '') {
  const needle = search.trim().toLocaleLowerCase()
  if (!needle) return threads
  return threads.filter((thread) => (thread.title || '').toLocaleLowerCase().includes(needle))
}

export function visibleThreads(threads = [], { projectCount = 1, expanded = false, searching = false } = {}) {
  if (projectCount === 1 || expanded || searching) return threads
  return threads.slice(0, DEFAULT_VISIBLE_THREAD_COUNT)
}

export function shouldShowThreadToggle(threads = [], { projectCount = 1, searching = false } = {}) {
  return projectCount > 1 && !searching && threads.length > DEFAULT_VISIBLE_THREAD_COUNT
}

export function providerConnectionState(providers = []) {
  if (!Array.isArray(providers) || providers.length === 0) return 'missing'
  if (providers.some((provider) => provider.active && provider.ready)) return 'ready'
  if (providers.some((provider) => provider.has_secret || provider.configured)) return 'needs_verification'
  return 'missing'
}
