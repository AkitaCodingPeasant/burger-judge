package authjwt

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"regs/internal/domain"
)

func TestManagerIssueAndParse(t *testing.T) {
	directory := t.TempDir()
	manager, err := New(filepath.Join(directory, "private.pem"), filepath.Join(directory, "public.pem"), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	token, err := manager.Issue(context.Background(), domain.User{ID: "user-1", Account: "alice", Role: domain.RoleAdmin})
	if err != nil {
		t.Fatal(err)
	}
	principal, err := manager.Parse(context.Background(), token)
	if err != nil {
		t.Fatal(err)
	}
	if principal.UserID != "user-1" || principal.Account != "alice" || principal.Role != domain.RoleAdmin || principal.TokenID == "" {
		t.Fatalf("unexpected principal: %#v", principal)
	}
}

func TestManagerRejectsInvalidToken(t *testing.T) {
	directory := t.TempDir()
	manager, err := New(filepath.Join(directory, "private.pem"), filepath.Join(directory, "public.pem"), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Parse(context.Background(), "not-a-token"); err == nil {
		t.Fatal("Parse() accepted an invalid token")
	}
}
