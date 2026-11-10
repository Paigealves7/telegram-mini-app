BEGIN;

-- Link produced stock rows to the operation that created them so production history
-- can report both inputs and outputs. Existing boards remain unlinked.
ALTER TABLE boards
    ADD COLUMN IF NOT EXISTS sawing_id BIGINT REFERENCES sawing_operations(id);

CREATE INDEX IF NOT EXISTS idx_boards_sawing_id ON boards(sawing_id) WHERE sawing_id IS NOT NULL;

COMMIT;
