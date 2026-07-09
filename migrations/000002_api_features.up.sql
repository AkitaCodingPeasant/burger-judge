ALTER TABLE problems
    ADD COLUMN description TEXT NOT NULL DEFAULT '';

CREATE TABLE revoked_tokens (
    token_id TEXT PRIMARY KEY,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX submissions_user_created_idx
    ON submissions (user_id, created_at DESC);

CREATE INDEX submissions_problem_status_idx
    ON submissions (problem_id, status);
