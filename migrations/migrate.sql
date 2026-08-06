CREATE TABLE IF NOT EXISTS tasks(
    id UUID PRIMARY KEY,
    owner_id BIGINT NOT NULL,
    title VARCHAR(50) NOT NULL,
    "status" INT NOT NULL,
    "description" TEXT,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_owner_created
ON tasks (owner_id, created_at DESC);