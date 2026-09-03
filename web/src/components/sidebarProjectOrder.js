export function mergeSidebarProjectOrder(previousOrder, projects) {
  const currentOrder = Array.isArray(previousOrder) ? previousOrder : []
  const incomingOrder = uniqueProjectUuids(projects)

  // An empty array is also used while the recent-project query is loading. Keep
  // the saved order until a non-empty response can reconcile it.
  if (incomingOrder.length === 0) return currentOrder

  const incomingUuids = new Set(incomingOrder)
  const nextOrder = currentOrder.filter((uuid) => incomingUuids.has(uuid))
  const includedUuids = new Set(nextOrder)

  for (const uuid of incomingOrder) {
    if (includedUuids.has(uuid)) continue
    nextOrder.push(uuid)
    includedUuids.add(uuid)
  }

  if (sameOrder(currentOrder, nextOrder)) return currentOrder
  return nextOrder
}

export function orderSidebarProjects(projects, projectOrder) {
  const projectsByUuid = new Map((projects || []).map((project) => [project.uuid, project]))
  return (projectOrder || []).map((uuid) => projectsByUuid.get(uuid)).filter(Boolean)
}

export function reorderSidebarProjectOrder(projectOrder, draggedUuid, targetUuid, placement = 'before') {
  if (!Array.isArray(projectOrder) || draggedUuid === targetUuid || !['before', 'after'].includes(placement)) return projectOrder
  if (!projectOrder.includes(draggedUuid) || !projectOrder.includes(targetUuid)) return projectOrder

  const nextOrder = projectOrder.filter((uuid) => uuid !== draggedUuid)
  const targetIndex = nextOrder.indexOf(targetUuid)
  nextOrder.splice(targetIndex + (placement === 'after' ? 1 : 0), 0, draggedUuid)

  if (sameOrder(projectOrder, nextOrder)) return projectOrder
  return nextOrder
}

function uniqueProjectUuids(projects) {
  const seen = new Set()
  const uuids = []
  for (const project of projects || []) {
    if (!project?.uuid || seen.has(project.uuid)) continue
    seen.add(project.uuid)
    uuids.push(project.uuid)
  }
  return uuids
}

function sameOrder(left, right) {
  return left.length === right.length && left.every((uuid, index) => uuid === right[index])
}
