package domain

import "time"

type SubmissionStatus string

const (
	StatusPending SubmissionStatus = "PENDING"
	StatusRunning SubmissionStatus = "RUNNING"
	StatusAC      SubmissionStatus = "AC"
	StatusWA      SubmissionStatus = "WA"
	StatusSE      SubmissionStatus = "SE"
	StatusCE      SubmissionStatus = "CE"
	StatusRE      SubmissionStatus = "RE"
	StatusTLE     SubmissionStatus = "TLE"
)

func (s SubmissionStatus) Terminal() bool {
	return s != StatusPending && s != StatusRunning
}

type Submission struct {
	ID        string
	UserID    string
	ProblemID string
	Status    SubmissionStatus
	Score     int
	CreatedAt time.Time
	StartedAt *time.Time
	EndedAt   *time.Time
}

