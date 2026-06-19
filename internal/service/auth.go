package service

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"test-task/internal/domain"
)

type AuthService struct {
	users  UserRepository
	tokens *TokenManager
}

func NewAuthService(users UserRepository, tokens *TokenManager) *AuthService {
	return &AuthService{users: users, tokens: tokens}
}

type RegisterInput struct {
	Email    string
	Name     string
	Password string
}

func (s *AuthService) Register(ctx context.Context, in RegisterInput) (*domain.User, error) {
	in.Email = strings.TrimSpace(strings.ToLower(in.Email))
	in.Name = strings.TrimSpace(in.Name)
	if err := validateEmail(in.Email); err != nil {
		return nil, err
	}
	if in.Name == "" {
		return nil, fmt.Errorf("%w: name is required", domain.ErrValidation)
	}
	if len(in.Password) < 8 {
		return nil, fmt.Errorf("%w: password must be at least 8 characters", domain.ErrValidation)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}
	u := &domain.User{Email: in.Email, Name: in.Name, PasswordHash: string(hash)}
	id, err := s.users.Create(ctx, u)
	if err != nil {
		return nil, err
	}
	u.ID = id
	return u, nil
}

func (s *AuthService) Login(ctx context.Context, email, password string) (string, *domain.User, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	u, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			// Не раскрываем, что именно неверно — email или пароль.
			return "", nil, domain.ErrInvalidCredential
		}
		return "", nil, err
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) != nil {
		return "", nil, domain.ErrInvalidCredential
	}
	token, err := s.tokens.Generate(u.ID)
	if err != nil {
		return "", nil, fmt.Errorf("generate token: %w", err)
	}
	return token, u, nil
}

func validateEmail(email string) error {
	if email == "" {
		return fmt.Errorf("%w: email is required", domain.ErrValidation)
	}
	if _, err := mail.ParseAddress(email); err != nil {
		return fmt.Errorf("%w: invalid email", domain.ErrValidation)
	}
	return nil
}
