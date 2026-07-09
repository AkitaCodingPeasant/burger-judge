DROP INDEX IF EXISTS submissions_problem_status_idx;
DROP INDEX IF EXISTS submissions_user_created_idx;
DROP TABLE IF EXISTS revoked_tokens;
ALTER TABLE problems DROP COLUMN IF EXISTS description;
