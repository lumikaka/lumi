import { useEffect } from 'react'
import { useQueryClient } from '@tanstack/react-query'

import { getRealtimeSocket } from '../api/realtime.js'
import { isProjectBusinessQuery, projectQueryKeys } from '../api/projectQueryKeys.js'

export function useSiteSettingsRealtime() {
  const queryClient = useQueryClient()
  useEffect(() => {
    const channel = getRealtimeSocket().channel('system')
    const invalidateSiteSettings = () => {
      queryClient.invalidateQueries({ queryKey: ['site-settings'] })
      queryClient.invalidateQueries({ queryKey: ['providers'] })
      queryClient.invalidateQueries({ queryKey: ['active-provider'] })
    }
    const invalidateProjectLifecycle = (payload = {}) => {
      queryClient.invalidateQueries({ queryKey: projectQueryKeys.recent() })
      queryClient.invalidateQueries({ queryKey: projectQueryKeys.openProjects() })
      if (payload.project_uuid) {
		if (!payload.open) queryClient.invalidateQueries({ queryKey: projectQueryKeys.open(payload.project_uuid), exact: true })
        if (payload.open) queryClient.invalidateQueries({ predicate: (query) => isProjectBusinessQuery(query, payload.project_uuid) })
      }
    }
    const invalidateAll = () => {
      invalidateSiteSettings()
      invalidateProjectLifecycle()
    }
    const cleanups = [
      channel.on('site_settings:updated', invalidateSiteSettings),
      channel.on('open_project:changed', invalidateProjectLifecycle),
      channel.on('phx_reconnected', invalidateAll),
    ]
    channel.join()
    return () => {
      cleanups.forEach((cleanup) => cleanup())
      channel.leave()
    }
  }, [queryClient])
}
