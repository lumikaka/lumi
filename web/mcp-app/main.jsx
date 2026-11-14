import React, { useEffect, useMemo, useRef, useState, useSyncExternalStore } from 'react'
import { createRoot } from 'react-dom/client'
import { QueryClient, QueryClientProvider, useQuery } from '@tanstack/react-query'
import { createPath, MemoryRouter, Route, Routes, useLocation, useNavigate } from 'react-router-dom'
import AddressBar from './AddressBar.jsx'
import WorkspaceActions from './WorkspaceActions.jsx'
import { ModelContextAction } from './model-context.js'
import { AppBridge } from './bridge.js'
import { HostLink, HostDownloadLink } from './HostLink.jsx'
import { AppRealtime } from './realtime.js'
import { bindReviewSync } from './sync.js'
import { PreviewCache } from './images.js'
import { createAppAPI, workspaceLaunch, workspaceOrigin } from './workspace.js'
import { createAppImage } from './WorkspaceImage.jsx'
import TextCandidates from '../src/workspace/TextCandidates.jsx'
import { WorkspaceRuntime } from '../src/workspace/runtime.jsx'
import { SimpleHomePage, SimpleBooksPage, SimplePageView, SimpleSettingsPage, SimpleSettingDetailPage, SimpleStoryPage, SimpleLoading, SimpleError } from '../src/pages/SimpleProjectPages.jsx'
import './workspace.scss'

