import { useMutation, useQueryClient } from '@tanstack/react-query'

import { cancelWorkflow, retryWorkflow } from '../api/chat.js'
import { handoffProjectInitialization } from '../api/projects.js'
import { isProjectBusinessQuery } from '../api/projectQueryKeys.js'
import LocalizedErrorMessage from '../i18n/LocalizedErrorMessage.jsx'
import { useI18n } from '../i18n/useI18n.js'
import { projectInitializationControls } from '../pages/projectInitialization.js'

export default function ProjectInitializationGate({ projectUuid, query, onOpenChat }) {
  const { t } = useI18n()
  const queryClient = useQueryClient()
  const initialization = query.data
  const controls = projectInitializationControls(initialization)
  const mutation = useMutation({
    mutationFn: (action) => {
      if (action === 'handoff') return handoffProjectInitialization(projectUuid, initialization.workflow_uuid)
      return action === 'cancel' ? cancelWorkflow(projectUuid, initialization.workflow_uuid) : retryWorkflow(projectUuid, initialization.workflow_uuid)
    },
    onSettled: () => queryClient.invalidateQueries({ predicate: (item) => isProjectBusinessQuery(item, projectUuid) }),
  })
  const stopped = controls.canRetry
  return (
    <section className="project-initialization-gate" aria-live="polite">
      <p className="eyebrow">{t('projects.initialization.eyebrow')}</p>
      <h1>{t(!initialization ? 'projects.initialization.loading' : stopped ? 'projects.initialization.stopped_title' : 'projects.initialization.running_title')}</h1>
      {initialization ? <p>{t(stopped ? 'projects.initialization.stopped_body' : 'projects.initialization.running_body')}</p> : null}
      <LocalizedErrorMessage error={mutation.error || query.error} />
      <div className="project-initialization-gate__actions">
        {initialization?.thread_uuid ? <button type="button" className="button-secondary" onClick={onOpenChat}>{t('projects.initialization.open_chat')}</button> : null}
        {controls.canCancel ? <button type="button" className="button-secondary" disabled={mutation.isPending} onClick={() => mutation.mutate('cancel')}>{t('chat.workflow.cancel')}</button> : null}
        {controls.canRetry ? <button type="button" disabled={mutation.isPending} onClick={() => mutation.mutate('retry')}>{t('chat.workflow.retry')}</button> : null}
        {stopped ? <button type="button" className="button-secondary" disabled={mutation.isPending || !controls.canTakeOver} onClick={() => mutation.mutate('handoff')}>{t('projects.initialization.take_over')}</button> : null}
        {!initialization && query.isError ? <button type="button" disabled={query.isFetching} onClick={() => query.refetch()}>{t('common.action.retry')}</button> : null}
      </div>
      {stopped && !controls.canTakeOver ? <p role="status">{t('projects.initialization.finishing_tasks')}</p> : null}
      {initialization ? <small>{t(stopped ? 'projects.initialization.take_over_hint' : 'projects.initialization.other_projects_hint')}</small> : null}
    </section>
  )
}
