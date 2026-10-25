import type { ModelRuntime } from "@earendil-works/pi-coding-agent";
export function configureEvaluationModel(runtime: ModelRuntime): void {
  runtime.registerProvider("lumi-eval", {"baseUrl": "https://ws-ie8ia23er4o7fynb.cn-beijing.maas.aliyuncs.com/compatible-mode/v1", "api": "openai-completions", "models": [{"id": "qwen3.7-plus", "name": "qwen3.7-plus", "reasoning": true, "input": ["text"], "cost": {"input": 0, "output": 0, "cacheRead": 0, "cacheWrite": 0}, "contextWindow": 262144, "maxTokens": 4096, "compat": {"maxTokensField": "max_tokens", "supportsDeveloperRole": false, "supportsStore": false, "supportsReasoningEffort": false, "thinkingFormat": "openai", "supportsUsageInStreaming": true}}]});
}
