// Prefix invalidation also refreshes every open project's inherited defaults.
export function invalidateModelSettingsQueries(queryClient) {
  for (const key of ['model-settings', 'project-model-settings', 'project-image-generation-preflight']) {
    queryClient.invalidateQueries({ queryKey: [key] })
  }
}
