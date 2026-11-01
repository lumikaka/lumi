package story

import "context"

// Read the committed profile without reconciling STORY.md or repairing a
// pending projection. Read-only callers must not cause implicit file writes.
func (service *Service) storedStoryProfile(ctx context.Context) (StoryProfile, error) {
	var current storyProfileRecord
	err := service.store.DB().WithContext(ctx).
		Where("project_id = (SELECT id FROM projects WHERE uuid = ?) AND is_current = 1", service.store.ProjectUUID()).
		First(&current).Error
	return profileDTO(current), err
}
