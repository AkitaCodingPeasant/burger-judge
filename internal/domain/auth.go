package domain

import "time"

// Role 表示應用程式層級的使用者角色。
type Role string

const (
	RoleUser  Role = "user"
	RoleAdmin Role = "admin"
)

// Allows 判斷目前角色是否符合指定的權限需求。
func (r Role) Allows(required Role) bool {
	if required != RoleUser && required != RoleAdmin {
		return false
	}
	return r == RoleAdmin || r == required
}

// Principal 是 JWT 驗證後放入 request context 的身分資訊。
type Principal struct {
	UserID    string
	Account   string
	Role      Role
	TokenID   string
	ExpiresAt time.Time
}
