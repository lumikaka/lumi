-- Cloudflare AI Gateway Unified Billing: 200 requests per 60 seconds per gateway.
-- json_insert preserves an existing rule set by the user.
UPDATE global_model_settings
SET provider_rate_limits = json_insert(
    provider_rate_limits,
    '$.cloudflare_ai_gateway',
    json('{"rpm":200,"rps":null}')
)
WHERE singleton = 1;
