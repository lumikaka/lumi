package project

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

const (
	PictureBookClassic     = "classic_picture_book"
	PictureBookWordless    = "wordless_picture_book"
	PictureBookInteractive = "interactive_picture_book"
	PictureBookComicStory  = "comic_story"
	PictureBookVertical    = "vertical_strip"

	AspectLandscape = "landscape"
	AspectSquare    = "square"
	AspectPortrait  = "portrait"
	AspectCustom    = "custom"
	AspectFixed     = "fixed"

	InteractionFindIt      = "find_it"
	InteractionMakeChoice  = "make_a_choice"
	InteractionGuess       = "guess"
	InteractionFollowAlong = "follow_along"

	ComicLayoutFourPanel = "four_panel"
	ComicLayoutPageComic = "page_comic"
)

type AspectRatioInput struct {
	Mode   string `json:"mode"`
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
}

type PictureBookInput struct {
	Format                string            `json:"format"`
	AspectRatio           *AspectRatioInput `json:"aspect_ratio"`
	LargeImageMinimalText *bool             `json:"large_image_minimal_text"`
	InteractionMode       *string           `json:"interaction_mode"`
	ComicLayout           *string           `json:"comic_layout"`
}

type AspectRatio struct {
	Mode   string `json:"mode"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

type PictureBookProfile struct {
	Format                string      `json:"format"`
	AspectRatio           AspectRatio `json:"aspect_ratio"`
	LargeImageMinimalText *bool       `json:"large_image_minimal_text"`
	InteractionMode       *string     `json:"interaction_mode"`
	ComicLayout           *string     `json:"comic_layout"`
}

type pictureBookProfileRecord struct {
	ID                    int64 `gorm:"primaryKey;autoIncrement"`
	ProjectID             int64
	Format                string
	AspectRatioMode       string
	AspectWidth           int
	AspectHeight          int
	LargeImageMinimalText *bool
	InteractionMode       *string
	ComicLayout           *string
	CreatedAt             time.Time
}

func (pictureBookProfileRecord) TableName() string { return "project_picture_book_profiles" }

func boolPointer(value bool) *bool       { return &value }
func stringPointer(value string) *string { return &value }

func clonePictureBookProfile(profile PictureBookProfile) PictureBookProfile {
	cloned := profile
	if profile.LargeImageMinimalText != nil {
		cloned.LargeImageMinimalText = boolPointer(*profile.LargeImageMinimalText)
	}
	if profile.InteractionMode != nil {
		cloned.InteractionMode = stringPointer(*profile.InteractionMode)
	}
	if profile.ComicLayout != nil {
		cloned.ComicLayout = stringPointer(*profile.ComicLayout)
	}
	return cloned
}

func gcd(left, right int) int {
	for right != 0 {
		left, right = right, left%right
	}
	if left < 0 {
		return -left
	}
	return left
}

func normalizedAspect(input *AspectRatioInput) (AspectRatio, error) {
	if input == nil {
		return AspectRatio{Mode: AspectLandscape, Width: 4, Height: 3}, nil
	}
	mode := strings.ToLower(strings.TrimSpace(input.Mode))
	if mode == "" {
		mode = AspectLandscape
	}
	switch mode {
	case AspectLandscape:
		if input.Width != 0 || input.Height != 0 {
			return AspectRatio{}, invalidPictureBook("预设比例不能同时提供 width 或 height。")
		}
		return AspectRatio{Mode: mode, Width: 4, Height: 3}, nil
	case AspectSquare:
		if input.Width != 0 || input.Height != 0 {
			return AspectRatio{}, invalidPictureBook("预设比例不能同时提供 width 或 height。")
		}
		return AspectRatio{Mode: mode, Width: 1, Height: 1}, nil
	case AspectPortrait:
		if input.Width != 0 || input.Height != 0 {
			return AspectRatio{}, invalidPictureBook("预设比例不能同时提供 width 或 height。")
		}
		return AspectRatio{Mode: mode, Width: 3, Height: 4}, nil
	case AspectCustom:
		if input.Width < 1 || input.Width > 100 || input.Height < 1 || input.Height > 100 {
			return AspectRatio{}, invalidPictureBook("自定义 width 与 height 必须是 1 到 100 的整数。")
		}
		if input.Width*3 < input.Height || input.Height*3 < input.Width {
			return AspectRatio{}, invalidPictureBook("自定义比例必须位于 1:3 到 3:1 之间。")
		}
		divisor := gcd(input.Width, input.Height)
		return AspectRatio{Mode: mode, Width: input.Width / divisor, Height: input.Height / divisor}, nil
	default:
		return AspectRatio{}, invalidPictureBook("aspect_ratio.mode 只支持 landscape、square、portrait 或 custom。")
	}
}

func optionalValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(*value))
}

// NormalizePictureBookInput applies the creation defaults and rejects fields
// which do not belong to the selected immutable format.
func NormalizePictureBookInput(input *PictureBookInput) (PictureBookProfile, error) {
	if input == nil {
		input = &PictureBookInput{}
	}
	format := strings.ToLower(strings.TrimSpace(input.Format))
	if format == "" {
		format = PictureBookClassic
	}
	if format == PictureBookVertical {
		if input.AspectRatio != nil || input.LargeImageMinimalText != nil || input.InteractionMode != nil || input.ComicLayout != nil {
			return PictureBookProfile{}, invalidPictureBook("条漫比例固定为 1:3，不能提供其他绘本参数。")
		}
		return PictureBookProfile{Format: format, AspectRatio: AspectRatio{Mode: AspectFixed, Width: 1, Height: 3}}, nil
	}
	if format == PictureBookInteractive && input.AspectRatio != nil {
		return PictureBookProfile{}, invalidPictureBook("互动绘本不支持 aspect_ratio 参数。")
	}
	aspect, err := normalizedAspect(input.AspectRatio)
	if err != nil {
		return PictureBookProfile{}, err
	}
	profile := PictureBookProfile{Format: format, AspectRatio: aspect}
	switch format {
	case PictureBookClassic:
		if input.InteractionMode != nil || input.ComicLayout != nil {
			return PictureBookProfile{}, invalidPictureBook("经典图文不能提供 interaction_mode 或 comic_layout。")
		}
		minimal := false
		if input.LargeImageMinimalText != nil {
			minimal = *input.LargeImageMinimalText
		}
		profile.LargeImageMinimalText = boolPointer(minimal)
	case PictureBookWordless:
		if input.LargeImageMinimalText != nil || input.InteractionMode != nil || input.ComicLayout != nil {
			return PictureBookProfile{}, invalidPictureBook("无字绘本只能配置画面比例。")
		}
	case PictureBookInteractive:
		if input.LargeImageMinimalText != nil || input.ComicLayout != nil {
			return PictureBookProfile{}, invalidPictureBook("互动绘本不能提供 large_image_minimal_text 或 comic_layout。")
		}
		mode := optionalValue(input.InteractionMode)
		if mode == "" {
			mode = InteractionFindIt
		}
		if mode != InteractionFindIt && mode != InteractionMakeChoice && mode != InteractionGuess && mode != InteractionFollowAlong {
			return PictureBookProfile{}, invalidPictureBook("interaction_mode 不受支持。")
		}
		profile.InteractionMode = stringPointer(mode)
	case PictureBookComicStory:
		if input.LargeImageMinimalText != nil || input.InteractionMode != nil {
			return PictureBookProfile{}, invalidPictureBook("漫画故事不能提供 large_image_minimal_text 或 interaction_mode。")
		}
		layout := optionalValue(input.ComicLayout)
		if layout == "" {
			layout = ComicLayoutPageComic
		}
		if layout != ComicLayoutFourPanel && layout != ComicLayoutPageComic {
			return PictureBookProfile{}, invalidPictureBook("comic_layout 只支持 four_panel 或 page_comic。")
		}
		profile.ComicLayout = stringPointer(layout)
	default:
		return PictureBookProfile{}, invalidPictureBook("picture_book.format 不受支持。")
	}
	return profile, nil
}

func invalidPictureBook(details string) error {
	return projectError(CodeInvalidPictureBook, "绘本形式配置无效", details, nil)
}

func loadPictureBookProfile(ctx context.Context, db *gorm.DB, projectID int64) (PictureBookProfile, error) {
	profile, err := loadPictureBookProfileOptional(ctx, db, projectID)
	if err != nil {
		return PictureBookProfile{}, err
	}
	if profile == nil {
		return PictureBookProfile{}, fmt.Errorf("project must contain exactly one picture book profile, found 0")
	}
	return *profile, nil
}

func loadPictureBookProfileOptional(ctx context.Context, db *gorm.DB, projectID int64) (*PictureBookProfile, error) {
	var records []pictureBookProfileRecord
	if err := db.WithContext(ctx).Where("project_id = ?", projectID).Limit(2).Find(&records).Error; err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, nil
	}
	if len(records) != 1 {
		return nil, fmt.Errorf("project must contain at most one picture book profile, found %d", len(records))
	}
	record := records[0]
	profile := PictureBookProfile{
		Format:                record.Format,
		AspectRatio:           AspectRatio{Mode: record.AspectRatioMode, Width: record.AspectWidth, Height: record.AspectHeight},
		LargeImageMinimalText: record.LargeImageMinimalText, InteractionMode: record.InteractionMode, ComicLayout: record.ComicLayout,
	}
	// Re-run the same cross-field rules used by creation so corrupt databases
	// are rejected even if foreign-key/check enforcement was disabled earlier.
	input := &PictureBookInput{Format: profile.Format}
	if profile.Format != PictureBookVertical && profile.Format != PictureBookInteractive {
		input.AspectRatio = &AspectRatioInput{Mode: profile.AspectRatio.Mode, Width: profile.AspectRatio.Width, Height: profile.AspectRatio.Height}
		if profile.AspectRatio.Mode != AspectCustom {
			input.AspectRatio.Width, input.AspectRatio.Height = 0, 0
		}
	}
	input.LargeImageMinimalText, input.InteractionMode, input.ComicLayout = profile.LargeImageMinimalText, profile.InteractionMode, profile.ComicLayout
	normalized, err := NormalizePictureBookInput(input)
	if err != nil {
		return nil, err
	}
	if normalized.AspectRatio != profile.AspectRatio {
		return nil, fmt.Errorf("picture book aspect ratio is not canonical")
	}
	return &normalized, nil
}
