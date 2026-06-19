// Package service содержит бизнес-логику: проверки прав, валидацию,
// формирование аудита и работу с кешем. Зависит от репозиториев через
// интерфейсы, что позволяет покрывать логику unit-тестами с моками.
package service

import (
	"context"

	"test-task/internal/domain"
)

type UserRepository interface {
	Create(ctx context.Context, u *domain.User) (int64, error)
	GetByEmail(ctx context.Context, email string) (*domain.User, error)
	GetByID(ctx context.Context, id int64) (*domain.User, error)
}

type TeamRepository interface {
	CreateWithOwner(ctx context.Context, name string, ownerID int64) (int64, error)
	ListByUser(ctx context.Context, userID int64) ([]domain.Team, error)
	GetMember(ctx context.Context, teamID, userID int64) (*domain.TeamMember, error)
	AddMember(ctx context.Context, teamID, userID int64, role domain.Role) error
}

type TaskRepository interface {
	Create(ctx context.Context, t *domain.Task) (int64, error)
	GetByID(ctx context.Context, id int64) (*domain.Task, error)
	List(ctx context.Context, f domain.TaskFilter) ([]domain.Task, error)
	UpdateWithHistory(ctx context.Context, t *domain.Task, history []domain.TaskHistory) error
	History(ctx context.Context, taskID int64) ([]domain.TaskHistory, error)
}
