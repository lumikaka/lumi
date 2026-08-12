import { useEffect } from 'react'

import { getRealtimeSocket } from '../api/realtime.js'
import { ensureProjectOpen } from '../api/projects.js'

export function useProjectPresence(projectUuid) {
  useEffect(() => {
    if (!projectUuid) return undefined
		let disposed = false
		const channel = getRealtimeSocket().channel(`project:${projectUuid}`)
		const cleanup = channel.on('phx_join_error', async (error) => {
			if (error?.reason !== 'project_not_open') return
			try {
				await ensureProjectOpen(projectUuid)
				if (!disposed) channel.join()
			} catch { /* The route gate renders the open failure. */ }
		})
		channel.join()
		return () => { disposed = true; cleanup(); channel.leave() }
  }, [projectUuid])
}
