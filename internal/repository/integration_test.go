//go:build integration

// Интеграционные тесты репозитория поднимают реальный MySQL через
// testcontainers, применяют миграции и проверяют SQL (включая три
// обязательных аналитических запроса). Запуск: go test -tags=integration ./...
package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcmysql "github.com/testcontainers/testcontainers-go/modules/mysql"

	"test-task/internal/config"
	"test-task/internal/database"
	"test-task/internal/domain"
	"test-task/internal/repository"
)

func setupDB(t *testing.T) *sqlx.DB {
	t.Helper()
	ctx := context.Background()

	ctr, err := tcmysql.Run(ctx, "mysql:8.0",
		tcmysql.WithDatabase("taskservice"),
		tcmysql.WithUsername("app"),
		tcmysql.WithPassword("app"),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = testcontainers.TerminateContainer(ctr) })

	dsn, err := ctr.ConnectionString(ctx, "parseTime=true", "charset=utf8mb4", "loc=UTC", "multiStatements=true")
	require.NoError(t, err)

	db, err := database.New(config.MySQLConfig{
		DSN:             dsn,
		MaxOpenConns:    10,
		MaxIdleConns:    10,
		ConnMaxLifetime: 5 * time.Minute,
		ConnMaxIdleTime: 5 * time.Minute,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	require.NoError(t, database.Migrate(db))
	return db
}

func createUser(t *testing.T, db *sqlx.DB, email string) int64 {
	t.Helper()
	r := repository.NewUserRepo(db)
	id, err := r.Create(context.Background(), &domain.User{Email: email, Name: email, PasswordHash: "x"})
	require.NoError(t, err)
	return id
}

func TestUserRepo_CRUD(t *testing.T) {
	db := setupDB(t)
	repo := repository.NewUserRepo(db)
	ctx := context.Background()

	id, err := repo.Create(ctx, &domain.User{Email: "a@b.com", Name: "A", PasswordHash: "h"})
	require.NoError(t, err)
	require.NotZero(t, id)

	_, err = repo.Create(ctx, &domain.User{Email: "a@b.com", Name: "Dup", PasswordHash: "h"})
	require.ErrorIs(t, err, domain.ErrConflict)

	got, err := repo.GetByEmail(ctx, "a@b.com")
	require.NoError(t, err)
	require.Equal(t, id, got.ID)

	_, err = repo.GetByEmail(ctx, "missing@b.com")
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestTeamRepo_CreateWithOwnerAndMembers(t *testing.T) {
	db := setupDB(t)
	repo := repository.NewTeamRepo(db)
	ctx := context.Background()

	owner := createUser(t, db, "owner@b.com")
	member := createUser(t, db, "member@b.com")

	teamID, err := repo.CreateWithOwner(ctx, "Team", owner)
	require.NoError(t, err)

	m, err := repo.GetMember(ctx, teamID, owner)
	require.NoError(t, err)
	require.Equal(t, domain.RoleOwner, m.Role)

	require.NoError(t, repo.AddMember(ctx, teamID, member, domain.RoleMember))
	require.ErrorIs(t, repo.AddMember(ctx, teamID, member, domain.RoleMember), domain.ErrConflict)

	teams, err := repo.ListByUser(ctx, member)
	require.NoError(t, err)
	require.Len(t, teams, 1)
}

func TestTaskRepo_CreateListUpdateHistory(t *testing.T) {
	db := setupDB(t)
	teamRepo := repository.NewTeamRepo(db)
	taskRepo := repository.NewTaskRepo(db)
	ctx := context.Background()

	owner := createUser(t, db, "owner@b.com")
	teamID, _ := teamRepo.CreateWithOwner(ctx, "Team", owner)

	id, err := taskRepo.Create(ctx, &domain.Task{
		TeamID: teamID, Title: "T1", Description: "d", Status: domain.StatusTodo, CreatedBy: owner,
	})
	require.NoError(t, err)

	task, err := taskRepo.GetByID(ctx, id)
	require.NoError(t, err)

	task.Status = domain.StatusDone
	hist := []domain.TaskHistory{{
		TaskID: id, ChangedBy: owner, Field: "status",
		OldValue: ptr("todo"), NewValue: ptr("done"),
	}}
	require.NoError(t, taskRepo.UpdateWithHistory(ctx, task, hist))

	gotHist, err := taskRepo.History(ctx, id)
	require.NoError(t, err)
	require.Len(t, gotHist, 1)
	require.Equal(t, "status", gotHist[0].Field)

	doneStatus := domain.StatusDone
	list, err := taskRepo.List(ctx, domain.TaskFilter{TeamID: teamID, Status: &doneStatus, Limit: 10})
	require.NoError(t, err)
	require.Len(t, list, 1)
}

func TestAnalytics_AllThreeQueries(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()
	teamRepo := repository.NewTeamRepo(db)
	taskRepo := repository.NewTaskRepo(db)
	analytics := repository.NewAnalyticsRepo(db)

	owner := createUser(t, db, "owner@b.com")
	member := createUser(t, db, "member@b.com")
	outsider := createUser(t, db, "outsider@b.com")

	teamID, _ := teamRepo.CreateWithOwner(ctx, "Team", owner)
	require.NoError(t, teamRepo.AddMember(ctx, teamID, member, domain.RoleMember))

	// Задача, переведённая в done за последние 7 дней (через history).
	doneTaskID, _ := taskRepo.Create(ctx, &domain.Task{TeamID: teamID, Title: "done", Description: "", Status: domain.StatusTodo, CreatedBy: owner})
	dt, _ := taskRepo.GetByID(ctx, doneTaskID)
	dt.Status = domain.StatusDone
	require.NoError(t, taskRepo.UpdateWithHistory(ctx, dt, []domain.TaskHistory{{
		TaskID: doneTaskID, ChangedBy: owner, Field: "status", OldValue: ptr("todo"), NewValue: ptr("done"),
	}}))

	// Несколько задач от owner, чтобы он лидировал в топ-создателях.
	for i := 0; i < 3; i++ {
		_, _ = taskRepo.Create(ctx, &domain.Task{TeamID: teamID, Title: "t", Description: "", Status: domain.StatusTodo, CreatedBy: owner})
	}

	// Задача с assignee = outsider (не член команды) — нарушение целостности.
	out := outsider
	_, err := taskRepo.Create(ctx, &domain.Task{TeamID: teamID, Title: "bad", Description: "", Status: domain.StatusTodo, AssigneeID: &out, CreatedBy: owner})
	require.NoError(t, err)

	// (а) TeamStats
	stats, err := analytics.TeamStats(ctx)
	require.NoError(t, err)
	require.Len(t, stats, 1)
	require.Equal(t, 2, stats[0].MemberCount)
	require.Equal(t, 1, stats[0].DoneLast7Day)

	// (б) TopCreators
	top, err := analytics.TopCreators(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, top)
	require.Equal(t, owner, top[0].UserID)
	require.Equal(t, 1, top[0].Rank)

	// (в) IntegrityIssues — должна найтись ровно одна "bad" задача.
	issues, err := analytics.IntegrityIssues(ctx)
	require.NoError(t, err)
	require.Len(t, issues, 1)
	require.Equal(t, outsider, issues[0].AssigneeID)

	_ = time.Now()
}

func ptr(s string) *string { return &s }
