import { createStoryAPI } from '../api/story.js'
import { createProductionAPI } from '../api/production.js'
import { createAssetsAPI } from '../api/assets.js'
import { createAiAPI } from '../api/ai.js'

export function createWorkspaceAPI(request) {
  return { ...createStoryAPI(request), ...createProductionAPI(request), ...createAssetsAPI(request), ...createAiAPI(request) }
}
