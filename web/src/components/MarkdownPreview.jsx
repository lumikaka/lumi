import { Fragment } from 'react'
import { useI18n } from '../i18n/useI18n.js'

export default function MarkdownPreview({ value = '', empty }) {
  const { t } = useI18n()
  const lines = value.split(/\r?\n/)
  if (!value.trim()) return <p className="markdown-preview__empty">{empty || t('common.label.none')}</p>

  return (
    <div className="markdown-preview">
      {lines.map((line, index) => {
        const key = `${index}-${line.slice(0, 12)}`
        if (!line.trim()) return <div className="markdown-preview__space" key={key} />
        if (line.startsWith('### ')) return <h3 key={key}>{line.slice(4)}</h3>
        if (line.startsWith('## ')) return <h2 key={key}>{line.slice(3)}</h2>
        if (line.startsWith('# ')) return <h1 key={key}>{line.slice(2)}</h1>
        if (line.startsWith('> ')) return <blockquote key={key}>{line.slice(2)}</blockquote>
        if (/^[-*] /.test(line)) return <p className="markdown-preview__bullet" key={key}><span>•</span>{line.slice(2)}</p>
        return <Fragment key={key}><p>{line}</p></Fragment>
      })}
    </div>
  )
}
