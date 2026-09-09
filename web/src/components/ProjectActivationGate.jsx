import { useEffect } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useParams } from 'react-router-dom'

import { ensureProjectOpen } from '../pages/projectActivation.js'
import { projectQueryKeys } from '../api/projectQueryKeys.js'
import { useI18n } from '../i18n/useI18n.js'
import { localizedErrorPresentation } from '../i18n/errorLocalization.js'
import AppPageShell from './AppPageShell.jsx'

export default function ProjectActivationGate({ children }) {
  const { t } = useI18n()
  const { projectUuid } = useParams()
  const queryClient = useQueryClient()
  const activationQuery = useQuery({
    queryKey: projectQueryKeys.open(projectUuid),
		queryFn: () => ensureProjectOpen(projectUuid),
    enabled: Boolean(projectUuid),
    retry: false,
    staleTime: 0,
    refetchOnMount: 'always',
  })

  useEffect(() => {
    if (!activationQuery.data || activationQuery.data.uuid !== projectUuid) return
    queryClient.setQueryData(projectQueryKeys.open(projectUuid), activationQuery.data)
    queryClient.invalidateQueries({ queryKey: projectQueryKeys.recent() })
  }, [activationQuery.data, projectUuid, queryClient])

  if (!activationQuery.data) {
    const presentation = activationQuery.isError
      ? localizedErrorPresentation(t, activationQuery.error, { titleKey: 'projects.error.enter_title' })
      : null
    return (
      <AppPageShell title={t('projects.workspace')}>
        {presentation ? (
          <section className="workspace-activation-error" role="alert">
            <h1>{presentation.title}</h1>
            <p>{presentation.message}</p>
            {presentation.code ? <small>{t('errors.diagnostic_code', { code: presentation.code })}</small> : null}
            <div>
              <button type="button" disabled={activationQuery.isFetching} onClick={() => activationQuery.refetch()}>{t(activationQuery.isFetching ? 'projects.loading.entering' : 'common.action.retry')}</button>
              <Link className="button-secondary" to="/">{t('projects.all')}</Link>
            </div>
          </section>
        ) : <p className="workspace-loading" role="status">{t('projects.loading.entering')}</p>}
      </AppPageShell>
    )
  }
  return <ProjectBoundary key={projectUuid}>{children}</ProjectBoundary>
}

function ProjectBoundary({ children }) {
  return children
}
