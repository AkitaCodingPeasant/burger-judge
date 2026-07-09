package usecase

import (
	"context"
	"errors"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"regs/internal/domain"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUnauthorized       = errors.New("authentication required")
	ErrForbidden          = errors.New("permission denied")
	ErrInvalidAccount     = errors.New("account must not be empty")
	ErrWeakPassword       = errors.New("password must contain at least 8 characters")
)

// AuthService 實作註冊、登入、JWT 驗證與 logout 撤銷。
type AuthService struct {
	users  UserRepository
	tokens TokenRepository
	jwt    TokenManager
}

func NewAuthService(users UserRepository, tokens TokenRepository, jwt TokenManager) *AuthService {
	return &AuthService{users: users, tokens: tokens, jwt: jwt}
}

func (s *AuthService) Register(ctx context.Context, account, password string) (domain.User, error) {
	account = strings.TrimSpace(account)
	if account == "" {
		return domain.User{}, ErrInvalidAccount
	}
	if len(password) < 8 {
		return domain.User{}, ErrWeakPassword
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return domain.User{}, err
	}
	id, err := newID()
	if err != nil {
		return domain.User{}, err
	}
	user := domain.User{
		ID:           id,
		Account:      account,
		PasswordHash: string(hash),
		Role:         domain.RoleUser,
	}
	if err := s.users.CreateUser(ctx, user); err != nil {
		return domain.User{}, err
	}
	return s.users.GetUserByID(ctx, user.ID)
}

func (s *AuthService) Login(ctx context.Context, account, password string) (string, domain.User, error) {
	user, err := s.users.GetUserByAccount(ctx, strings.TrimSpace(account))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return "", domain.User{}, ErrInvalidCredentials
		}
		return "", domain.User{}, err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return "", domain.User{}, ErrInvalidCredentials
	}
	token, err := s.jwt.Issue(ctx, user)
	return token, user, err
}

func (s *AuthService) Authenticate(ctx context.Context, rawToken string) (domain.Principal, error) {
	principal, err := s.jwt.Parse(ctx, rawToken)
	if err != nil {
		return domain.Principal{}, ErrUnauthorized
	}
	revoked, err := s.tokens.IsTokenRevoked(ctx, principal.TokenID)
	if err != nil {
		return domain.Principal{}, err
	}
	if revoked {
		return domain.Principal{}, ErrUnauthorized
	}
	return principal, nil
}

func (s *AuthService) Logout(ctx context.Context, principal domain.Principal) error {
	return s.tokens.RevokeToken(ctx, principal.TokenID, principal.ExpiresAt)
}

func (s *AuthService) Me(ctx context.Context, userID string) (domain.User, error) {
	return s.users.GetUserByID(ctx, userID)
}

// EnsureAdmin creates the configured administrator or refreshes its password and role.
func (s *AuthService) EnsureAdmin(ctx context.Context, account, password string) error {
	account = strings.TrimSpace(account)
	if account == "" || password == "" {
		return nil
	}
	if len(password) < 8 {
		return ErrWeakPassword
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	id, err := newID()
	if err != nil {
		return err
	}
	return s.users.EnsureAdmin(ctx, domain.User{
		ID:           id,
		Account:      account,
		PasswordHash: string(hash),
		Role:         domain.RoleAdmin,
	})
}
