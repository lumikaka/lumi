UPDATE global_model_settings
SET provider_rate_limits = json_remove(provider_rate_limits, '$.cloudflare_ai_gateway')
WHERE singleton = 1
  AND json_extract(provider_rate_limits, '$.cloudflare_ai_gateway.rpm') = 200
  AND json_type(provider_rate_limits, '$.cloudflare_ai_gateway.rps') = 'null';
