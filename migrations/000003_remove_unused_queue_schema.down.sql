CREATE INDEX submissions_queue_idx
    ON submissions (created_at)
    WHERE status = 'PENDING';

CREATE TABLE submission_logs (
    submission_id UUID NOT NULL REFERENCES submissions(id) ON DELETE CASCADE,
    stage TEXT NOT NULL CHECK (stage IN ('configure', 'compile', 'run')),
    path TEXT NOT NULL,
    PRIMARY KEY (submission_id, stage)
);
