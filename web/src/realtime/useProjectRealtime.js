import { useEffect } from 'react'

import { getRealtimeSocket } from '../api/realtime.js'
import { ensureProjectOpen } from '../api/projects.js'
import { PROJECT_AGENT_EVENTS } from '../pages/chatWorkspaceState.js'

const TASK_EVENTS = [
  'task:queued', 'task:running', 'task:progress', 'task:completed', 'task:failed', 'task:cancelled',
  'production_task:queued', 'production_task:running', 'production_task:progress', 'production_task:completed', 'production_task:failed', 'production_task:cancelled',
  'production:resource_changed',
  'story:chapter_changed',
]

const PROJECT_EVENTS = Array.from(new Set([...TASK_EVENTS, ...PROJECT_AGENT_EVENTS]))

export function useProjectRealtime(projectUuid, onEvent) {
  useEffect(() => {
    if (!projectUuid) return undefined
		let disposed = false
    const channel = getRealtimeSocket().channel(`project:${projectUuid}`)
    const cleanups = PROJECT_EVENTS.map((event) => channel.on(event, (payload) => onEvent?.(event, payload)))
		cleanups.push(channel.on('phx_reconnected', () => onEvent?.('phx_reconnected', { project_uuid: projectUuid })))
		cleanups.push(channel.on('phx_join_error', async (error) => {
			if (error?.reason !== 'project_not_open') return
			try {
				await ensureProjectOpen(projectUuid)
				if (!disposed) channel.join()
			} catch { /* REST queries expose the actionable open error. */ }
		}))
    channel.join()
    return () => {
			disposed = true
      cleanups.forEach((cleanup) => cleanup())
      channel.leave()
    }
  }, [projectUuid, onEvent])
}
