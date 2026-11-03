-- +goose Up
-- Add explicit capture_source column to screenshots table
ALTER TABLE screenshots ADD COLUMN IF NOT EXISTS capture_source text NOT NULL DEFAULT 'automatic';

-- Constraint: allow 'automatic', 'manual', or 'unknown' (for legacy data)
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_screenshots_capture_source'
    ) THEN
        ALTER TABLE screenshots ADD CONSTRAINT chk_screenshots_capture_source CHECK (capture_source IN ('automatic', 'manual', 'unknown'));
    END IF;
END $$;
-- +goose StatementEnd

-- Backfill records: if request_id is not null, it was manual
UPDATE screenshots SET capture_source = 'manual' WHERE request_id IS NOT NULL;

-- Create indexes for filtering by business/user and capture source
CREATE INDEX IF NOT EXISTS idx_screenshots_user_source_ts ON screenshots(user_id, capture_source, ts DESC);
CREATE INDEX IF NOT EXISTS idx_screenshots_biz_source_ts ON screenshots(business_id, capture_source, ts DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_screenshots_biz_source_ts;
DROP INDEX IF EXISTS idx_screenshots_user_source_ts;
ALTER TABLE screenshots DROP CONSTRAINT IF EXISTS chk_screenshots_capture_source;
ALTER TABLE screenshots DROP COLUMN IF EXISTS capture_source;
