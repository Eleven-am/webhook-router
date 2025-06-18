-- Add signature verification configuration to routes table
-- This migration adds support for configurable webhook signature verification

-- Add signature configuration columns to routes table
ALTER TABLE routes ADD COLUMN signature_config TEXT DEFAULT NULL;
ALTER TABLE routes ADD COLUMN signature_secret TEXT DEFAULT NULL;

-- The signature_config column will store JSON configuration like:
-- {
--   "enabled": true,
--   "verifications": [{
--     "header": "X-Signature",
--     "format": "sha256=${signature}",
--     "algorithm": "hmac-sha256",
--     "encoding": "hex"
--   }],
--   "timestamp_validation": {
--     "enabled": true,
--     "header": "X-Timestamp",
--     "tolerance": 300
--   }
-- }

-- The signature_secret column will store encrypted secret keys
-- Multiple secrets can be stored as encrypted JSON if needed

-- Create index for routes with signature verification enabled
CREATE INDEX idx_routes_signature_enabled ON routes((signature_config IS NOT NULL));