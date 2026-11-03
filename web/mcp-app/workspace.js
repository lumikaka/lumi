import { createWorkspaceAPI } from '../src/workspace/services.js'
import { ApiError } from '../src/api/client.js'
import { uuidV7, safeOpenURL, envelope as resultEnvelope } from './contracts.js'

export function appResponse(result) {
  const response = result?._meta?.['lumi/app_response']
  if (!response || typeof response.success !== 'boolean') throw new ApiError({code:'app_meta_missing',message:'宿主未转交界面专用数据',details:'请在 Lumi 中打开；不会改为把完整正文重复放入模型上下文。'},502)
  if (!response.success || result.isError) throw new ApiError(response.error, response.error?.code?.includes('conflict') ? 409 : 400)
  return response.data
}
export function createAppRequest(bridge, project, {canWrite=true} = {}) {
  if (!uuidV7.test(project)) throw new Error('工作区项目无效')
  const prefix = `/api/v1/projects/${project}`
  return async (path, options = {}) => {
    if (typeof path !== 'string' || !(path === prefix || path.startsWith(`${prefix}/`) || path.startsWith(`${prefix}?`))) throw new ApiError({code:'app_project_mismatch',message:'请求超出当前绑定项目'},403)
    const parsed = new URL(path, 'http://lumi.invalid')
    if (!(parsed.pathname === prefix || parsed.pathname.startsWith(`${prefix}/`)) || path.split('?')[0].includes('%') || path.split('?')[0].split('/').some(part => part === '.' || part === '..')) throw new ApiError({code:'app_resource_forbidden',message:'工作区资源路径无效'},403)
    const resource = parsed.pathname.slice(prefix.length + 1) || 'project'
    const method = options.method || 'GET'
    let uploadMeta
    let uploadBody
    if (options.body instanceof FormData) {
      const file=options.body.get('file')
      if (resource !== 'asset-uploads' || !file || typeof file.arrayBuffer !== 'function') throw unavailable('当前文件请求不受支持')
      if (file.size > 6 * 1024 * 1024) throw unavailable('此宿主的单次文件传输上限为 6 MiB')
      const bytes=new Uint8Array(await file.arrayBuffer());let binary='';for(let i=0;i<bytes.length;i+=32768)binary+=String.fromCharCode(...bytes.subarray(i,i+32768))
      uploadMeta={'lumi/upload':{data_base64:btoa(binary)}}
      uploadBody={purpose:options.body.get('purpose'),display_name:options.body.get('display_name')||'',file_name:file.name,file_type:file.type}
    } else if (options.body && typeof options.body !== 'string') throw unavailable('当前文件请求不受支持')
    const query = Object.fromEntries(parsed.searchParams)
    const args = {resource, ...(Object.keys(query).length ? {query} : {})}
    if (method !== 'GET') {
      if (!canWrite) throw new ApiError({code:'project_read_only',message:'当前连接只有读取权限',details:'写入操作未执行。'},403)
      if (/generations|generation-batches|reconstructions/.test(resource)) throw unavailable('此操作需要 Lumi 的生成或导出运行环境')
      args.method = method
      if (uploadBody) args.body=uploadBody
      else if (options.body) args.body = JSON.parse(options.body)
    }
    return appResponse(await bridge.call(method === 'GET' ? 'read_app_resource' : 'write_app_resource', args, {signal:options.signal,meta:{'lumi/expected_project_uuid':project,...uploadMeta}}))
  }
}
function unavailable(message) { return new ApiError({code:'workspace_capability_unavailable',message,details:'已保留原有功能入口。请使用“在 Lumi Web 中打开”在同一业务资源继续操作。'},501) }
export function createAppAPI(bridge, project, options) { return createWorkspaceAPI(createAppRequest(bridge, project, options)) }

export function workspaceLaunch(result) {
  const envelope = resultEnvelope(result)
  const data = envelope?.data
  if (!envelope?.success || !data || !uuidV7.test(data.project_uuid) || !uuidV7.test(data.target_uuid) || !Number.isSafeInteger(data.revision) || data.revision < 0 || !safeOpenURL(data.open_url,data.project_uuid)) throw new Error(envelope?.error?.message || '工作区入口无效')
  const args = data.request?.arguments || {}
  for (const key of ['chapter_uuid','page_uuid','setting_uuid']) if (args[key] && !uuidV7.test(args[key])) throw new Error('工作区目标无效')
  let view = data.view
  const base = `/projects/${data.project_uuid}`
  const ch = args.chapter_uuid
  const legacyBook = view === 'reader' || view === 'book'
  if (legacyBook && data.scope === 'chapter' && !ch) throw new Error('指定章节缺少章节 UUID，未扩大为整本')
  if (legacyBook && data.scope && (data.scope !== (ch ? 'chapter' : 'book'))) throw new Error('作品范围无效')
  if (data.chapter_uuid && data.chapter_uuid !== ch) throw new Error('章节身份与入口参数不一致')
  const expectedTarget = view === 'chapter' ? ch : view === 'page' || (view === 'candidates' && args.kind === 'page') ? args.page_uuid : ((view === 'settings' || (view === 'candidates' && args.kind === 'setting')) && args.setting_uuid) || data.project_uuid
  if (data.target_uuid !== expectedTarget) throw new Error('工作区目标与入口参数不一致')
  // Historical reader/book results carry a project target even for one chapter.
  // Validate that identity first, then route to the same normal business scope.
  if (legacyBook) view = ch ? 'chapter' : 'book'
  let path = `${base}/`
  if (view === 'chapters') path = `${base}/chapters`
  else if (view === 'chapter' && ch) path = `${base}/chapters/${ch}`
  else if (view === 'page' && ch && args.page_uuid) path = `${base}/chapters/${ch}/sections/${args.page_uuid}`
  else if (view === 'settings') path = args.setting_uuid ? `${base}/premise/assets/${args.setting_uuid}` : `${base}/premise`
  else if (view === 'candidates' && args.kind === 'page' && (!ch || !args.page_uuid)) throw new Error('候选页面目标不完整')
  else if (view === 'candidates' && args.kind === 'setting' && !args.setting_uuid) throw new Error('候选设定目标不完整')
  else if (view === 'candidates') path = args.kind === 'page' ? `${base}/chapters/${ch}/sections/${args.page_uuid}?panel=candidates` : args.kind === 'setting' ? `${base}/premise/assets/${args.setting_uuid}?panel=candidates` : `${base}/candidates`
  else if (view !== 'book') throw new Error('工作区视图或目标不完整')
  const origin = new URL(data.open_url).origin
  const {mode, ...normalArgs} = args
  return {...data,view,path,open_url:legacyBook ? origin + path : data.open_url,target_uuid:legacyBook && ch ? ch : data.target_uuid,request:legacyBook ? {name:`review_${view}`,arguments:normalArgs} : data.request,ui_arguments:result?._meta?.['lumi/launch_arguments'],origin}
}

// A rejoin can connect to a restarted CLI on another port. The calibrated
// project REST response supplies its current Web origin; never retain a dead
// launch port or accept a URL belonging to another project.
export function workspaceOrigin(project, launch) {
 return safeOpenURL(project?.open_url,launch.project_uuid) ? new URL(project.open_url).origin : launch.origin
}
