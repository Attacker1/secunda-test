package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"

	"test-task/internal/domain"
)

type TaskRepo struct {
	db *sqlx.DB
}

func NewTaskRepo(db *sqlx.DB) *TaskRepo {
	return &TaskRepo{db: db}
}

func (r *TaskRepo) Create(ctx context.Context, t *domain.Task) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO tasks (team_id, title, description, status, assignee_id, created_by)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		t.TeamID, t.Title, t.Description, t.Status, t.AssigneeID, t.CreatedBy)
	if err != nil {
		return 0, fmt.Errorf("insert task: %w", err)
	}
	return res.LastInsertId()
}

func (r *TaskRepo) GetByID(ctx context.Context, id int64) (*domain.Task, error) {
	var t domain.Task
	err := r.db.GetContext(ctx, &t,
		`SELECT id, team_id, title, description, status, assignee_id, created_by, created_at, updated_at
		 FROM tasks WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get task: %w", err)
	}
	return &t, nil
}

// List применяет фильтры и пагинацию на уровне БД (LIMIT/OFFSET).
func (r *TaskRepo) List(ctx context.Context, f domain.TaskFilter) ([]domain.Task, error) {
	query := `SELECT id, team_id, title, description, status, assignee_id, created_by, created_at, updated_at
	          FROM tasks WHERE team_id = ?`
	args := []any{f.TeamID}
	if f.Status != nil {
		query += " AND status = ?"
		args = append(args, *f.Status)
	}
	if f.AssigneeID != nil {
		query += " AND assignee_id = ?"
		args = append(args, *f.AssigneeID)
	}
	query += " ORDER BY id DESC LIMIT ? OFFSET ?"
	args = append(args, f.Limit, f.Offset)

	tasks := []domain.Task{}
	if err := r.db.SelectContext(ctx, &tasks, query, args...); err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	return tasks, nil
}

// UpdateWithHistory обновляет задачу и пишет записи аудита в одной транзакции.
func (r *TaskRepo) UpdateWithHistory(ctx context.Context, t *domain.Task, history []domain.TaskHistory) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.ExecContext(ctx,
		`UPDATE tasks SET title = ?, description = ?, status = ?, assignee_id = ?
		 WHERE id = ?`,
		t.Title, t.Description, t.Status, t.AssigneeID, t.ID)
	if err != nil {
		return fmt.Errorf("update task: %w", err)
	}

	for _, h := range history {
		_, err = tx.ExecContext(ctx,
			`INSERT INTO task_history (task_id, changed_by, field, old_value, new_value)
			 VALUES (?, ?, ?, ?, ?)`,
			h.TaskID, h.ChangedBy, h.Field, h.OldValue, h.NewValue)
		if err != nil {
			return fmt.Errorf("insert history: %w", err)
		}
	}
	return tx.Commit()
}

func (r *TaskRepo) History(ctx context.Context, taskID int64) ([]domain.TaskHistory, error) {
	items := []domain.TaskHistory{}
	err := r.db.SelectContext(ctx, &items,
		`SELECT id, task_id, changed_by, field, old_value, new_value, changed_at
		 FROM task_history WHERE task_id = ? ORDER BY changed_at DESC, id DESC`, taskID)
	if err != nil {
		return nil, fmt.Errorf("task history: %w", err)
	}
	return items, nil
}
