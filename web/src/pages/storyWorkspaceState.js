export const chapterCodePattern = /^vol([0-9]{2,})\.ch([0-9]{2,})$/

export function nextChapterCode(chapters = []) {
  const highest = chapters.reduce((current, chapter) => {
    const volume = Number(chapter?.volume_no) || 0
    const number = Number(chapter?.chapter_no) || 0
    if (volume > current.volume || (volume === current.volume && number > current.chapter)) {
      return { volume, chapter: number }
    }
    return current
  }, { volume: 1, chapter: 0 })
  const volume = Math.max(highest.volume, 1)
  const chapter = highest.chapter + 1
  return `vol${String(volume).padStart(2, '0')}.ch${String(chapter).padStart(2, '0')}`
}

export const CHAPTER_CREATION_ACTIONS = Object.freeze([
  { key: 'batch', labelKey: 'story.chapters.create.batch' },
  { key: 'next', labelKey: 'story.chapters.create.next' },
  { key: 'continue', labelKey: 'story.chapters.create.continue' },
  { key: 'manual', labelKey: 'story.chapters.create.manual' },
  { key: 'upload', labelKey: 'story.chapters.create.upload' },
])

export function sortChaptersByDirection(chapters = [], direction = 'asc') {
  const multiplier = direction === 'desc' ? -1 : 1
  return [...chapters].sort((left, right) => ((Number(left?.sort_order) || 0) - (Number(right?.sort_order) || 0)) * multiplier)
}

export function chapterContinuationContext(chapters = []) {
  const sourceChapter = sortChaptersByDirection(chapters, 'desc')[0] || null
  return {
    sourceChapter,
    targetChapterCode: nextChapterCode(chapters),
    hasCurrentStory: Boolean(sourceChapter?.current_story?.content?.trim()),
  }
}

export function chapterGenerationPlan({ mode, chapters = [], prompt = '', count = 1, storyMd = '' }) {
  const continuation = chapterContinuationContext(chapters)
  const requestedCount = mode === 'batch' ? Math.min(10, Math.max(1, Number(count) || 1)) : 1
  const start = parseChapterCode(continuation.targetChapterCode)
  const guidance = boundedContext(prompt, 30000)
  void storyMd

  return Array.from({ length: requestedCount }, (_, index) => {
    const chapterCode = formatChapterCode(start.volume, start.chapter + index)
    // i18n-exempt: this becomes model prompt content in the project's generation language, not interface copy.
    const fallback = mode === 'continue' ? '自然承接上一章并保持人物状态与叙事视角。' : '根据当前 STORY.md 创作下一章。'
    return { chapterCode, title: '', prompt: guidance || fallback }
  })
}

function parseChapterCode(value) {
  const match = String(value || '').match(chapterCodePattern)
  return { volume: Number(match?.[1]) || 1, chapter: Number(match?.[2]) || 1 }
}

function formatChapterCode(volume, chapter) {
  return `vol${String(volume).padStart(2, '0')}.ch${String(chapter).padStart(2, '0')}`
}

function boundedContext(value, limit) {
  const normalized = String(value || '').trim()
  if (normalized.length <= limit) return normalized
  // i18n-exempt: the truncation marker is model context content and follows the generation pipeline.
  return `${normalized.slice(0, limit)}\n\n[上下文已截断]`
}

export function isSupportedStoryFile(file) {
  return Boolean(file?.name && /\.(txt|md)$/i.test(file.name) && file.size <= 2 * 1024 * 1024)
}

export function saveStateForError(error) {
  if (['chapter_revision_conflict', 'story_profile_revision_conflict', 'story_md_conflict'].includes(error?.code)) return 'conflict'
  return 'failed'
}

export function storyConflictChoices(profile) {
  return profile?.projection_state === 'conflict'
    ? ['import_external', 'regenerate_database']
    : []
}

export function mayPermanentlyDelete(chapter, confirmed) {
  return Boolean(chapter?.trashed_at && confirmed)
}
