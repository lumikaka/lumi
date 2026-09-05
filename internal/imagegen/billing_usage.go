package imagegen

import (
	"lumi/internal/pricing"
	"math"
	"strings"
)

type responsesUsage struct {
	InputTokens        *int64 `json:"input_tokens"`
	OutputTokens       *int64 `json:"output_tokens"`
	InputTokensDetails struct {
		CachedTokens        *int64 `json:"cached_tokens"`
		TextTokens          *int64 `json:"text_tokens"`
		ImageTokens         *int64 `json:"image_tokens"`
		CachedTokensDetails struct {
			TextTokens  *int64 `json:"text_tokens"`
			ImageTokens *int64 `json:"image_tokens"`
		} `json:"cached_tokens_details"`
	} `json:"input_tokens_details"`
}
type responsesImage struct {
	Type          string          `json:"type"`
	Result        string          `json:"result"`
	RevisedPrompt string          `json:"revised_prompt"`
	Model         string          `json:"model"`
	Size          string          `json:"size"`
	Quality       string          `json:"quality"`
	Usage         *responsesUsage `json:"usage"`
}
type responsesEnvelope struct {
	Output []responsesImage `json:"output"`
	Usage  *responsesUsage  `json:"usage"`
	Tools  []struct {
		Type  string `json:"type"`
		Model string `json:"model"`
	} `json:"tools"`
}

// Responses mainline usage is separate from image generation costs:
// https://developers.openai.com/api/docs/guides/image-generation
// Never guess image-tool usage from the mainline totals or from output size.
// Gate token pricing on complete modality counters and a reported image model.
func (e responsesEnvelope) billingUsage(input Request) pricing.Usage {
	u := pricing.Usage{InputImages: pricing.Int(int64(len(input.Images))), Size: input.Size, Quality: input.Quality}
	if e.Usage != nil {
		u.InputTokens = e.Usage.InputTokens
		u.CachedInputTokens = e.Usage.InputTokensDetails.CachedTokens
		u.OutputTokens = e.Usage.OutputTokens
	}
	var item *responsesImage
	count := 0
	for i := range e.Output {
		if e.Output[i].Type == "image_generation_call" {
			item = &e.Output[i]
			count++
		}
	}
	if count != 1 {
		return u
	}
	u.ImageModel = item.Model
	if u.ImageModel == "" {
		models := []string{}
		for _, tool := range e.Tools {
			if tool.Type == "image_generation" && tool.Model != "" {
				models = append(models, tool.Model)
			}
		}
		if len(models) == 1 {
			u.ImageModel = models[0]
		}
	}
	if item.Size != "" && item.Size != "auto" {
		u.Size = item.Size
	}
	if item.Quality != "" && item.Quality != "auto" {
		u.Quality = item.Quality
	}
	if item.Usage == nil {
		return u
	}
	tool := item.Usage
	u.ToolInputTokens = tool.InputTokensDetails.TextTokens
	u.ImageInputTokens = tool.InputTokensDetails.ImageTokens
	u.ImageOutputTokens = tool.OutputTokens
	u.CachedToolInputTokens = tool.InputTokensDetails.CachedTokensDetails.TextTokens
	u.CachedImageInputTokens = tool.InputTokensDetails.CachedTokensDetails.ImageTokens
	if cached := tool.InputTokensDetails.CachedTokens; cached != nil && *cached == 0 {
		u.CachedToolInputTokens = pricing.Int(0)
		u.CachedImageInputTokens = pricing.Int(0)
	}
	if u.ToolInputTokens == nil || u.ImageInputTokens == nil || tool.InputTokens == nil {
		return u
	}
	a, b := *u.ToolInputTokens, *u.ImageInputTokens
	if a < 0 || b < 0 || a > math.MaxInt64-b || a+b != *tool.InputTokens {
		return u
	}
	u.Disjoint = strings.HasPrefix(input.Model, "openai/")
	return u
}
