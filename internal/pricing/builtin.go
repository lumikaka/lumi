package pricing

// Public standard CNY prices verified on 2026-09-05, excluding promotions.
// https://help.aliyun.com/zh/model-studio/model-pricing
// https://help.aliyun.com/zh/model-studio/context-cache
// Only unambiguous published channel prices are seeded. In particular, the
// qwen3.8-max cache price is console-only, and some overseas text models have
// multiple deployment scopes for the same region/model. They require overrides.
func builtinRules() []Rule {
	base := func(model, region, kind, mode string) Rule {
		return Rule{ProviderType: "aliyun_bailian", Model: model, Region: region, RequestType: kind, Mode: mode, Currency: "CNY", SourceURL: "https://help.aliyun.com/zh/model-studio/model-pricing", VerifiedAt: "2026-09-05"}
	}
	text := base("qwen3.7-plus", "cn-beijing", "text", "tokens")
	for _, tier := range []struct {
		from, to              int64
		input, cached, output string
	}{{0, 256000, "2", "0.4", "8"}, {256000, 1000000, "6", "1.2", "24"}} {
		for _, entry := range []struct{ metric, price string }{{"input_tokens", tier.input}, {"cached_input_tokens", tier.cached}, {"output_tokens", tier.output}} {
			text.Rates = append(text.Rates, Rate{Metric: entry.metric, Price: entry.price, InputFrom: tier.from, InputTo: Int(tier.to)})
		}
	}
	rules := []Rule{text}
	for _, region := range []string{"cn-beijing", "ap-southeast-1", "eu-central-1", "ap-northeast-1"} {
		input, standard, pro1, pro2 := "0.02", "0.18", "0.25", "0.5"
		if region == "ap-southeast-1" {
			input, standard, pro1, pro2 = "0.022483", "0.224826", "0.299768", "0.562065"
		}
		normal := base("qwen-image-3.0", region, "image", "images")
		normal.Rates = []Rate{{Metric: "input_images", Price: input}, {Metric: "output_images", Price: standard}}
		pro := base("qwen-image-3.0-pro", region, "image", "images")
		pro.Rates = []Rate{{Metric: "input_images", Price: input}, {Metric: "output_images", Price: pro1, Resolution: "1k"}, {Metric: "output_images", Price: pro2, Resolution: "2k"}}
		rules = append(rules, normal, pro)
	}
	return rules
}
