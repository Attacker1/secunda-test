// Package repository — слой доступа к данным поверх database/sql (sqlx).
// Здесь сосредоточен весь SQL; бизнес-логики тут нет.
package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"

	"test-task/internal/database"
	"test-task/internal/domain"
)

type UserRepo struct {
	db *sqlx.DB
}

func NewUserRepo(db *sqlx.DB) *UserRepo {
	return &UserRepo{db: db}
}

func (r *UserRepo) Create(ctx context.Context, u *domain.User) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO users (email, name, password_hash) VALUES (?, ?, ?)`,
		u.Email, u.Name, u.PasswordHash)
	if err != nil {
		if database.IsDuplicate(err) {
			return 0, domain.ErrConflict
		}
		return 0, fmt.Errorf("insert user: %w", err)
	}
	return res.LastInsertId()
}

func (r *UserRepo) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	var u domain.User
	err := r.db.GetContext(ctx, &u,
		`SELECT id, email, name, password_hash, created_at FROM users WHERE email = ?`, email)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get user by email: %w", err)
	}
	return &u, nil
}

func (r *UserRepo) GetByID(ctx context.Context, id int64) (*domain.User, error) {
	var u domain.User
	err := r.db.GetContext(ctx, &u,
		`SELECT id, email, name, password_hash, created_at FROM users WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get user by id: %w", err)
	}
	return &u, nil
}
