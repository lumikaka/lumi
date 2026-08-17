import { useState } from 'react'
import { Link } from 'react-router-dom'

import { ChatComposer } from '../components/ChatArea.jsx'
import { CHAT_COMPOSER_ACCEPTANCE_STATES } from './chatComposerAcceptanceStates.js'
import { useI18n } from '../i18n/useI18n.js'

export default function ChatComposerAcceptancePage() {
  const { t } = useI18n()
  return (
    <main className="composer-acceptance">
      <header className="composer-acceptance__header">
        <div><p>{t('chat.acceptance.eyebrow')}</p><h1>{t('chat.acceptance.title')}</h1><span>{t('chat.acceptance.subtitle')}</span></div>
        <Link to="/">{t('chat.acceptance.back')}</Link>
      </header>
      <nav className="composer-acceptance__index" aria-label={t('chat.acceptance.index')}>
        {CHAT_COMPOSER_ACCEPTANCE_STATES.map((state, index) => <a href={`#composer-state-${state.id}`} key={state.id}>{String(index + 1).padStart(2, '0')} {t(state.titleKey)}</a>)}
      </nav>
      <section className="composer-acceptance__list">
        {CHAT_COMPOSER_ACCEPTANCE_STATES.map((state, index) => <ComposerStatePreview index={index} key={state.id} state={state} />)}
      </section>
    </main>
  )
}

function ComposerStatePreview({ index, state }) {
  const { t } = useI18n()
  const [draft, setDraft] = useState(state.initialDraft)
  const [attachments, setAttachments] = useState(state.attachments || [])
  const [action, setAction] = useState(() => t('chat.acceptance.action.none'))
  const removeAttachment = (localId) => setAttachments((current) => current.filter((item) => item.localId !== localId))
  const retryAttachment = (localId) => {
    setAttachments((current) => current.map((item) => item.localId === localId ? { ...item, status: 'uploading' } : item))
    setAction(t('chat.acceptance.action.retry'))
  }
  const attachmentBlocked = state.attachmentBlocked && attachments.some((item) => item.status !== 'ready')

  return (
    <article className="composer-acceptance__state" id={`composer-state-${state.id}`} data-acceptance-state={state.id}>
      <div className="composer-acceptance__details">
        <p><span>{String(index + 1).padStart(2, '0')}</span>{t(state.groupKey)}</p>
        <h2>{t(state.titleKey)}</h2>
        <dl><div><dt>{t('chat.acceptance.trigger')}</dt><dd>{t(state.triggerKey)}</dd></div><div><dt>{t('chat.acceptance.pass')}</dt><dd>{t(state.expectationKey)}</dd></div></dl>
      </div>
      <div className="composer-acceptance__preview">
        <ChatComposer
          activeTurn={state.activeTurn || null}
          draft={draft}
          pending={Boolean(state.pending)}
          abortPending={Boolean(state.abortPending)}
          forceFocus={Boolean(state.forceFocus)}
          scene={state.scene || 'premise_asset_generation'}
          attachments={attachments}
          attachmentBlocked={attachmentBlocked}
          onDraftChange={setDraft}
          onSend={(mode) => setAction(t(mode === 'follow_up' ? 'chat.acceptance.action.queue' : mode === 'steering' ? 'chat.acceptance.action.steer' : 'chat.acceptance.action.send'))}
          onAbort={() => setAction(t('chat.acceptance.action.stop'))}
          onAddFiles={(files) => setAction(t('chat.acceptance.action.files', { count: files?.length || 0 }))}
          onRemoveAttachment={removeAttachment}
          onRetryAttachment={retryAttachment}
          onPaste={() => {}}
        />
        <p className="composer-acceptance__action" role="status">{action}</p>
      </div>
    </article>
  )
}
