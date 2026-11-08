-- API token authentication looks a digest up on every request (Story 5.5).
-- A digest must map to exactly one token, so the index is unique and additive.
CREATE UNIQUE INDEX IF NOT EXISTS api_tokens_token_hash_key ON api_tokens (token_hash);