const bridge = new AppBridge()
const modelContext = new ModelContextAction(bridge)
const previews = new PreviewCache((...args)=>bridge.call(...args))
const detailPreviews = new PreviewCache((...args)=>bridge.call(...args), { detail: true, concurrency: 1 })
const client = new QueryClient({defaultOptions:{queries:{retry:false,refetchOnWindowFocus:false,refetchOnReconnect:false,staleTime:Infinity,gcTime:300000}}})
function OpenLumi({url}) {
  return <HostLink bridge={bridge} url={url}/>
}
function Workspace({launch}) {
  const location = useLocation(), navigate = useNavigate()
  const referenceState = useSyncExternalStore(modelContext.subscribe, modelContext.getSnapshot)
  const [status,setStatus] = useState('正在连接实时提示…')
  const [connection,setConnection] = useState(0)
  const api = useMemo(()=>createAppAPI(bridge,launch.project_uuid,{canWrite:launch.permissions?.can_write !== false}),[launch.project_uuid,launch.permissions?.can_write])
  const projectQuery = useQuery({queryKey:['story-project',launch.project_uuid],queryFn:()=>api.getStoryProject(launch.project_uuid)})
  const project = projectQuery.data
  const origin = workspaceOrigin(project,launch)
  const leaveMessage = useRef(null)
  const confirmLeave = () => !leaveMessage.current || window.confirm(leaveMessage.current)
  const runtime = useMemo(() => ({
    leaveMessageRef: leaveMessage,
    DownloadLink: props => <HostDownloadLink {...props} bridge={bridge} origin={origin} project={launch.project_uuid} />,
    api,
    referenceToAgent: reference => modelContext.select({ ...reference, project_uuid: launch.project_uuid }),
    Image: createAppImage(previews, launch.project_uuid, detailPreviews),
    capabilities: { fileUpload: true, download: true, generation: false, chat: false },
  }), [api, launch.project_uuid, origin])
  const base = `/projects/${launch.project_uuid}`
  const pageEditor = <SimplePageView
    key={location.pathname + location.search}
    project={project}
    projectUuid={launch.project_uuid}
    initialCandidatesOpen={new URLSearchParams(location.search).get('panel') === 'candidates'}
  />
  const url = location.pathname.endsWith('/candidates') ? origin + new URL(launch.open_url).pathname : origin + location.pathname + location.search
  useEffect(()=>bindReviewSync({socket:new AppRealtime(bridge,launch.project_uuid),project:launch.project_uuid,queryClient:client,onStatus:setStatus,workspace:true}),[launch.project_uuid,connection])
  useEffect(() => {
    const candidates = launch.ui_arguments?.candidates || launch.request?.arguments?.candidates
    navigate(launch.path, {
      replace: createPath(location) === launch.path,
      state: launch.path === base + '/candidates' ? { candidates } : null,
    })
    void client.invalidateQueries({ predicate: query => query.queryKey?.[1] === launch.project_uuid, refetchType: 'active' })
  }, [launch.sequence])
  return <WorkspaceRuntime.Provider value={runtime}><div className="mcp-workspace">
    <AddressBar base={base} confirmLeave={confirmLeave}>
      <WorkspaceActions base={base} status={status} bridge={bridge} url={url}
        onReconnect={() => setConnection(n => n + 1)} confirmLeave={confirmLeave} />
    </AddressBar>
    {referenceState.message ? <p className="mcp-workspace__notice" role="status" aria-live="polite" aria-busy={referenceState.pending}>{referenceState.message}</p> : null}
    {new URLSearchParams(location.search).has('chat_reference_uuid') ? <p className="mcp-workspace__notice" role="status">聊天引用已准备好，请在 Lumi 中继续。<OpenLumi url={url}/></p> : null}
    {launch.permissions?.can_write === false ? <p className="mcp-workspace__notice" role="status">当前连接只有读取权限，提交操作不会写入。</p> : null}
    <section className="simple-project-body mcp-workspace__body">
      {projectQuery.error?<SimpleError error={projectQuery.error} onRetry={()=>projectQuery.refetch()}/>:!project?<SimpleLoading message="正在读取作品…"/>:<Routes>
        <Route path={`${base}/`} element={<SimpleHomePage project={project} projectUuid={launch.project_uuid} projectQuery={projectQuery}/>}/>
        <Route path={`${base}/chapters`} element={<SimpleBooksPage projectUuid={launch.project_uuid}/>}/>
        <Route path={`${base}/chapters/:chapterUuid`} element={pageEditor} />
        <Route path={`${base}/chapters/:chapterUuid/sections/:sectionUuid`} element={pageEditor} />
        <Route path={`${base}/premise`} element={<SimpleSettingsPage projectUuid={launch.project_uuid}/>}/>
        <Route path={`${base}/premise/assets/:assetUuid`} element={<SimpleSettingDetailPage projectUuid={launch.project_uuid}/>}/>
        <Route path={`${base}/story`} element={<SimpleStoryPage project={project} projectUuid={launch.project_uuid}/>}/>
        <Route path={`${base}/candidates`} element={<TextCandidates key={location.key} candidates={location.state?.candidates}/>}/>
        <Route path="*" element={<div className="simple-project-page"><p>此页面需要 Lumi 的完整运行环境。</p><OpenLumi url={url}/></div>}/>
      </Routes>}
    </section>
  </div></WorkspaceRuntime.Provider>
}
function App() {
  const [launch,setLaunch] = useState(null), [error,setError] = useState(''), [fallback,setFallback] = useState('等待作品工作区工具结果…')
  const sequence = useRef(0)
  useEffect(()=>{
    const off = bridge.onNotification((method,params)=>{
      if(method==='ui/resource-teardown'){root.unmount();bridge.dispose();return}
      if(method!=='ui/notifications/tool-result')return
      try{const next=workspaceLaunch(params);setLaunch({...next,sequence:++sequence.current});setError('')}catch(e){setLaunch(null);setError(e.message);setFallback(params?.content?.filter(c=>c.type==='text').map(c=>c.text).join('\n')||'请在项目目录运行 lumi open。')}
    })
    bridge.initialize().catch(e=>setError(e.message))
    const observer=new ResizeObserver(()=>bridge.notify('ui/notifications/size-changed',{height:Math.min(1000,document.documentElement.scrollHeight)}));observer.observe(document.body)
    return()=>{off();observer.disconnect()}
  },[])
  return launch?<MemoryRouter key={launch.project_uuid} initialEntries={[launch.path]}><Workspace launch={launch}/></MemoryRouter>:<main className="mcp-fallback"><h1>Lumi 作品工作区</h1>{error&&<p role="alert">{error}</p>}<pre>{fallback}</pre></main>
}
const root = createRoot(document.getElementById('root'))
root.render(<QueryClientProvider client={client}><App/></QueryClientProvider>)
