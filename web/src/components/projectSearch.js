function normalizeSearchValue(value) {
  return String(value || '').normalize('NFKC').trim().toLocaleLowerCase()
}

const DIALOG_CHROME_HEIGHT = 152
const EMPTY_RESULTS_HEIGHT = 72
const RESULT_ROW_HEIGHT = 60
const MAX_VISIBLE_RESULT_ROWS = 8

export function filterProjectSearchResults(projects = [], query = '') {
  const tokens = normalizeSearchValue(query).split(/\s+/u).filter(Boolean)
  if (!tokens.length) return projects

  return projects.filter((project) => {
    const haystack = normalizeSearchValue([project?.name, project?.root_path].filter(Boolean).join('\n'))
    return tokens.every((token) => haystack.includes(token))
  })
}

export function projectSearchDialogHeight(projectCount = 0) {
  const count = Math.max(0, Number(projectCount) || 0)
  const resultsHeight = Math.max(
    EMPTY_RESULTS_HEIGHT,
    Math.min(count, MAX_VISIBLE_RESULT_ROWS) * RESULT_ROW_HEIGHT,
  )
  return DIALOG_CHROME_HEIGHT + resultsHeight
}
