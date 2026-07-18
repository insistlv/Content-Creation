ALTER TABLE commerce_generation_items
    ADD COLUMN IF NOT EXISTS progress_percent INTEGER NOT NULL DEFAULT 0;

UPDATE commerce_generation_items
SET progress_percent = CASE
    WHEN status IN ('succeeded', 'failed', 'canceled') THEN 100
    WHEN status = 'running' THEN 10
    ELSE 0
END
WHERE progress_percent = 0;
