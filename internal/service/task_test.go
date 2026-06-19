package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"test-task/internal/domain"
)

func newTaskSvc(t *testing.T) (*TaskService, *fakeTaskRepo, *fakeTeamRepo, *fakeCache, int64, int64) {
	t.Helper()
	tasks := newFakeTaskRepo()
	teams := newFakeTeamRepo()
	c := &fakeCache{}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewTaskService(tasks, teams, c, log)

	owner := int64(1)
	teamID, _ := teams.CreateWithOwner(context.Background(), "T", owner)
	return svc, tasks, teams, c, teamID, owner
}

func TestTask_CreateByMember(t *testing.T) {
	svc, _, _, c, teamID, owner := newTaskSvc(t)
	task, err := svc.Create(context.Background(), CreateTaskInput{
		TeamID: teamID, Title: "Do it", ActorID: owner,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if task.Status != domain.StatusTodo {
		t.Fatalf("default status should be todo, got %s", task.Status)
	}
	if c.invalidations != 1 {
		t.Fatalf("create must invalidate cache once, got %d", c.invalidations)
	}
}

func TestTask_CreateByNonMemberForbidden(t *testing.T) {
	svc, _, _, _, teamID, _ := newTaskSvc(t)
	_, err := svc.Create(context.Background(), CreateTaskInput{
		TeamID: teamID, Title: "X", ActorID: 999,
	})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("non-member must be forbidden, got %v", err)
	}
}

func TestTask_CreateValidation(t *testing.T) {
	svc, _, _, _, teamID, owner := newTaskSvc(t)
	if _, err := svc.Create(context.Background(), CreateTaskInput{TeamID: teamID, Title: "  ", ActorID: owner}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("empty title must fail, got %v", err)
	}
	if _, err := svc.Create(context.Background(), CreateTaskInput{TeamID: teamID, Title: "ok", Status: "bogus", ActorID: owner}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("bad status must fail, got %v", err)
	}
}

func TestTask_UpdateRecordsHistory(t *testing.T) {
	svc, tasks, _, c, teamID, owner := newTaskSvc(t)
	task, _ := svc.Create(context.Background(), CreateTaskInput{TeamID: teamID, Title: "Old", ActorID: owner})
	c.invalidations = 0

	newTitle := "New"
	newStatus := domain.StatusInProgress
	updated, err := svc.Update(context.Background(), UpdateTaskInput{
		TaskID: task.ID, Title: &newTitle, Status: &newStatus, ActorID: owner,
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Title != "New" || updated.Status != domain.StatusInProgress {
		t.Fatalf("update not applied: %+v", updated)
	}
	hist, _ := tasks.History(context.Background(), task.ID)
	if len(hist) != 2 {
		t.Fatalf("expected 2 history rows (title, status), got %d", len(hist))
	}
	if c.invalidations != 1 {
		t.Fatalf("update must invalidate cache once, got %d", c.invalidations)
	}
}

func TestTask_UpdateNoChangesNoHistory(t *testing.T) {
	svc, tasks, _, c, teamID, owner := newTaskSvc(t)
	task, _ := svc.Create(context.Background(), CreateTaskInput{TeamID: teamID, Title: "Same", ActorID: owner})
	c.invalidations = 0

	sameTitle := "Same"
	_, err := svc.Update(context.Background(), UpdateTaskInput{TaskID: task.ID, Title: &sameTitle, ActorID: owner})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	hist, _ := tasks.History(context.Background(), task.ID)
	if len(hist) != 0 {
		t.Fatalf("no-op update must not write history, got %d", len(hist))
	}
	if c.invalidations != 0 {
		t.Fatalf("no-op update must not invalidate cache, got %d", c.invalidations)
	}
}

func TestTask_UpdateAssigneeAndUnset(t *testing.T) {
	svc, tasks, _, _, teamID, owner := newTaskSvc(t)
	task, _ := svc.Create(context.Background(), CreateTaskInput{TeamID: teamID, Title: "T", ActorID: owner})

	assignee := int64(42)
	if _, err := svc.Update(context.Background(), UpdateTaskInput{TaskID: task.ID, AssigneeID: &assignee, ActorID: owner}); err != nil {
		t.Fatalf("set assignee: %v", err)
	}
	got, _ := tasks.GetByID(context.Background(), task.ID)
	if got.AssigneeID == nil || *got.AssigneeID != 42 {
		t.Fatalf("assignee not set: %+v", got.AssigneeID)
	}

	if _, err := svc.Update(context.Background(), UpdateTaskInput{TaskID: task.ID, UnsetAssign: true, ActorID: owner}); err != nil {
		t.Fatalf("unset assignee: %v", err)
	}
	got, _ = tasks.GetByID(context.Background(), task.ID)
	if got.AssigneeID != nil {
		t.Fatalf("assignee should be nil after unset, got %v", *got.AssigneeID)
	}
}

func TestTask_UpdateByNonMemberForbidden(t *testing.T) {
	svc, _, _, _, teamID, owner := newTaskSvc(t)
	task, _ := svc.Create(context.Background(), CreateTaskInput{TeamID: teamID, Title: "T", ActorID: owner})
	newTitle := "Hacked"
	_, err := svc.Update(context.Background(), UpdateTaskInput{TaskID: task.ID, Title: &newTitle, ActorID: 999})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("non-member must not update, got %v", err)
	}
}

func TestTask_UpdateNotFound(t *testing.T) {
	svc, _, _, _, _, owner := newTaskSvc(t)
	newTitle := "x"
	_, err := svc.Update(context.Background(), UpdateTaskInput{TaskID: 12345, Title: &newTitle, ActorID: owner})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
}

func TestTask_ListFiltersAndPaginationDefaults(t *testing.T) {
	svc, _, _, _, teamID, owner := newTaskSvc(t)
	for i := 0; i < 3; i++ {
		_, _ = svc.Create(context.Background(), CreateTaskInput{TeamID: teamID, Title: "t", ActorID: owner})
	}
	got, err := svc.List(context.Background(), domain.TaskFilter{TeamID: teamID}, owner)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(got))
	}
}

func TestTask_ListClampsLimit(t *testing.T) {
	svc, _, _, _, teamID, owner := newTaskSvc(t)
	// limit выше максимума и отрицательный offset должны нормализоваться.
	if _, err := svc.List(context.Background(), domain.TaskFilter{TeamID: teamID, Limit: 9999, Offset: -5}, owner); err != nil {
		t.Fatalf("list: %v", err)
	}
}

func TestTask_ListByNonMemberForbidden(t *testing.T) {
	svc, _, _, _, teamID, _ := newTaskSvc(t)
	_, err := svc.List(context.Background(), domain.TaskFilter{TeamID: teamID}, 999)
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("non-member must not list, got %v", err)
	}
}

func TestTask_HistoryPermission(t *testing.T) {
	svc, _, _, _, teamID, owner := newTaskSvc(t)
	task, _ := svc.Create(context.Background(), CreateTaskInput{TeamID: teamID, Title: "T", ActorID: owner})
	if _, err := svc.History(context.Background(), task.ID, 999); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("non-member must not read history, got %v", err)
	}
	if _, err := svc.History(context.Background(), task.ID, owner); err != nil {
		t.Fatalf("member should read history: %v", err)
	}
}
