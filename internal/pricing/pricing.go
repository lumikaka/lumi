// Package pricing calculates local estimates using immutable, channel-specific
// prices. It deliberately has no dependency on runtime tasks or project stores.
package pricing

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"regexp"
	"strings"
)

var ErrInvalid = errors.New("invalid model price")
var ErrConflict = errors.New("model price changed")
var ErrNotFound = errors.New("model price not found")
var decimalPattern = regexp.MustCompile(`^(0|[1-9][0-9]{0,9})(\.[0-9]{1,9})?$`)
var currencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)

const Scale int64 = 1_000_000_000

type Rate struct {
	Resolution string `json:"resolution,omitempty"`
	Metric     string `json:"metric"`
	Price      string `json:"price"`
	InputFrom  int64  `json:"input_from"`
	InputTo    *int64 `json:"input_to"`
	Size       string `json:"size,omitempty"`
	Quality    string `json:"quality,omitempty"`
}
type Rule struct {
	ProviderUUID string `json:"provider_uuid,omitempty"`
	ProviderType string `json:"provider_type"`
	Model        string `json:"model"`
	Region       string `json:"region"`
	RequestType  string `json:"request_type"`
	Mode         string `json:"mode"`
	Currency     string `json:"currency"`
	ImageModel   string `json:"image_model,omitempty"`
	Rates        []Rate `json:"rates"`
	SourceURL    string `json:"source_url,omitempty"`
	VerifiedAt   string `json:"verified_at,omitempty"`
}
type Price struct {
	UUID string `json:"uuid"`
	Rule
	Source    string `json:"source"`
	Active    bool   `json:"active"`
	CreatedAt string `json:"created_at"`
}
type Context struct {
	ProviderUUID string `json:"provider_uuid"`
	ProviderType string `json:"provider_type"`
	Model        string `json:"model"`
	Region       string `json:"region"`
	RequestType  string `json:"request_type"`
	Size         string `json:"size,omitempty"`
	Quality      string `json:"quality,omitempty"`
}
type Snapshot struct {
	Context Context `json:"context"`
	Price   *Price  `json:"price"`
	Reason  string  `json:"reason,omitempty"`
}

// Nil counters are unknown, including on failed requests. Input token counters
// include cache hits; the calculator subtracts them exactly once.
type Usage struct {
	ToolInputTokens        *int64 `json:"tool_input_tokens,omitempty"`
	CachedToolInputTokens  *int64 `json:"cached_tool_input_tokens,omitempty"`
	Resolution             string `json:"resolution,omitempty"`
	InputTokens            *int64 `json:"input_tokens,omitempty"`
	CachedInputTokens      *int64 `json:"cached_input_tokens,omitempty"`
	OutputTokens           *int64 `json:"output_tokens,omitempty"`
	InputImages            *int64 `json:"input_images,omitempty"`
	OutputImages           *int64 `json:"output_images,omitempty"`
	ImageInputTokens       *int64 `json:"image_input_tokens,omitempty"`
	CachedImageInputTokens *int64 `json:"cached_image_input_tokens,omitempty"`
	ImageOutputTokens      *int64 `json:"image_output_tokens,omitempty"`
	ImageModel             string `json:"image_model,omitempty"`
	Size                   string `json:"size,omitempty"`
	Quality                string `json:"quality,omitempty"`
	// True only when the provider explicitly reports disjoint parent and tool usage.
	Disjoint bool `json:"disjoint,omitempty"`
}
type Line struct {
	Metric   string `json:"metric"`
	Quantity int64  `json:"quantity"`
	Price    string `json:"price"`
	Per      int64  `json:"per"`
	Amount   string `json:"amount"`
}
type Estimate struct {
	Status   string  `json:"status"`
	Reason   string  `json:"reason"`
	Currency string  `json:"currency"`
	Amount   *string `json:"amount"`
	Nanos    *int64  `json:"-"`
	Lines    []Line  `json:"lines"`
}

