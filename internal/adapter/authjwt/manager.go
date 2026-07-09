package authjwt

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"regs/internal/domain"
)

type claims struct {
	Account string      `json:"account"`
	Role    domain.Role `json:"role"`
	jwt.RegisteredClaims
}

// Manager 使用 ES256 簽發並驗證 JWT。
type Manager struct {
	privateKey *ecdsa.PrivateKey
	publicKey  *ecdsa.PublicKey
	ttl        time.Duration
}

// New 載入 P-256 PEM；兩個檔案都不存在時會建立本機開發金鑰。
func New(privatePath, publicPath string, ttl time.Duration) (*Manager, error) {
	if err := ensureKeys(privatePath, publicPath); err != nil {
		return nil, err
	}
	privateKey, err := readPrivateKey(privatePath)
	if err != nil {
		return nil, err
	}
	publicKey, err := readPublicKey(publicPath)
	if err != nil {
		return nil, err
	}
	return &Manager{privateKey: privateKey, publicKey: publicKey, ttl: ttl}, nil
}

func (m *Manager) Issue(ctx context.Context, user domain.User) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	now := time.Now().UTC()
	id, err := randomID()
	if err != nil {
		return "", err
	}
	value := claims{
		Account: user.Account,
		Role:    user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "regs",
			Subject:   user.ID,
			ID:        id,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.ttl)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodES256, value).SignedString(m.privateKey)
}

func (m *Manager) Parse(ctx context.Context, raw string) (domain.Principal, error) {
	if err := ctx.Err(); err != nil {
		return domain.Principal{}, err
	}
	value := &claims{}
	token, err := jwt.ParseWithClaims(raw, value, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodES256 {
			return nil, errors.New("unexpected JWT signing method")
		}
		return m.publicKey, nil
	}, jwt.WithIssuer("regs"), jwt.WithValidMethods([]string{"ES256"}))
	if err != nil || !token.Valid || value.Subject == "" || value.ID == "" || value.ExpiresAt == nil {
		return domain.Principal{}, errors.New("invalid JWT")
	}
	return domain.Principal{
		UserID:    value.Subject,
		Account:   value.Account,
		Role:      value.Role,
		TokenID:   value.ID,
		ExpiresAt: value.ExpiresAt.Time,
	}, nil
}

func ensureKeys(privatePath, publicPath string) error {
	_, privateErr := os.Stat(privatePath)
	_, publicErr := os.Stat(publicPath)
	if privateErr == nil && publicErr == nil {
		return nil
	}
	if !errors.Is(privateErr, os.ErrNotExist) || !errors.Is(publicErr, os.ErrNotExist) {
		return fmt.Errorf("JWT key pair is incomplete")
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	privateDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return err
	}
	publicDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(privatePath), 0o700); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(publicPath), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(privatePath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER}), 0o600); err != nil {
		return err
	}
	return os.WriteFile(publicPath, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER}), 0o644)
}

func readPrivateKey(path string) (*ecdsa.PrivateKey, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(content)
	if block == nil {
		return nil, errors.New("invalid JWT private PEM")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	key, ok := parsed.(*ecdsa.PrivateKey)
	if !ok {
		return nil, errors.New("JWT private key is not EC")
	}
	return key, nil
}

func readPublicKey(path string) (*ecdsa.PublicKey, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(content)
	if block == nil {
		return nil, errors.New("invalid JWT public PEM")
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	key, ok := parsed.(*ecdsa.PublicKey)
	if !ok {
		return nil, errors.New("JWT public key is not EC")
	}
	return key, nil
}

func randomID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}
