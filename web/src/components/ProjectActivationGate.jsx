import { useEffect } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useParams } from 'react-router-dom'

import { ensureProjectOpen } from '../pages/projectActivation.js'
import { projectQueryKeys } from '../api/projectQueryKeys.js'
import { useI18n } from '../i18n/useI18n.js'
import { localizedErrorPresentation } from '../i18n/errorLocalization.js'

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

  if (activationQuery.isError && !activationQuery.data) {
    const error = activationQuery.error
    const presentation = localizedErrorPresentation(t, error, { titleKey: 'projects.error.enter_title' })
    return (
      <main className="workspace-loading workspace-activation-error" role="alert">
        <strong>{presentation.title}</strong>
        <span>{presentation.message}</span>
        {presentation.code ? <small>{t('errors.diagnostic_code', { code: presentation.code })}</small> : null}
        <div>
          <button type="button" onClick={() => activationQuery.refetch()}>{t('common.action.retry')}</button>
          <Link className="button-link" to="/">{t('projects.all')}</Link>
        </div>
      </main>
    )
  }
  if (!activationQuery.data) {
    return <p className="workspace-loading">{t('projects.loading.entering')}</p>
  }
  return <ProjectBoundary key={projectUuid}>{children}</ProjectBoundary>
}

function ProjectBoundary({ children }) {
  return children
}
