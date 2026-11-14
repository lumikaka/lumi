import { useState } from 'react'
import { useI18n } from '../i18n/useI18n.js'
import { useWorkspaceRuntime } from './runtime.jsx'

export default function TextCandidates({ candidates }) {
  const { t } = useI18n()
  const { referenceToAgent } = useWorkspaceRuntime()
  const [selected, setSelected] = useState(null)
  const select = (candidate, index) => {
    setSelected(index)
    referenceToAgent?.({ resource_type: 'text_candidate', number: index + 1, candidate_count: candidates.length, title: candidate.title, text: candidate.text })
  }
  return <div className="simple-project-page">
    <h1>{t('workspace.text_candidates.title')}</h1>
    <p>{t(referenceToAgent ? 'workspace.text_candidates.context_body' : 'workspace.text_candidates.body')}</p>
    {!candidates?.length ? <p role="alert">{t('workspace.text_candidates.unavailable')}</p> : <div className="simple-candidate-grid">
      {candidates.map((candidate, index) => <article key={index}>
        <button type="button" aria-pressed={selected === index} onClick={() => select(candidate, index)}><strong>{index + 1}. {candidate.title}</strong></button>
        <p className="workspace-candidate-text">{candidate.text}</p>
      </article>)}
    </div>}
    {selected !== null && <p role="status">{t(referenceToAgent ? 'workspace.text_candidates.context_selection' : 'workspace.text_candidates.selection', { number: selected + 1 })}</p>}
  </div>
}
