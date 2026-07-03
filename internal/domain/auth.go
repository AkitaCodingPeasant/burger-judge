package domain

type Role string

const (
	RoleUser  Role = "user"
	RoleAdmin Role = "admin"
)

func (r Role) Allows(required Role) bool {
	return r == RoleAdmin || r == required
}

