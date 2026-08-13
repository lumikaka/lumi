import { useEffect } from 'react'
import { useQueryClient } from '@tanstack/react-query'

import { getRealtimeSocket } from '../api/realtime.js'
import { ensureProjectOpen } from '../api/projects.js'
import { isProjectBusinessQuery } from '../api/projectQueryKeys.js'
import { projectRealtimeInvalidation } from './projectRealtimeQueries.js'

export function useProjectRealtimeSync(projectUuid) {
  const queryClient = useQueryClient()

  useEffect(() => {
    if (!projectUuid) return undefined

    let disposed = false
    let scheduled = false
    let invalidateAll = false
    const pending = new Map()

    const flush = () => {
      scheduled = false
      if (disposed) return
      if (invalidateAll) {
        invalidateAll = false
        pending.clear()
        void queryClient.invalidateQueries({
          predicate: (query) => isProjectBusinessQuery(query, projectUuid),
          refetchType: 'active',
        })
        return
      }
      const queryKeys = Array.from(pending.values())
      pending.clear()
      queryKeys.forEach((queryKey) => {
        void queryClient.invalidateQueries({ queryKey, refetchType: 'active' })
      })
    }

    const schedule = () => {
      if (scheduled) return
      scheduled = true
      if (typeof queueMicrotask === 'function') queueMicrotask(flush)
      else Promise.resolve().then(flush)
    }

    const resyncProject = () => {
      invalidateAll = true
      schedule()
    }

    const handleMessage = (event, payload) => {
      const invalidation = projectRealtimeInvalidation(projectUuid, event, payload)
      if (invalidation.invalidateAll) invalidateAll = true
      invalidation.queryKeys.forEach((queryKey) => pending.set(JSON.stringify(queryKey), queryKey))
      if (invalidation.invalidateAll || invalidation.queryKeys.length > 0) schedule()
    }

    const channel = getRealtimeSocket().channel(`project:${projectUuid}`)
    const cleanups = [
      channel.onMessage(handleMessage),
      channel.on('phx_joined', resyncProject),
      channel.on('phx_join_error', async (error) => {
        if (error?.reason !== 'project_not_open') return
        try {
          await ensureProjectOpen(projectUuid)
          if (!disposed) channel.join()
        } catch { /* REST queries expose the actionable open error. */ }
      }),
    ]
    const handleFocus = () => resyncProject()
    const handleVisibility = () => {
      if (document.visibilityState === 'visible') resyncProject()
    }

    window.addEventListener('focus', handleFocus)
    document.addEventListener('visibilitychange', handleVisibility)
    channel.join()

    return () => {
      disposed = true
      cleanups.forEach((cleanup) => cleanup())
      window.removeEventListener('focus', handleFocus)
      document.removeEventListener('visibilitychange', handleVisibility)
      channel.leave()
    }
  }, [projectUuid, queryClient])
}
