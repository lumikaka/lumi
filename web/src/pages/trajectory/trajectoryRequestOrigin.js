import { workflowDisplayTitle } from '../chatAreaPresentation.js'
import { formatTerminologyMessageKey } from '../pictureBookProfile.js'

export function trajectoryWorkflowTitle(workflow, t, pictureBook) {
  return workflowDisplayTitle(workflow, (key, values) => t(formatTerminologyMessageKey(pictureBook, key), values))
}

export function trajectoryRequestOrigin(source, t, pictureBook) {
  const origins = source?.workflow_origins || []
  const sourceLabel = source?.source_type ? t(`trajectory.source.${source.source_type}`) : ''
  if (!origins.length) return sourceLabel
  const workflow = t('trajectory.origin.workflow_name', { name: origins.map((origin) => trajectoryWorkflowTitle(origin, t, pictureBook)).join(' / ') })
  return source.source_type === 'workflow' ? workflow : t('trajectory.origin.via', { workflow, source: sourceLabel })
}
