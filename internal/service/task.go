package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"test-task/internal/cache"
	"test-task/internal/domain"
)

const (
	defaultLimit = 20
	maxLimit     = 100
)

type TaskService struct {
	tasks TaskRepository
	teams TeamRepository
	cache cache.Cache
	log   *slog.Logger
}

func NewTaskService(tasks TaskRepository, teams TeamRepository, c cache.Cache, log *slog.Logger) *TaskService {
	return &TaskService{tasks: tasks, teams: teams, cache: c, log: log}
}

type CreateTaskInput struct {
	TeamID      int64
	Title       string
	Description string
	Status      domain.TaskStatus
	AssigneeID  *int64
	ActorID     int64
}

func (s *TaskService) Create(ctx context.Context, in CreateTaskInput) (*domain.Task, error) {
	if err := s.requireMember(ctx, in.TeamID, in.ActorID); err != nil {
		return nil, err
	}
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" {
		return nil, fmt.Errorf("%w: title is required", domain.ErrValidation)
	}
	if in.Status == "" {
		in.Status = domain.StatusTodo
	}
	if !in.Status.Valid() {
		return nil, fmt.Errorf("%w: invalid status", domain.ErrValidation)
	}

	t := &domain.Task{
		TeamID:      in.TeamID,
		Title:       in.Title,
		Description: in.Description,
		Status:      in.Status,
		AssigneeID:  in.AssigneeID,
		CreatedBy:   in.ActorID,
	}
	id, err := s.tasks.Create(ctx, t)
	if err != nil {
		return nil, err
	}
	t.ID = id
	s.invalidate(ctx, in.TeamID)
	return t, nil
}

// List возвращает задачи команды с фильтрами и пагинацией. Результат
// кешируется в Redis на TTL (см. cache). Доступно только членам команды.
func (s *TaskService) List(ctx context.Context, f domain.TaskFilter, actorID int64) ([]domain.Task, error) {
	if err := s.requireMember(ctx, f.TeamID, actorID); err != nil {
		return nil, err
	}
	normalizeFilter(&f)

	key := cache.TasksKey(f)
	if cached, ok, err := s.cache.GetTasks(ctx, key); err != nil {
		s.log.Warn("cache get failed", "err", err)
	} else if ok {
		return cached, nil
	}

	tasks, err := s.tasks.List(ctx, f)
	if err != nil {
		return nil, err
	}
	if err := s.cache.SetTasks(ctx, key, tasks); err != nil {
		s.log.Warn("cache set failed", "err", err)
	}
	return tasks, nil
}

type UpdateTaskInput struct {
	TaskID      int64
	Title       *string
	Description *string
	Status      *domain.TaskStatus
	AssigneeID  *int64
	UnsetAssign bool // явно снять исполнителя (assignee_id -> NULL)
	ActorID     int64
}

// Update применяет частичное обновление задачи, фиксирует изменения в
// task_history (в одной транзакции с UPDATE) и сбрасывает кеш команды.
func (s *TaskService) Update(ctx context.Context, in UpdateTaskInput) (*domain.Task, error) {
	current, err := s.tasks.GetByID(ctx, in.TaskID)
	if err != nil {
		return nil, err
	}
	if err := s.requireMember(ctx, current.TeamID, in.ActorID); err != nil {
		return nil, err
	}

	updated := *current
	var history []domain.TaskHistory
	add := func(field, oldV, newV string) {
		o, n := oldV, newV
		history = append(history, domain.TaskHistory{
			TaskID: current.ID, ChangedBy: in.ActorID, Field: field,
			OldValue: &o, NewValue: &n,
		})
	}

	if in.Title != nil {
		nt := strings.TrimSpace(*in.Title)
		if nt == "" {
			return nil, fmt.Errorf("%w: title cannot be empty", domain.ErrValidation)
		}
		if nt != current.Title {
			add("title", current.Title, nt)
			updated.Title = nt
		}
	}
	if in.Description != nil && *in.Description != current.Description {
		add("description", current.Description, *in.Description)
		updated.Description = *in.Description
	}
	if in.Status != nil {
		if !in.Status.Valid() {
			return nil, fmt.Errorf("%w: invalid status", domain.ErrValidation)
		}
		if *in.Status != current.Status {
			add("status", string(current.Status), string(*in.Status))
			updated.Status = *in.Status
		}
	}
	if in.UnsetAssign {
		if current.AssigneeID != nil {
			add("assignee_id", strconv.FormatInt(*current.AssigneeID, 10), "")
			updated.AssigneeID = nil
		}
	} else if in.AssigneeID != nil {
		if current.AssigneeID == nil || *current.AssigneeID != *in.AssigneeID {
			oldV := ""
			if current.AssigneeID != nil {
				oldV = strconv.FormatInt(*current.AssigneeID, 10)
			}
			add("assignee_id", oldV, strconv.FormatInt(*in.AssigneeID, 10))
			updated.AssigneeID = in.AssigneeID
		}
	}

	if len(history) == 0 {
		return current, nil // нечего менять
	}
	if err := s.tasks.UpdateWithHistory(ctx, &updated, history); err != nil {
		return nil, err
	}
	s.invalidate(ctx, current.TeamID)
	return &updated, nil
}

func (s *TaskService) History(ctx context.Context, taskID, actorID int64) ([]domain.TaskHistory, error) {
	t, err := s.tasks.GetByID(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if err := s.requireMember(ctx, t.TeamID, actorID); err != nil {
		return nil, err
	}
	return s.tasks.History(ctx, taskID)
}

func (s *TaskService) requireMember(ctx context.Context, teamID, userID int64) error {
	_, err := s.teams.GetMember(ctx, teamID, userID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return fmt.Errorf("%w: not a member of the team", domain.ErrForbidden)
		}
		return err
	}
	return nil
}

func (s *TaskService) invalidate(ctx context.Context, teamID int64) {
	if err := s.cache.InvalidateTeam(ctx, teamID); err != nil {
		s.log.Warn("cache invalidation failed", "team_id", teamID, "err", err)
	}
}

func normalizeFilter(f *domain.TaskFilter) {
	if f.Limit <= 0 {
		f.Limit = defaultLimit
	}
	if f.Limit > maxLimit {
		f.Limit = maxLimit
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
}
