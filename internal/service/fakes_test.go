package service

import (
	"context"
	"sync"

	"test-task/internal/domain"
)

// Внутрипамятные фейки репозиториев и зависимостей для unit-тестов.

type fakeUserRepo struct {
	mu      sync.Mutex
	byID    map[int64]*domain.User
	byEmail map[string]*domain.User
	nextID  int64
	failDup bool
}

func newFakeUserRepo() *fakeUserRepo {
	return &fakeUserRepo{byID: map[int64]*domain.User{}, byEmail: map[string]*domain.User{}}
}

func (f *fakeUserRepo) Create(_ context.Context, u *domain.User) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.byEmail[u.Email]; ok {
		return 0, domain.ErrConflict
	}
	f.nextID++
	cp := *u
	cp.ID = f.nextID
	f.byID[cp.ID] = &cp
	f.byEmail[cp.Email] = &cp
	return cp.ID, nil
}

func (f *fakeUserRepo) GetByEmail(_ context.Context, email string) (*domain.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if u, ok := f.byEmail[email]; ok {
		return u, nil
	}
	return nil, domain.ErrNotFound
}

func (f *fakeUserRepo) GetByID(_ context.Context, id int64) (*domain.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if u, ok := f.byID[id]; ok {
		return u, nil
	}
	return nil, domain.ErrNotFound
}

type memberKey struct{ team, user int64 }

type fakeTeamRepo struct {
	mu      sync.Mutex
	members map[memberKey]*domain.TeamMember
	teams   map[int64]*domain.Team
	nextID  int64
}

func newFakeTeamRepo() *fakeTeamRepo {
	return &fakeTeamRepo{members: map[memberKey]*domain.TeamMember{}, teams: map[int64]*domain.Team{}}
}

func (f *fakeTeamRepo) CreateWithOwner(_ context.Context, name string, ownerID int64) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	id := f.nextID
	f.teams[id] = &domain.Team{ID: id, Name: name, CreatedBy: ownerID}
	f.members[memberKey{id, ownerID}] = &domain.TeamMember{TeamID: id, UserID: ownerID, Role: domain.RoleOwner}
	return id, nil
}

func (f *fakeTeamRepo) ListByUser(_ context.Context, userID int64) ([]domain.Team, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []domain.Team
	for k := range f.members {
		if k.user == userID {
			out = append(out, *f.teams[k.team])
		}
	}
	return out, nil
}

func (f *fakeTeamRepo) GetMember(_ context.Context, teamID, userID int64) (*domain.TeamMember, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if m, ok := f.members[memberKey{teamID, userID}]; ok {
		return m, nil
	}
	return nil, domain.ErrNotFound
}

func (f *fakeTeamRepo) AddMember(_ context.Context, teamID, userID int64, role domain.Role) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := memberKey{teamID, userID}
	if _, ok := f.members[k]; ok {
		return domain.ErrConflict
	}
	f.members[k] = &domain.TeamMember{TeamID: teamID, UserID: userID, Role: role}
	return nil
}

type fakeTaskRepo struct {
	mu      sync.Mutex
	tasks   map[int64]*domain.Task
	history map[int64][]domain.TaskHistory
	nextID  int64
}

func newFakeTaskRepo() *fakeTaskRepo {
	return &fakeTaskRepo{tasks: map[int64]*domain.Task{}, history: map[int64][]domain.TaskHistory{}}
}

func (f *fakeTaskRepo) Create(_ context.Context, t *domain.Task) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	cp := *t
	cp.ID = f.nextID
	f.tasks[cp.ID] = &cp
	return cp.ID, nil
}

func (f *fakeTaskRepo) GetByID(_ context.Context, id int64) (*domain.Task, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if t, ok := f.tasks[id]; ok {
		cp := *t
		return &cp, nil
	}
	return nil, domain.ErrNotFound
}

func (f *fakeTaskRepo) List(_ context.Context, fl domain.TaskFilter) ([]domain.Task, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []domain.Task{}
	for _, t := range f.tasks {
		if t.TeamID != fl.TeamID {
			continue
		}
		if fl.Status != nil && t.Status != *fl.Status {
			continue
		}
		if fl.AssigneeID != nil && (t.AssigneeID == nil || *t.AssigneeID != *fl.AssigneeID) {
			continue
		}
		out = append(out, *t)
	}
	return out, nil
}

func (f *fakeTaskRepo) UpdateWithHistory(_ context.Context, t *domain.Task, h []domain.TaskHistory) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *t
	f.tasks[t.ID] = &cp
	f.history[t.ID] = append(f.history[t.ID], h...)
	return nil
}

func (f *fakeTaskRepo) History(_ context.Context, taskID int64) ([]domain.TaskHistory, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.history[taskID], nil
}

// fakeCache — кеш-заглушка, всегда мимо (cache miss), считает инвалидации.
type fakeCache struct {
	invalidations int
}

func (f *fakeCache) GetTasks(context.Context, string) ([]domain.Task, bool, error) {
	return nil, false, nil
}
func (f *fakeCache) SetTasks(context.Context, string, []domain.Task) error { return nil }
func (f *fakeCache) InvalidateTeam(context.Context, int64) error {
	f.invalidations++
	return nil
}

// fakeMailer — отправитель писем с управляемым поведением.
type fakeMailer struct {
	calls int
	err   error
}

func (f *fakeMailer) SendInvite(context.Context, string, string) error {
	f.calls++
	return f.err
}
