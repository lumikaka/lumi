import React from 'react'
import { createRoot } from 'react-dom/client'
import { HostUnavailable } from './Workspace.jsx'
import './workspace.scss'
export { CindyWorkspace } from './Workspace.jsx'
export { createCindySession } from './session.js'

// Deliberately no query/session/BroadcastChannel connection and no /wake.
// A verified Host sender/permission connector is required before enabling this.
createRoot(document.getElementById('root')).render(<HostUnavailable />)
