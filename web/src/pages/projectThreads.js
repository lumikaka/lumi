import { useInfiniteQuery } from '@tanstack/react-query'

import { listChatThreads } from '../api/chat.js'

export const PROJECT_THREADS_PAGE_SIZE = 20

export function projectThreadsQueryKey(projectUuid) {
  return ['chat-threads', projectUuid, 'pages']
}

export function useProjectThreads(projectUuid, enabled = true) {
  return useInfiniteQuery({
    queryKey: projectThreadsQueryKey(projectUuid),
    queryFn: ({ pageParam }) => listChatThreads(projectUuid, { page: pageParam, perPage: PROJECT_THREADS_PAGE_SIZE }),
    initialPageParam: 1,
    getNextPageParam: (lastPage) => lastPage.pagination.current_page < lastPage.pagination.last_page
      ? lastPage.pagination.current_page + 1
      : undefined,
    enabled: Boolean(projectUuid) && enabled,
  })
}

export function flattenProjectThreads(pages = []) {
  const seen = new Set()
  return pages.flatMap((page) => page.items || []).filter((thread) => {
    if (!thread?.uuid || seen.has(thread.uuid)) return false
    seen.add(thread.uuid)
    return true
  })
}