func Int(value int64) *int64 { return &value }
func Amount(n int64) string  { return fmt.Sprintf("%d.%09d", n/Scale, n%Scale) }
func parsePrice(s string) (int64, error) {
	if !decimalPattern.MatchString(s) {
		return 0, ErrInvalid
	}
	parts := strings.SplitN(s, ".", 2)
	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
	}
	fraction += strings.Repeat("0", 9-len(fraction))
	n, ok := new(big.Int).SetString(parts[0]+fraction, 10)
	if !ok || !n.IsInt64() {
		return 0, ErrInvalid
	}
	return n.Int64(), nil
}
func Metrics(mode string) []string {
	switch mode {
	case "tokens":
		return []string{"input_tokens", "cached_input_tokens", "output_tokens"}
	case "images":
		return []string{"input_images", "output_images"}
	case "responses":
		return []string{"input_tokens", "cached_input_tokens", "output_tokens", "tool_input_tokens", "cached_tool_input_tokens", "image_input_tokens", "cached_image_input_tokens", "image_output_tokens"}
	}
	return nil
}
func Validate(r Rule) error {
	if r.ProviderType != "aliyun_bailian" && r.ProviderType != "cloudflare_ai_gateway" {
		return ErrInvalid
	}
	if strings.TrimSpace(r.Model) != r.Model || r.Model == "" || len(r.Model) > 512 || len(r.Region) > 80 || !currencyPattern.MatchString(r.Currency) || len(r.Rates) > 100 || len(r.SourceURL) > 2048 {
		return ErrInvalid
	}
	if (r.RequestType == "text" && r.Mode != "tokens") || (r.RequestType == "image" && r.Mode != "images" && r.Mode != "responses") || (r.RequestType != "text" && r.RequestType != "image") {
		return ErrInvalid
	}
	if r.Mode == "responses" && (r.ImageModel == "" || len(r.ImageModel) > 512) {
		return ErrInvalid
	}
	if r.SourceURL != "" {
		u, e := url.Parse(r.SourceURL)
		if e != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			return ErrInvalid
		}
	}
	allowed := map[string]bool{}
	for _, m := range Metrics(r.Mode) {
		allowed[m] = true
	}
	found := map[string]bool{}
	for i, a := range r.Rates {
		if !allowed[a.Metric] || a.InputFrom < 0 || (a.InputTo != nil && *a.InputTo <= a.InputFrom) || len(a.Size) > 40 || len(a.Quality) > 40 || (a.Resolution != "" && a.Resolution != "1k" && a.Resolution != "2k" && a.Resolution != "4k") {
			return ErrInvalid
		}
		if _, e := parsePrice(a.Price); e != nil {
			return e
		}
		found[a.Metric] = true
		// A request must have at most one matching rate per metric. Explicit size and
		// quality selectors cannot overlap a wildcard selector in the same tier.
		for _, b := range r.Rates[:i] {
			if a.Metric == b.Metric && (a.Resolution == b.Resolution || a.Resolution == "" || b.Resolution == "") && (a.Size == b.Size || a.Size == "" || b.Size == "") && (a.Quality == b.Quality || a.Quality == "" || b.Quality == "") && (a.InputTo == nil || b.InputFrom < *a.InputTo) && (b.InputTo == nil || a.InputFrom < *b.InputTo) {
				return ErrInvalid
			}
		}
	}
	for m := range allowed {
		if !found[m] {
			return ErrInvalid
		}
	}
	return nil
}
func Calculate(s Snapshot, u Usage) Estimate {
	fail := func(reason string) Estimate { return Estimate{Status: "unpriced", Reason: reason, Lines: []Line{}} }
	if s.Price == nil {
		reason := s.Reason
		if reason == "" {
			reason = "missing_price"
		}
		return fail(reason)
	}
	r := s.Price.Rule
	if Validate(r) != nil {
		return fail("invalid_price")
	}
	if r.Mode == "responses" && (!u.Disjoint || u.ImageModel != r.ImageModel) {
		return fail("incomplete_components")
	}
	counts := map[string]*int64{"tool_input_tokens": u.ToolInputTokens, "cached_tool_input_tokens": u.CachedToolInputTokens, "input_tokens": u.InputTokens, "cached_input_tokens": u.CachedInputTokens, "output_tokens": u.OutputTokens, "input_images": u.InputImages, "output_images": u.OutputImages, "image_input_tokens": u.ImageInputTokens, "cached_image_input_tokens": u.CachedImageInputTokens, "image_output_tokens": u.ImageOutputTokens}
	size, quality := u.Size, u.Quality
	if size == "" {
		size = s.Context.Size
	}
	if quality == "" {
		quality = s.Context.Quality
	}
	lines := []Line{}
	sum := new(big.Int)
	for _, metric := range Metrics(r.Mode) {
		quantity := counts[metric]
		if quantity == nil {
			return fail("missing_usage")
		}
		if *quantity < 0 {
			return fail("invalid_usage")
		}
		n := *quantity
		if metric == "input_tokens" || metric == "image_input_tokens" || metric == "tool_input_tokens" {
			cached := u.CachedInputTokens
			if metric == "tool_input_tokens" {
				cached = u.CachedToolInputTokens
			}
			if metric == "image_input_tokens" {
				cached = u.CachedImageInputTokens
			}
			if cached == nil {
				return fail("missing_usage")
			}
			if *cached < 0 || *cached > n {
				return fail("invalid_usage")
			}
			n -= *cached
		}
		var chosen *Rate
		for i := range r.Rates {
			rate := &r.Rates[i]
			if rate.Metric != metric {
				continue
			}
			if rate.Resolution != "" && rate.Resolution != u.Resolution {
				continue
			}
			if rate.Size != "" && rate.Size != size {
				continue
			}
			if rate.Quality != "" && rate.Quality != quality {
				continue
			}
			if rate.InputFrom > 0 || rate.InputTo != nil {
				if u.InputTokens == nil {
					return fail("missing_usage")
				}
				if (*u.InputTokens <= rate.InputFrom && rate.InputFrom > 0) || (rate.InputTo != nil && *u.InputTokens > *rate.InputTo) {
					continue
				}
			}
			if chosen != nil {
				return fail("invalid_price")
			}
			chosen = rate
		}
		if chosen == nil {
			return fail("missing_rate")
		}
		p, err := parsePrice(chosen.Price)
		if err != nil {
			return fail("invalid_price")
		}
		per := int64(1)
		if strings.HasSuffix(metric, "_tokens") {
			per = 1_000_000
		}
		product := new(big.Int).Mul(big.NewInt(n), big.NewInt(p))
		product.Add(product, big.NewInt(per/2))
		product.Quo(product, big.NewInt(per))
		if !product.IsInt64() {
			return fail("amount_overflow")
		}
		sum.Add(sum, product)
		if !sum.IsInt64() {
			return fail("amount_overflow")
		}
		lines = append(lines, Line{Metric: metric, Quantity: n, Price: chosen.Price, Per: per, Amount: Amount(product.Int64())})
	}
	n := sum.Int64()
	amount := Amount(n)
	return Estimate{Status: "calculated", Currency: r.Currency, Amount: &amount, Nanos: &n, Lines: lines}
}
func RegionFromURL(endpoint string) string {
	u, err := url.Parse(endpoint)
	if err != nil {
		return ""
	}
	for _, region := range []string{"cn-beijing", "ap-southeast-1", "eu-central-1", "ap-northeast-1"} {
		if strings.HasSuffix(u.Hostname(), "."+region+".maas.aliyuncs.com") {
			return region
		}
	}
	return ""
}
func JSON(v any) string { b, _ := json.Marshal(v); return string(b) }

// ParseAmount is the exact inverse of Amount for nonnegative SQLite amounts.
func ParseAmount(s string) (int64, error) { return parsePrice(s) }
