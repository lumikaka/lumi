import { useEffect, useId, useMemo, useRef, useState } from 'react'
import { BookOpen, Search, X } from 'lucide-react'

import { useI18n } from '../i18n/useI18n.js'
import LumiDialog from './LumiDialog.jsx'
import { formatProjectEditTime } from './projectEditTime.js'
import { filterProjectSearchResults, projectSearchDialogHeight } from './projectSearch.js'

export default function ProjectSearchDialog({
  currentProjectUuid = '',
  id,
  loading = false,
  onClose,
  onSwitchProject,
  projects = [],
}) {
  const { locale, t } = useI18n()
  const titleId = useId()
  const inputRef = useRef(null)
  const [query, setQuery] = useState('')
  const results = useMemo(() => filterProjectSearchResults(projects, query), [projects, query])
  const dialogHeight = projectSearchDialogHeight(projects.length)

  useEffect(() => {
    inputRef.current?.focus()
  }, [])

  const switchProject = (project) => {
    if (!project.available) return
    onClose?.()
    onSwitchProject?.(project.uuid)
  }

  return (
    <LumiDialog
      className="project-search-dialog"
      id={id}
      style={{ '--project-search-dialog-height': `${dialogHeight}px` }}
      aria-labelledby={titleId}
      onClose={onClose}
      onKeyDown={(event) => {
        if (event.key !== 'Escape') return
        event.preventDefault()
        onClose?.()
      }}
    >
      <header className="project-search-dialog__header">
        <h2 id={titleId}>{t('common.action.search')}</h2>
        <button className="project-search-dialog__close" type="button" aria-label={t('common.action.close')} onClick={onClose}>
          <X size={16} aria-hidden="true" />
        </button>
      </header>

      <label className="project-search-dialog__field">
        <span className="project-search-dialog__label">{t('projects.index.search_label')}</span>
        <Search size={16} aria-hidden="true" />
        <input
          ref={inputRef}
          autoFocus
          type="search"
          value={query}
          placeholder={t('projects.search.placeholder')}
          onChange={(event) => setQuery(event.target.value)}
        />
      </label>

      <div className="project-search-dialog__results" aria-label={t('projects.title')}>
        {results.map((project) => {
          const edited = formatProjectEditTime(project.last_opened_at, locale, t) || t('projects.status.recent')
          const available = Boolean(project.available)
          const current = project.uuid === currentProjectUuid
          return (
            <button
              className="project-search-dialog__result"
              type="button"
              key={project.uuid}
              disabled={!available}
              aria-current={current ? 'true' : undefined}
              aria-label={t('projects.search.switch_label', { name: project.name })}
              title={!available ? t('projects.status.unavailable') : undefined}
              onClick={() => switchProject(project)}
            >
              <BookOpen size={16} aria-hidden="true" />
              <span>
                <strong data-no-i18n>{project.name}</strong>
                <small>{available ? t('projects.search.result_meta', { time: edited }) : t('projects.search.unavailable_meta')}</small>
              </span>
            </button>
          )
        })}

        {!loading && results.length === 0 ? (
          <p className="project-search-dialog__empty">{t(query.trim() ? 'projects.index.empty_search' : 'projects.index.empty')}</p>
        ) : null}
        {loading && projects.length === 0 ? <p className="project-search-dialog__empty" aria-live="polite">{t('projects.loading.index')}</p> : null}
      </div>
    </LumiDialog>
  )
}
