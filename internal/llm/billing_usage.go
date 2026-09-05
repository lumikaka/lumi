package llm

import "lumi/internal/pricing"

type wireUsage struct {
	PromptTokens        *int `json:"prompt_tokens"`
	CompletionTokens    *int `json:"completion_tokens"`
	PromptTokensDetails *struct {
		CachedTokens *int `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
}

func (w wireUsage) normalized() Usage {
	u := Usage{}
	if w.PromptTokens != nil {
		u.InputTokens = *w.PromptTokens
		u.InputKnown = true
	}
	if w.CompletionTokens != nil {
		u.OutputTokens = *w.CompletionTokens
		u.OutputKnown = true
	}
	if w.PromptTokensDetails != nil {
		u.CachedInputTokens = w.PromptTokensDetails.CachedTokens
	}
	return u
}
func (u Usage) BillingUsage() pricing.Usage {
	v := pricing.Usage{}
	if u.InputKnown {
		v.InputTokens = pricing.Int(int64(u.InputTokens))
	}
	if u.OutputKnown {
		v.OutputTokens = pricing.Int(int64(u.OutputTokens))
	}
	if u.CachedInputTokens != nil {
		v.CachedInputTokens = pricing.Int(int64(*u.CachedInputTokens))
	}
	return v
}
func (r Response) BillingUsage() pricing.Usage     { return r.Usage.BillingUsage() }
func (r ChatResponse) BillingUsage() pricing.Usage { return r.Usage.BillingUsage() }
