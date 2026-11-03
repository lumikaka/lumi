import { createContext, useContext } from 'react'
import { createWorkspaceAPI } from './services.js'
import { loadWebBookImage } from '../components/book-preview/webAdapter.js'
import WorkspaceImage from './WorkspaceImage.jsx'


const webRuntime = Object.freeze({
  api: createWorkspaceAPI(), DownloadLink: WorkspaceDownloadLink, Image: WorkspaceImage, loadImage: loadWebBookImage,
  capabilities: { fileUpload: true, download: true, generation: true, chat: true },
})
export const WorkspaceRuntime = createContext(webRuntime)
export function useWorkspaceRuntime() { return useContext(WorkspaceRuntime) }

function WorkspaceDownloadLink({children,...props}) { return <a {...props}>{children}</a> }
