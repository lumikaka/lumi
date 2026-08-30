package story

import "fmt"

const (
	CodeValidationFailed        = "validation_failed"
	CodeProjectRevisionConflict = "project_revision_conflict"
	CodeChapterNotFound         = "chapter_not_found"
	CodeChapterConflict         = "chapter_conflict"
	CodeChapterRevisionConflict = "chapter_revision_conflict"
	CodeChapterStateConflict    = "chapter_state_conflict"
	CodeChapterDeleteBlocked    = "chapter_delete_blocked"
	CodeStoryProfileConflict    = "story_profile_revision_conflict"
	CodeStoryProfileNotFound    = "story_profile_version_not_found"
	CodeStoryMDConflict         = "story_md_conflict"
	CodeStoryProjectionFailed   = "story_projection_failed"
	CodePromptVersionNotFound   = "prompt_version_not_found"
	CodePromptRevisionConflict  = "prompt_revision_conflict"
)

type Error struct {
	Code    string
	Message string
	Details string
	Err     error
}

func (err *Error) Error() string {
	if err.Err != nil {
		return fmt.Sprintf("%s: %v", err.Message, err.Err)
	}
	return err.Message
}

func (err *Error) Unwrap() error { return err.Err }

func storyError(code, message, details string, err error) *Error {
	return &Error{Code: code, Message: message, Details: details, Err: err}
}
