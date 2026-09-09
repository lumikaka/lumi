-- Preserve supported selections; older models fall back to the Terra default.
DELETE FROM site_settings WHERE key IN (
    'ai_providers.openai_compatible.default_model',
    'ai_providers.openai_compatible.default_image_model'
) AND value NOT IN ('"openai/gpt-5.6-terra"', '"openai/gpt-5.6-sol"');

-- Chat Completions verification does not establish access through Responses.
DELETE FROM site_settings WHERE key IN (
    'ai_providers.openai_compatible.verified',
    'ai_providers.openai_compatible.verified_at',
    'ai_providers.openai_compatible.verified_fingerprint'
);
