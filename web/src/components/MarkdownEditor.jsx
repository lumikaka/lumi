import { forwardRef, useEffect, useImperativeHandle, useMemo, useRef } from 'react'
import { defaultKeymap, history, historyKeymap } from '@codemirror/commands'
import { markdown } from '@codemirror/lang-markdown'
import { defaultHighlightStyle, syntaxHighlighting } from '@codemirror/language'
import { openSearchPanel, search, searchKeymap } from '@codemirror/search'
import { Compartment, EditorState } from '@codemirror/state'
import { EditorView, keymap, placeholder } from '@codemirror/view'

import { useI18n } from '../i18n/useI18n.js'
import { codeMirrorSearchPhrases } from './codeMirrorSearchPhrases.js'

const MarkdownEditor = forwardRef(function MarkdownEditor(
  {
    value,
    onChange,
    disabled = false,
    enableSearch = false,
    placeholderText = '',
    ariaLabel = '',
    maxLength,
    className = 'markdown-editor',
  },
  forwardedRef,
) {
  const { t } = useI18n()
  const hostRef = useRef(null)
  const viewRef = useRef(null)
  const changeRef = useRef(onChange)
  const editableRef = useRef(new Compartment())
  const phrasesRef = useRef(new Compartment())
  const searchRef = useRef(new Compartment())
  const searchPhrases = useMemo(() => codeMirrorSearchPhrases(t), [t])

  useImperativeHandle(forwardedRef, () => ({
    openSearchPanel() {
      const view = viewRef.current
      if (!view || !enableSearch || view.state.readOnly) return false
      return openSearchPanel(view)
    },
    focus() {
      viewRef.current?.focus()
    },
  }), [enableSearch])

  useEffect(() => { changeRef.current = onChange }, [onChange])

  useEffect(() => {
    if (!hostRef.current) return undefined

    const extensions = [
      history(),
      markdown(),
      syntaxHighlighting(defaultHighlightStyle),
      placeholder(placeholderText),
      EditorView.lineWrapping,
      EditorView.contentAttributes.of({ 'aria-label': ariaLabel }),
      EditorView.updateListener.of((update) => {
        if (update.docChanged) changeRef.current?.(update.state.doc.toString())
      }),
      keymap.of([...defaultKeymap, ...historyKeymap]),
      editableRef.current.of([
        EditorView.editable.of(!disabled),
        EditorState.readOnly.of(disabled),
      ]),
      phrasesRef.current.of(EditorState.phrases.of(searchPhrases)),
      searchRef.current.of(enableSearch ? [search(), keymap.of(searchKeymap)] : []),
    ]

    if (Number.isFinite(maxLength) && maxLength > 0) {
      extensions.push(EditorState.changeFilter.of((transaction) => !transaction.docChanged || transaction.newDoc.length <= maxLength))
    }

    const view = new EditorView({
      parent: hostRef.current,
      state: EditorState.create({ doc: value || '', extensions }),
    })
    viewRef.current = view

    return () => {
      view.destroy()
      viewRef.current = null
    }
  }, [])

  useEffect(() => {
    const view = viewRef.current
    const nextValue = value || ''
    if (!view || view.state.doc.toString() === nextValue) return
    view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: nextValue } })
  }, [value])

  useEffect(() => {
    const view = viewRef.current
    if (!view) return
    view.dispatch({
      effects: editableRef.current.reconfigure([
        EditorView.editable.of(!disabled),
        EditorState.readOnly.of(disabled),
      ]),
    })
  }, [disabled])

  useEffect(() => {
    const view = viewRef.current
    if (!view) return
    view.dispatch({ effects: phrasesRef.current.reconfigure(EditorState.phrases.of(searchPhrases)) })
  }, [searchPhrases])

  useEffect(() => {
    const view = viewRef.current
    if (!view) return
    view.dispatch({ effects: searchRef.current.reconfigure(enableSearch ? [search(), keymap.of(searchKeymap)] : []) })
  }, [enableSearch])

  return <div className={className} ref={hostRef} data-disabled={disabled ? 'true' : 'false'} />
})

export default MarkdownEditor
