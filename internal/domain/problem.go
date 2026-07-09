package domain

import "time"

// Problem 保存題目公開資訊與伺服器端題庫路徑。
type Problem struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	Description  string    `json:"description"`
	SettingsPath string    `json:"-"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// ProblemStatistics 是單題提交統計。
type ProblemStatistics struct {
	ProblemID   string `json:"problemId"`
	Submissions int    `json:"submissions"`
	Accepted    int    `json:"accepted"`
	Users       int    `json:"users"`
}

// UserStatistics 是單一使用者的提交統計。
type UserStatistics struct {
	UserID      string `json:"userId"`
	Submissions int    `json:"submissions"`
	Accepted    int    `json:"accepted"`
	Solved      int    `json:"solved"`
}
