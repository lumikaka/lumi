import { modelTestKeys } from '../api/modelTests.js'

export function invalidateModelTestQueries(queryClient, payload = {}) {
  if (!payload.test_uuid) {
    queryClient.invalidateQueries({ queryKey: modelTestKeys.all })
    return
  }
  queryClient.invalidateQueries({ queryKey: modelTestKeys.lists })
  queryClient.invalidateQueries({ queryKey: modelTestKeys.detail(payload.test_uuid), exact: true })
}
