import { useI18n } from './useI18n.js'
import { localizedErrorPresentation } from './errorLocalization.js'

export default function LocalizedErrorMessage({
  error,
  className = 'workspace-notice workspace-notice--error',
  titleKey,
  messageKey,
  onDismiss,
  compact = false,
}) {
  const { t } = useI18n()
  if (!error) return null
  const presentation = localizedErrorPresentation(t, error, { titleKey, messageKey })
  return (
    <div className={className} role="alert">
      <div>
        <strong>{presentation.title}</strong>
        <span>{presentation.message}</span>
        {!compact && (presentation.code || presentation.status) ? (
          <small>
            {presentation.code ? t('errors.diagnostic_code', { code: presentation.code }) : null}
            {presentation.code && presentation.status ? ' · ' : null}
            {presentation.status ? t('errors.diagnostic_status', { status: presentation.status }) : null}
          </small>
        ) : null}
        {!compact && presentation.diagnostic ? (
          <details>
            <summary>{t('errors.details')}</summary>
            <pre data-user-content>{presentation.diagnostic}</pre>
          </details>
        ) : null}
      </div>
      {onDismiss ? <button type="button" className="button-quiet" onClick={onDismiss} aria-label={t('errors.dismiss')}>{t('common.action.close')}</button> : null}
    </div>
  )
}
