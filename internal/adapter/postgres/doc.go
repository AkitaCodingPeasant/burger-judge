// Package postgres implements use-case repositories with PostgreSQL.
// ClaimPending should use SELECT ... FOR UPDATE SKIP LOCKED so multiple worker
// processes cannot claim the same submission.
package postgres

