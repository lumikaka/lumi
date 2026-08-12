const MAIN_TABS = new Set(['storyboard', 'body', 'prompts'])
const PREVIEW_TABS = new Set(['current', 'reference', 'candidates'])
const CHAPTER_PROMPT_KEYS = new Set(['json_system', 'comic_storyboard', 'section_premise_selection', 'section_image'])
const ACTIVE_IMAGE_TASK_STATUSES = new Set(['queued', 'running'])

export function normalizedChapterTab(value) {
  return MAIN_TABS.has(value) ? value : 'storyboard'
}

export function normalizedPreviewTab(value) {
  return PREVIEW_TABS.has(value) ? value : 'current'
}

export function patchWorkbenchSearch(search, patch) {
  const next = new URLSearchParams(search)
  Object.entries(patch).forEach(([key, value]) => {
    if (value === '' || value === null || value === undefined) next.delete(key)
    else next.set(key, value)
  })
  return next
}

export function chapterWorkbenchPrompts(items) {
  return (items || []).filter((prompt) => CHAPTER_PROMPT_KEYS.has(prompt.prompt_key))
}

export function chapterPromptKey(definition) {
  return `${definition.prompt_group}/${definition.prompt_key}`
}

export function dirtyChapterPrompts(prompts, drafts) {
  return (prompts || []).filter((definition) => {
    const value = drafts?.[chapterPromptKey(definition)] ?? definition.effective_value ?? ''
    return value.trim() && value !== (definition.effective_value || '')
  })
}

export function promptSaveFailures(results) {
  return Object.fromEntries((results || []).filter((result) => result.error).map((result) => [result.key, result.error]))
}

export function sectionImageGenerationActive(tasks, sectionUuid) {
  return Boolean(sectionUuid && (tasks || []).some((task) => (
    task.kind === 'comic_image_generation'
    && task.resource_uuid === sectionUuid
    && ACTIVE_IMAGE_TASK_STATUSES.has(task.status)
  )))
}

export function timelineSelectableSectionUuids(sections, tasks) {
  return (sections || [])
    .filter((section) => section.current_storyboard && !sectionImageGenerationActive(tasks, section.uuid))
    .map((section) => section.uuid)
}

export function enterTimelineMultiSelect(currentSectionUuid, selectableUuids) {
  return new Set((selectableUuids || []).includes(currentSectionUuid) ? [currentSectionUuid] : [])
}

export function filterTimelineSelection(selectedUuids, selectableUuids) {
  const selectable = new Set(selectableUuids || [])
  return new Set([...(selectedUuids || [])].filter((uuid) => selectable.has(uuid)))
}

export function toggleTimelineSelection(selectedUuids, sectionUuid, selectableUuids) {
  const next = filterTimelineSelection(selectedUuids, selectableUuids)
  if (!(selectableUuids || []).includes(sectionUuid)) return next
  if (next.has(sectionUuid)) next.delete(sectionUuid)
  else next.add(sectionUuid)
  return next
}

export function timelineSelectionControls(selectedUuids, selectableUuids, pending = false) {
  const selection = filterTimelineSelection(selectedUuids, selectableUuids)
  const selectableCount = (selectableUuids || []).length
  const selectedCount = selection.size
  return {
    selection,
    selectedCount,
    selectableCount,
    selectAllDisabled: pending || selectableCount === 0 || selectedCount === selectableCount,
    clearDisabled: pending || selectedCount === 0,
    generateDisabled: pending || selectedCount === 0,
  }
}

export function timelineManageDisabledState({
  createPending = false,
  deletePending = false,
  reorderPending = false,
  imageGenerationActive = false,
  index = -1,
  total = 0,
} = {}) {
  const pending = createPending || deletePending || reorderPending
  return {
    pending,
    createDisabled: pending,
    doneDisabled: pending,
    dragDisabled: pending,
    deleteDisabled: pending || imageGenerationActive,
    moveBeforeDisabled: pending || index <= 0,
    moveAfterDisabled: pending || index < 0 || index >= total - 1,
  }
}

export function timelineSectionDropIntent({ timelineRect, sectionRects, clientX, clientY, draggingUuid }) {
  if (!timelineRect || clientY < timelineRect.top - 24 || clientY > timelineRect.bottom + 24) return null
  const targets = (sectionRects || []).filter((item) => (
    item.uuid
    && item.uuid !== draggingUuid
    && item.width > 0
    && item.height > 0
  ))
  if (!targets.length) return null

  const beforeTarget = targets.find((item) => clientX < item.left + item.width / 2)
  if (beforeTarget) return { targetUuid: beforeTarget.uuid, placement: 'before' }
  return { targetUuid: targets[targets.length - 1].uuid, placement: 'after' }
}

export function reorderedTimelineUuids(uuids, sectionUuid, targetUuid, placement) {
  if (!sectionUuid || !targetUuid || sectionUuid === targetUuid || !['before', 'after'].includes(placement)) return null
  const current = [...(uuids || [])]
  if (!current.includes(sectionUuid) || !current.includes(targetUuid)) return null
  const remaining = current.filter((uuid) => uuid !== sectionUuid)
  const targetIndex = remaining.indexOf(targetUuid)
  remaining.splice(targetIndex + (placement === 'after' ? 1 : 0), 0, sectionUuid)
  return remaining.every((uuid, index) => uuid === current[index]) ? null : remaining
}

export function timelineDragScrollDelta(timelineRect, clientX, edgeSize = 36, step = 22) {
  if (!timelineRect) return 0
  if (clientX < timelineRect.left + edgeSize) return -step
  if (clientX > timelineRect.right - edgeSize) return step
  return 0
}

export function timelineDragTransition(state, action) {
  if (action?.type === 'start') {
    return {
      sectionUuid: action.sectionUuid,
      targetUuid: null,
      placement: null,
      preview: action.preview || null,
    }
  }
  if (action?.type === 'move' && state) {
    return {
      ...state,
      targetUuid: action.targetUuid || null,
      placement: action.placement || null,
      preview: state.preview ? { ...state.preview, ...(action.preview || {}) } : null,
    }
  }
  if (action?.type === 'cancel' || action?.type === 'complete') return null
  return state || null
}
