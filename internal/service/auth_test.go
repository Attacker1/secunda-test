package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"test-task/internal/domain"
)

func newAuth() (*AuthService, *fakeUserRepo) {
	users := newFakeUserRepo()
	tm := NewTokenManager("test-secret", time.Hour)
	return NewAuthService(users, tm), users
}

func TestRegister_Success(t *testing.T) {
	svc, _ := newAuth()
	u, err := svc.Register(context.Background(), RegisterInput{
		Email: "Alice@Example.com", Name: "Alice", Password: "supersecret",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u.ID == 0 {
		t.Fatal("expected assigned id")
	}
	if u.Email != "alice@example.com" {
		t.Fatalf("email should be normalized, got %q", u.Email)
	}
	if u.PasswordHash == "supersecret" || u.PasswordHash == "" {
		t.Fatal("password must be hashed")
	}
}

func TestRegister_Validation(t *testing.T) {
	svc, _ := newAuth()
	cases := []struct {
		name string
		in   RegisterInput
	}{
		{"empty email", RegisterInput{Email: "", Name: "A", Password: "supersecret"}},
		{"bad email", RegisterInput{Email: "not-an-email", Name: "A", Password: "supersecret"}},
		{"empty name", RegisterInput{Email: "a@b.com", Name: "  ", Password: "supersecret"}},
		{"short password", RegisterInput{Email: "a@b.com", Name: "A", Password: "short"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := svc.Register(context.Background(), c.in)
			if !errors.Is(err, domain.ErrValidation) {
				t.Fatalf("expected validation error, got %v", err)
			}
		})
	}
}

func TestRegister_DuplicateEmail(t *testing.T) {
	svc, _ := newAuth()
	in := RegisterInput{Email: "dup@example.com", Name: "Dup", Password: "supersecret"}
	if _, err := svc.Register(context.Background(), in); err != nil {
		t.Fatalf("first register failed: %v", err)
	}
	_, err := svc.Register(context.Background(), in)
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
}

func TestLogin_Success(t *testing.T) {
	svc, _ := newAuth()
	_, _ = svc.Register(context.Background(), RegisterInput{
		Email: "bob@example.com", Name: "Bob", Password: "supersecret",
	})
	token, u, err := svc.Login(context.Background(), "bob@example.com", "supersecret")
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}
	if token == "" || u == nil {
		t.Fatal("expected token and user")
	}
	// Токен должен валидироваться и содержать корректный userID.
	tm := NewTokenManager("test-secret", time.Hour)
	uid, err := tm.Parse(token)
	if err != nil || uid != u.ID {
		t.Fatalf("token parse mismatch: uid=%d err=%v", uid, err)
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	svc, _ := newAuth()
	_, _ = svc.Register(context.Background(), RegisterInput{
		Email: "carol@example.com", Name: "Carol", Password: "supersecret",
	})
	_, _, err := svc.Login(context.Background(), "carol@example.com", "wrongpass")
	if !errors.Is(err, domain.ErrInvalidCredential) {
		t.Fatalf("expected invalid credential, got %v", err)
	}
}

func TestLogin_UnknownUser(t *testing.T) {
	svc, _ := newAuth()
	_, _, err := svc.Login(context.Background(), "ghost@example.com", "whatever")
	if !errors.Is(err, domain.ErrInvalidCredential) {
		t.Fatalf("expected invalid credential (no user enumeration), got %v", err)
	}
}

func TestTokenManager_InvalidSignature(t *testing.T) {
	a := NewTokenManager("secret-a", time.Hour)
	b := NewTokenManager("secret-b", time.Hour)
	tok, _ := a.Generate(7)
	if _, err := b.Parse(tok); err == nil {
		t.Fatal("expected signature verification to fail")
	}
}

func TestTokenManager_Expired(t *testing.T) {
	tm := NewTokenManager("secret", -time.Minute) // уже истёк
	tok, _ := tm.Generate(1)
	if _, err := tm.Parse(tok); err == nil {
		t.Fatal("expected expired token error")
	}
}
