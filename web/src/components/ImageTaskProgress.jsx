import { useEffect, useState } from 'react'
import { useI18n } from '../i18n/useI18n.js'
import { imageStageElapsed, imageTaskStage } from './imageTaskPresentation.js'
import '../styles/image-task-progress.sass'

export default function ImageTaskProgress({ task }) {
  const { t } = useI18n()
  const [now, setNow] = useState(Date.now)
  const active = task?.status === 'running' && !task.cancel_requested_at
  useEffect(() => {
    setNow(Date.now())
    if (!active) return undefined
    // A display clock only: never requests or synthesizes business progress.
    const timer = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(timer)
  }, [active, task?.uuid, task?.stage_started_at])
  if (!task) return null
  const stage = imageTaskStage(task)
  const elapsed = imageStageElapsed(task, now)
  const busy = ['queued', 'running'].includes(task.status)
  return <span className="image-task-progress">
    {busy ? <progress aria-label={t('image.task.active')} /> : null}
    <span role="status">{t(`image.task.stage.${stage}`)}</span>
    {elapsed !== null ? <span className="image-task-progress__elapsed">{t('image.task.elapsed', { minutes: Math.floor(elapsed / 60), seconds: elapsed % 60 })}</span> : null}
  </span>
}
