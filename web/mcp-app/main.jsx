import React, { useEffect, useMemo, useRef, useState } from 'react'
import { createRoot } from 'react-dom/client'
import { QueryClient, QueryClientProvider, useQuery } from '@tanstack/react-query'
import { Link, MemoryRouter, Route, Routes, useLocation, useNavigate } from 'react-router-dom'
import { AppBridge } from './bridge.js'
import { AppRealtime } from './realtime.js'
import { bindReviewSync } from './sync.js'
import { PreviewCache } from './images.js'
import { createAppAPI, workspaceLaunch, workspaceOrigin } from './workspace.js'
import { createAppImage } from './WorkspaceImage.jsx'
import TextCandidates from '../src/workspace/TextCandidates.jsx'
import { WorkspaceRuntime } from '../src/workspace/runtime.jsx'
import { SimpleHomePage, SimpleBooksPage, SimplePagesPage, SimplePageView, SimpleSettingsPage, SimpleSettingDetailPage, SimpleStoryPage, SimpleBookView, SimpleWorkPreview, SimpleLoading, SimpleError } from '../src/pages/SimpleProjectPages.jsx'
import './workspace.scss'

const bridge = new AppBridge()
const previews = new PreviewCache((...args)=>bridge.call(...args))
const client = new QueryClient({defaultOptions:{queries:{retry:false,refetchOnWindowFocus:false,refetchOnReconnect:false,staleTime:Infinity,gcTime:300000}}})
function OpenLumi({url}) {
  const [error,setError] = useState('')
  return <span><a href={url} onClick={event=>{event.preventDefault();bridge.request('ui/open-link',{url}).then(r=>{if(r?.isError)setError('请复制链接到 Lumi 中打开。')}).catch(()=>setError('请复制链接到 Lumi 中打开。'))}}>在 Lumi 中打开 ↗</a>{error&&<small>{error} {url}</small>}</span>
}
function Workspace({launch}) {
  const location = useLocation(), navigate = useNavigate()
  const [status,setStatus] = useState('正在连接实时提示…')
  const [connection,setConnection] = useState(0)
  const api = useMemo(()=>createAppAPI(bridge,launch.project_uuid,{canWrite:launch.permissions?.can_write !== false}),[launch.project_uuid,launch.permissions?.can_write])
  const projectQuery = useQuery({queryKey:['story-project',launch.project_uuid],queryFn:()=>api.getStoryProject(launch.project_uuid)})
  const project = projectQuery.data
  const origin = workspaceOrigin(project,launch)
  const runtime = useMemo(()=>({DownloadLink:({href,children,...props})=><a {...props} href={origin+href} onClick={event=>{event.preventDefault();if(!href.startsWith(`/media/projects/${launch.project_uuid}/comic-exports/`))return;bridge.request('ui/open-link',{url:origin+href}).then(result=>{if(result?.isError)setStatus('下载入口未打开，请在 Lumi 中继续')}).catch(()=>setStatus('下载入口未打开，请在 Lumi 中继续'))}}>{children}</a>,api,Image:createAppImage(previews,launch.project_uuid),loadImage:async(page,{large,signal,attempt})=>{const uuid=page.current_image?.asset?.uuid||page.asset_uuid;const edge=large?1024:384;if(attempt)previews.evict(launch.project_uuid,uuid,edge);return{src:(await previews.load(launch.project_uuid,uuid,edge,signal)).data_uri}},capabilities:{fileUpload:true,download:true,generation:false,chat:false}}),[api,launch.project_uuid,origin])
  const base = `/projects/${launch.project_uuid}`
  const url = location.pathname.endsWith('/candidates') ? origin + new URL(launch.open_url).pathname : origin + location.pathname + location.search
  useEffect(()=>bindReviewSync({socket:new AppRealtime(bridge,launch.project_uuid),project:launch.project_uuid,queryClient:client,onStatus:setStatus,workspace:true}),[launch.project_uuid,connection])
  useEffect(()=>{navigate(launch.path,{replace:true});void client.invalidateQueries({predicate:query=>query.queryKey?.[1]===launch.project_uuid,refetchType:'active'})},[launch.path,launch.sequence])
  return <WorkspaceRuntime.Provider value={runtime}><div className="mcp-workspace">
    <header className="mcp-workspace__toolbar"><nav aria-label="作品工作区"><Link to={`${base}/`}>作品</Link><Link to={`${base}/chapters`}>章节</Link><Link to={`${base}/premise`}>设定</Link><Link to={`${base}/preview`}>阅读预览</Link></nav><span role="status">{status}</span><button onClick={()=>setConnection(n=>n+1)}>重新连接</button><OpenLumi url={url}/></header>
    {new URLSearchParams(location.search).has('chat_reference_uuid') ? <p className="mcp-workspace__notice" role="status">聊天引用已准备好，请在 Lumi 中继续。<OpenLumi url={url}/></p> : null}
    {launch.permissions?.can_write === false ? <p className="mcp-workspace__notice" role="status">当前连接只有读取权限，提交操作不会写入。</p> : null}
    <details className="mcp-workspace__capabilities"><summary>宿主能力与替代入口</summary><p>编辑、排序、回收站与版本采用在此工作区直接保存到同一项目。文件导入支持单次 6 MiB，导出下载通过宿主在 Lumi 服务中打开。AI 生成和聊天引用需在 Lumi 中继续；不会自动调用模型或切换付费渠道。</p></details>
    <section className={`simple-project-body mcp-workspace__body${/\/sections\//.test(location.pathname)?' mcp-workspace__body--page':''}`}>
      {projectQuery.error?<SimpleError error={projectQuery.error} onRetry={()=>projectQuery.refetch()}/>:!project?<SimpleLoading message="正在读取作品…"/>:<Routes>
        <Route path={`${base}/`} element={<SimpleHomePage project={project} projectUuid={launch.project_uuid} projectQuery={projectQuery}/>}/>
        <Route path={`${base}/chapters`} element={<SimpleBooksPage projectUuid={launch.project_uuid}/>}/>
        <Route path={`${base}/chapters/:chapterUuid`} element={<SimplePagesPage project={project} projectUuid={launch.project_uuid}/>}/>
        <Route path={`${base}/chapters/:chapterUuid/sections/:sectionUuid`} element={<SimplePageView project={project} projectUuid={launch.project_uuid}/>}/>
        <Route path={`${base}/premise`} element={<SimpleSettingsPage projectUuid={launch.project_uuid}/>}/>
        <Route path={`${base}/premise/assets/:assetUuid`} element={<SimpleSettingDetailPage projectUuid={launch.project_uuid}/>}/>
        <Route path={`${base}/story`} element={<SimpleStoryPage project={project} projectUuid={launch.project_uuid}/>}/>
        <Route path={`${base}/preview`} element={<SimpleWorkPreview projectUuid={launch.project_uuid}/>}/>
        <Route path={`${base}/chapters/:chapterUuid/preview`} element={<SimpleBookView projectUuid={launch.project_uuid}/>}/>
        <Route path={`${base}/candidates`} element={<TextCandidates key={launch.sequence} candidates={launch.ui_arguments?.candidates || launch.request.arguments.candidates}/>}/>
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
