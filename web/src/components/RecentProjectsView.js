import { createElement as h } from 'react'
import { useI18n } from '../i18n/useI18n.js'

export const projectStatusCopy = {
	open: 'projects.status.open',
  recent: 'projects.status.recent',
  available: 'projects.status.available',
  project_not_found: 'projects.status.project_not_found',
  project_permission_denied: 'projects.status.project_permission_denied',
  project_identity_mismatch: 'projects.status.project_identity_mismatch',
  project_format_too_new: 'projects.status.project_format_too_new',
  project_migration_failed: 'projects.status.project_migration_failed',
  project_locked: 'projects.status.project_locked',
  invalid_project: 'projects.status.invalid_project',
}

export const projectErrorCopy = {
  project_not_found: 'projects.error.project_not_found',
  project_permission_denied: 'projects.error.project_permission_denied',
  project_identity_mismatch: 'projects.error.project_identity_mismatch',
  project_format_too_new: 'projects.error.project_format_too_new',
  project_migration_failed: 'projects.error.project_migration_failed',
  project_locked: 'projects.error.project_locked',
  invalid_project: 'projects.error.invalid_project',
}

function statusText(t, item) {
  return t(projectStatusCopy[item.status] || 'projects.status.unavailable')
}

export default function RecentProjectsView({ items = [], loading = false, onOpen, onRelocate, onForget }) {
  const { t } = useI18n()
  if (loading) {
    return h('div', { className: 'project-state', 'aria-live': 'polite' },
      h('span', { className: 'project-state__pulse', 'aria-hidden': 'true' }),
      h('p', null, t('projects.loading.index')))
  }
  if (items.length === 0) {
    return h('div', { className: 'project-empty' },
      h('span', { 'aria-hidden': 'true' }, '✦'),
      h('h2', null, t('projects.empty.title')),
      h('p', null, t('projects.empty.body')))
  }
  return h('div', { className: 'recent-projects' }, ...items.map((item) => h('article', {
    className: `project-card project-card--${item.available ? 'available' : 'unavailable'}`,
    key: item.uuid,
  },
  h('div', { className: 'project-card__top' },
    h('span', { className: `project-status project-status--${item.status}` }, statusText(t, item)),
    h('code', { title: item.uuid }, item.uuid.slice(0, 8))),
  h('h3', null, item.name),
  h('p', { className: 'project-card__path', title: item.root_path }, item.root_path),
  projectErrorCopy[item.status] ? h('div', { className: 'project-card__detail' },
    h('p', null, t(projectErrorCopy[item.status])),
    item.status_detail ? h('details', null,
      h('summary', null, t('errors.details')),
      h('pre', { 'data-user-content': true }, item.status_detail)) : null) : null,
  h('div', { className: 'project-card__actions' },
	item.available ? h('button', { type: 'button', onClick: () => onOpen?.(item.uuid) }, t('projects.action.enter')) : null,
	!item.open ? h('button', { type: 'button', className: 'button-secondary', onClick: () => onRelocate?.(item.uuid) }, t('projects.action.relocate')) : null,
    h('button', { type: 'button', className: 'button-quiet', onClick: () => onForget?.(item.uuid) }, t('projects.action.forget'))))))
}
