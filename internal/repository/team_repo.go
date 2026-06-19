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

type TeamRepo struct {
	db *sqlx.DB
}

func NewTeamRepo(db *sqlx.DB) *TeamRepo {
	return &TeamRepo{db: db}
}

// CreateWithOwner создаёт команду и в той же транзакции добавляет создателя
// как owner. Возвращает id новой команды.
func (r *TeamRepo) CreateWithOwner(ctx context.Context, name string, ownerID int64) (int64, error) {
	var teamID int64
	err := r.withTx(ctx, func(tx *sqlx.Tx) error {
		res, err := tx.ExecContext(ctx,
			`INSERT INTO teams (name, created_by) VALUES (?, ?)`, name, ownerID)
		if err != nil {
			return fmt.Errorf("insert team: %w", err)
		}
		teamID, err = res.LastInsertId()
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx,
			`INSERT INTO team_members (team_id, user_id, role) VALUES (?, ?, 'owner')`,
			teamID, ownerID)
		if err != nil {
			return fmt.Errorf("insert owner membership: %w", err)
		}
		return nil
	})
	return teamID, err
}

func (r *TeamRepo) ListByUser(ctx context.Context, userID int64) ([]domain.Team, error) {
	var teams []domain.Team
	err := r.db.SelectContext(ctx, &teams,
		`SELECT t.id, t.name, t.created_by, t.created_at
		 FROM teams t
		 JOIN team_members tm ON tm.team_id = t.id
		 WHERE tm.user_id = ?
		 ORDER BY t.created_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list teams by user: %w", err)
	}
	return teams, nil
}

func (r *TeamRepo) GetMember(ctx context.Context, teamID, userID int64) (*domain.TeamMember, error) {
	var m domain.TeamMember
	err := r.db.GetContext(ctx, &m,
		`SELECT team_id, user_id, role, joined_at
		 FROM team_members WHERE team_id = ? AND user_id = ?`, teamID, userID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get team member: %w", err)
	}
	return &m, nil
}

func (r *TeamRepo) AddMember(ctx context.Context, teamID, userID int64, role domain.Role) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO team_members (team_id, user_id, role) VALUES (?, ?, ?)`,
		teamID, userID, role)
	if err != nil {
		if database.IsDuplicate(err) {
			return domain.ErrConflict
		}
		return fmt.Errorf("add team member: %w", err)
	}
	return nil
}

func (r *TeamRepo) withTx(ctx context.Context, fn func(tx *sqlx.Tx) error) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}
