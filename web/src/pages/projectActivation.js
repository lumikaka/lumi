import { ensureProjectOpen as openProject } from '../api/projects.js'

export async function ensureProjectOpen(projectUuid) {
	return openProject(projectUuid)
}
