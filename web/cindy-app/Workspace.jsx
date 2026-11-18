import React, { useEffect, useMemo, useRef, useState, useSyncExternalStore } from 'react'
import { QueryClientProvider, useQuery } from '@tanstack/react-query'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom'
import { WorkspaceRuntime } from '../src/workspace/runtime.jsx'
import { createAppImage } from '../mcp-app/WorkspaceImage.jsx'
import { AppRealtime } from '../mcp-app/realtime.js'
import { bindReviewSync } from '../mcp-app/sync.js'
import AddressBar from '../mcp-app/AddressBar.jsx'
import WorkspaceActions from '../mcp-app/WorkspaceActions.jsx'
import { ModelContextAction } from '../mcp-app/model-context.js'
import { workspaceHistoryEntries, workspaceOrigin } from '../mcp-app/workspace.js'
import TextCandidates from '../src/workspace/TextCandidates.jsx'
import { SimpleHomePage, SimpleBooksPage, SimplePageView, SimpleSettingsPage, SimpleSettingDetailPage, SimpleStoryPage, SimpleLoading, SimpleError } from '../src/pages/SimpleProjectPages.jsx'

function UnavailableDownload({ children }) {
  const [message, setMessage] = useState('')
  return <span><button type="button" onClick={() => setMessage('请在当前 Cindy 任务对话中导出，并在任务文件列表打开。此页面未提交下载。')}>{children}</button>{message && <span role="status">{message}</span>}</span>
}
function Workspace({ session }) {
  const { launch, api, previews, detailPreviews, bridge, client } = session
  const location = useLocation()
  const modelContext = useMemo(() => new ModelContextAction(bridge), [session])
  const referenceState = useSyncExternalStore(modelContext.subscribe, modelContext.getSnapshot)
  const leaveMessage = useRef(null)
  const confirmLeave = () => !leaveMessage.current || window.confirm(leaveMessage.current)
  const [status, setStatus] = useState('正在连接实时提示…')
  const [connection, setConnection] = useState(0)
  const projectQuery = useQuery({ queryKey: ['story-project', launch.project_uuid], queryFn: () => api.getStoryProject(launch.project_uuid) })
  const runtime = useMemo(() => ({ api, leaveMessageRef: leaveMessage, referenceToAgent: reference => modelContext.select({ ...reference, project_uuid: launch.project_uuid }),
    DownloadLink: UnavailableDownload, Image: createAppImage(previews, launch.project_uuid, detailPreviews),
    loadImage: async (page, { large, signal, attempt }) => {
      const uuid = page.current_image?.asset?.uuid || page.asset_uuid, edge = large ? 1024 : 384
      if (attempt) previews.evict(launch.project_uuid, uuid, edge)
      return { src: (await previews.load(launch.project_uuid, uuid, edge, signal)).data_uri }
    }, capabilities: { fileUpload: false, download: false, generation: false, chat: false },
  }), [session])
  useEffect(() => bindReviewSync({ socket: new AppRealtime(bridge, launch.project_uuid), project: launch.project_uuid, queryClient: client, onStatus: setStatus, workspace: true }), [session, connection])
  const base = `/projects/${launch.project_uuid}`
  const project = projectQuery.data
  const url = workspaceOrigin(project, launch) + (location.pathname.endsWith('/candidates') ? new URL(launch.open_url).pathname : location.pathname + location.search)
  const pageEditor = <SimplePageView key={location.pathname + location.search} project={project} projectUuid={launch.project_uuid} initialCandidatesOpen={new URLSearchParams(location.search).get('panel') === 'candidates'} />
  return <WorkspaceRuntime.Provider value={runtime}><div className="cindy-workspace mcp-workspace">
    <AddressBar confirmLeave={confirmLeave}><WorkspaceActions base={base} status={status} bridge={bridge} url={url} onReconnect={() => setConnection(value => value + 1)} confirmLeave={confirmLeave} /></AddressBar>
    {referenceState.message && <p className="cindy-workspace__notice" role="status" aria-live="polite" aria-busy={referenceState.pending}>{referenceState.message}</p>}
    {launch.permissions?.can_write !== true && <p className="cindy-workspace__notice" role="status">当前连接只有读取权限，提交操作不会写入。</p>}
    <section className="simple-project-body cindy-workspace__body mcp-workspace__body">
      {projectQuery.error ? <SimpleError error={projectQuery.error} onRetry={() => projectQuery.refetch()} /> : !project ? <SimpleLoading message="正在读取作品…" /> : <Routes>
        <Route path={`${base}/`} element={<SimpleHomePage project={project} projectUuid={launch.project_uuid} projectQuery={projectQuery} />} />
        <Route path={`${base}/chapters`} element={<SimpleBooksPage projectUuid={launch.project_uuid} />} />
        <Route path={`${base}/chapters/:chapterUuid`} element={pageEditor} />
        <Route path={`${base}/chapters/:chapterUuid/sections/:sectionUuid`} element={pageEditor} />
        <Route path={`${base}/premise`} element={<SimpleSettingsPage projectUuid={launch.project_uuid} />} />
        <Route path={`${base}/premise/assets/:assetUuid`} element={<SimpleSettingDetailPage projectUuid={launch.project_uuid} />} />
        <Route path={`${base}/story`} element={<SimpleStoryPage project={project} projectUuid={launch.project_uuid} />} />
        <Route path={`${base}/candidates`} element={<TextCandidates candidates={launch.ui_arguments?.candidates || launch.request?.arguments?.candidates} />} />
        <Route path="*" element={<p className="cindy-workspace__notice">此页面需要尚未接入的宿主能力。请在当前 Cindy 任务对话中继续。</p>} />
      </Routes>}
    </section>
  </div></WorkspaceRuntime.Provider>
}
export function CindyWorkspace({ session }) {
  useEffect(() => {
    const close = () => { void session.close().catch(() => {}) }
    window.addEventListener('pagehide', close)
    return () => { window.removeEventListener('pagehide', close); close() }
  }, [session])
  return <QueryClientProvider client={session.client}><MemoryRouter key={session.key} initialEntries={workspaceHistoryEntries(session.launch)}><Workspace session={session} /></MemoryRouter></QueryClientProvider>
}
export function HostUnavailable() {
  return <main className="cindy-workspace__blocked"><h1>作品工作区暂不可连接</h1><p>当前 Cindy 的页面任务身份与读写授权尚未完成核验，工作区连接保持关闭。</p><p>已保存作品不受影响。可继续在当前任务对话中创作、审阅和导出，并通过任务文件列表打开交付文件。</p></main>
}
