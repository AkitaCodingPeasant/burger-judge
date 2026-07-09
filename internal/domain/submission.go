package domain

import "time"

// SubmissionStatus 表示判題生命週期中的一個狀態。
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

// Terminal 判斷狀態是否為已完成的合法判題結果。
func (s SubmissionStatus) Terminal() bool {
	switch s {
	case StatusAC, StatusWA, StatusSE, StatusCE, StatusRE, StatusTLE:
		return true
	default:
		return false
	}
}

// Submission 保存單次提交的中繼資料與判題結果。
type Submission struct {
	ID        string           `json:"operatorId"`
	UserID    string           `json:"userId"`
	ProblemID string           `json:"problemId"`
	Status    SubmissionStatus `json:"status"`
	Score     int              `json:"score"`
	CreatedAt time.Time        `json:"createdAt"`
	StartedAt *time.Time       `json:"startedAt,omitempty"`
	EndedAt   *time.Time       `json:"endedAt,omitempty"`
}
