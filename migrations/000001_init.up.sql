CREATE TYPE user_role AS ENUM ('user', 'admin');
CREATE TYPE submission_status AS ENUM ('PENDING', 'RUNNING', 'AC', 'WA', 'SE', 'CE', 'RE', 'TLE');

CREATE TABLE users (
    id UUID PRIMARY KEY,
    account TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role user_role NOT NULL DEFAULT 'user',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE problems (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    settings_path TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE submissions (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id),
    problem_id TEXT NOT NULL REFERENCES problems(id),
    status submission_status NOT NULL DEFAULT 'PENDING',
    score INTEGER NOT NULL DEFAULT 0 CHECK (score >= 0),
    source_path TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at TIMESTAMPTZ,
    ended_at TIMESTAMPTZ
);

CREATE INDEX submissions_queue_idx
    ON submissions (created_at)
    WHERE status = 'PENDING';

CREATE TABLE submission_logs (
    submission_id UUID NOT NULL REFERENCES submissions(id) ON DELETE CASCADE,
    stage TEXT NOT NULL CHECK (stage IN ('configure', 'compile', 'run')),
    path TEXT NOT NULL,
    PRIMARY KEY (submission_id, stage)
);

