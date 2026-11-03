import React, { useMemo, useSyncExternalStore } from 'react'
import { LinkAction, downloadURL } from './link-action.js'

export function HostLink({ bridge, url, children = '在 Lumi Web 中打开 ↗', className, title }) {
  const action = useMemo(() => new LinkAction(bridge, url), [bridge, url])
  const state = useSyncExternalStore(action.subscribe, action.getSnapshot)
  return <span className="mcp-host-link">
    <span className="mcp-host-link__actions">
      <button type="button" className={className} title={title || '请求宿主在浏览器打开 Lumi Web 页面'} disabled={state.opening || !url} onClick={() => void action.open()}>{state.opening ? '正在请求打开…' : children}</button>
      {url && <button type="button" disabled={state.copying} onClick={() => void action.copy()}>{state.copying ? '正在复制…' : '复制链接'}</button>}
    </span>
    <span className="mcp-host-link__feedback" role="status" aria-live="polite">{state.openMessage}{state.copyMessage && <span>{state.copyMessage}</span>}</span>
    {state.manualCopy && <input className="mcp-host-link__url" aria-label="完整链接（可手动复制）" readOnly value={url} onFocus={event => event.target.select()}/>}
    {state.diagnostics && <details className="mcp-host-link__diagnostics"><summary>链接请求详情</summary><pre>{JSON.stringify(state.diagnostics, null, 2)}</pre></details>}
    {!url && <small role="alert">下载链接未通过路径校验，请在 Lumi Web 页面继续。</small>}
  </span>
}

export function HostDownloadLink({ bridge, origin, project, href, children, className, title }) {
  return <HostLink bridge={bridge} url={downloadURL(origin, href, project)} className={className} title={title || '请求宿主打开下载地址'}>{children}</HostLink>
}
