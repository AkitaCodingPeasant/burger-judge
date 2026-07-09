package domain

import "time"

// User 是答題系統帳號；PasswordHash 僅供認證用例使用，不輸出至 API。
type User struct {
	ID           string    `json:"id"`
	Account      string    `json:"account"`
	PasswordHash string    `json:"-"`
	Role         Role      `json:"role"`
	CreatedAt    time.Time `json:"createdAt"`
}
