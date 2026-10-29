export const projectQueryKeys = {
  recent: () => ['recent-projects'],
  openProjects: () => ['open-projects'],
  open: (projectUuid) => ['project-open', projectUuid],
  setup: (projectUuid) => ['project-setup', projectUuid],
  initialization: (projectUuid) => ['project-initialization', projectUuid],
}

export function isProjectBusinessQuery(query, projectUuid) {
  const key = query?.queryKey
  return Array.isArray(key) && key[1] === projectUuid && key[0] !== 'project-open'
}
