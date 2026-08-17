export function projectRowActions(project) {
	if (project.open) return ['enter', 'reveal', 'forget']
  if (project.available) return ['enter', 'reveal', 'relocate', 'forget']
  return ['relocate', 'forget']
}

export function projectRowPrimaryAction(project) {
	if (project.open || project.available) return 'enter'
  return null
}

export function createdProjectPath(projectUuid, { continueToConversation = false, threadUuid = '', workflowUuid = '' } = {}) {
  const base = `/projects/${encodeURIComponent(projectUuid)}/premise`
  const params = new URLSearchParams()
  if (threadUuid) params.set('chat_thread_uuid', threadUuid)
  if (workflowUuid) params.set('workflow_uuid', workflowUuid)
  if (!threadUuid && continueToConversation) {
    params.set('chat_scope', 'project')
    params.set('chat_new', '1')
  }
  const search = params.toString()
  return search ? `${base}?${search}` : base
}
