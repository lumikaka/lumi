import { useEffect, useState } from 'react'

import { getRealtimeSocket } from '../api/realtime.js'

export function useRealtimeStatus() {
  const [status, setStatus] = useState('connecting')

  useEffect(() => {
    const channel = getRealtimeSocket().channel('system')
    channel.on('phx_status', ({ status: channelStatus }) => {
      if (channelStatus === 'joined') setStatus('online')
      else if (channelStatus === 'error' || channelStatus === 'timeout') setStatus('offline')
      else setStatus('connecting')
    })
    channel.on('phx_disconnected', () => setStatus('connecting'))
    channel.on('phx_join_error', () => setStatus('offline'))
    channel.join()
      .receive('ok', () => setStatus('online'))
      .receive('error', () => setStatus('offline'))
      .receive('timeout', () => setStatus('offline'))

    return () => channel.leave()
  }, [])

  return status
}
