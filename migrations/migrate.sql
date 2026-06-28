CREATE TABLE IF NOT EXISTS tasks(
    id UUID PRIMARY KEY,
    owner_id UUID NOT NULL,
    title VARCHAR(50) NOT NULL,
    "status" INT NOT NULL,
    "description" TEXT,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
)