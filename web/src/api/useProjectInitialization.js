import { useQuery } from '@tanstack/react-query'

import { getProjectInitialization } from './projects.js'
import { projectQueryKeys } from './projectQueryKeys.js'

export function useProjectInitialization(projectUuid) {
  return useQuery({
    queryKey: projectQueryKeys.initialization(projectUuid),
    queryFn: () => getProjectInitialization(projectUuid),
    enabled: Boolean(projectUuid),
  })
}
