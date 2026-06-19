package service

import (
	"context"
	"errors"
	"testing"

	"test-task/internal/domain"
)

func seedUser(t *testing.T, users *fakeUserRepo, email string) int64 {
	t.Helper()
	id, err := users.Create(context.Background(), &domain.User{Email: email, Name: email})
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return id
}

func TestTeam_CreateMakesOwner(t *testing.T) {
	teams := newFakeTeamRepo()
	users := newFakeUserRepo()
	svc := NewTeamService(teams, users, &fakeMailer{})

	owner := seedUser(t, users, "owner@example.com")
	team, err := svc.Create(context.Background(), "Engineering", owner)
	if err != nil {
		t.Fatalf("create team: %v", err)
	}
	m, err := teams.GetMember(context.Background(), team.ID, owner)
	if err != nil || m.Role != domain.RoleOwner {
		t.Fatalf("creator must be owner, got %+v err=%v", m, err)
	}
}

func TestTeam_ListForUser(t *testing.T) {
	teams := newFakeTeamRepo()
	users := newFakeUserRepo()
	svc := NewTeamService(teams, users, &fakeMailer{})

	owner := seedUser(t, users, "owner@example.com")
	if _, err := svc.Create(context.Background(), "A", owner); err != nil {
		t.Fatalf("create A: %v", err)
	}
	if _, err := svc.Create(context.Background(), "B", owner); err != nil {
		t.Fatalf("create B: %v", err)
	}
	got, err := svc.ListForUser(context.Background(), owner)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 teams, got %d", len(got))
	}
}

func TestTeam_CreateValidation(t *testing.T) {
	svc := NewTeamService(newFakeTeamRepo(), newFakeUserRepo(), &fakeMailer{})
	if _, err := svc.Create(context.Background(), "   ", 1); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected validation error, got %v", err)
	}
}

func TestTeam_InviteByOwner(t *testing.T) {
	teams := newFakeTeamRepo()
	users := newFakeUserRepo()
	mailer := &fakeMailer{}
	svc := NewTeamService(teams, users, mailer)

	owner := seedUser(t, users, "owner@example.com")
	invitee := seedUser(t, users, "newbie@example.com")
	_ = invitee
	team, _ := svc.Create(context.Background(), "Team", owner)

	err := svc.Invite(context.Background(), InviteInput{
		TeamID: team.ID, InviterID: owner, Email: "newbie@example.com", Role: domain.RoleMember,
	})
	if err != nil {
		t.Fatalf("invite failed: %v", err)
	}
	if mailer.calls != 1 {
		t.Fatalf("expected one email send, got %d", mailer.calls)
	}
}

func TestTeam_InviteForbiddenForMember(t *testing.T) {
	teams := newFakeTeamRepo()
	users := newFakeUserRepo()
	svc := NewTeamService(teams, users, &fakeMailer{})

	owner := seedUser(t, users, "owner@example.com")
	member := seedUser(t, users, "member@example.com")
	target := seedUser(t, users, "target@example.com")
	_ = target
	team, _ := svc.Create(context.Background(), "Team", owner)
	_ = teams.AddMember(context.Background(), team.ID, member, domain.RoleMember)

	err := svc.Invite(context.Background(), InviteInput{
		TeamID: team.ID, InviterID: member, Email: "target@example.com",
	})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("member must not be allowed to invite, got %v", err)
	}
}

func TestTeam_InviteByNonMember(t *testing.T) {
	teams := newFakeTeamRepo()
	users := newFakeUserRepo()
	svc := NewTeamService(teams, users, &fakeMailer{})

	owner := seedUser(t, users, "owner@example.com")
	outsider := seedUser(t, users, "outsider@example.com")
	team, _ := svc.Create(context.Background(), "Team", owner)

	err := svc.Invite(context.Background(), InviteInput{
		TeamID: team.ID, InviterID: outsider, Email: "owner@example.com",
	})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("non-member must be forbidden, got %v", err)
	}
}

func TestTeam_InviteUnknownEmail(t *testing.T) {
	teams := newFakeTeamRepo()
	users := newFakeUserRepo()
	svc := NewTeamService(teams, users, &fakeMailer{})

	owner := seedUser(t, users, "owner@example.com")
	team, _ := svc.Create(context.Background(), "Team", owner)

	err := svc.Invite(context.Background(), InviteInput{
		TeamID: team.ID, InviterID: owner, Email: "ghost@example.com",
	})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected validation error for unknown email, got %v", err)
	}
}

func TestTeam_InviteDuplicateMember(t *testing.T) {
	teams := newFakeTeamRepo()
	users := newFakeUserRepo()
	svc := NewTeamService(teams, users, &fakeMailer{})

	owner := seedUser(t, users, "owner@example.com")
	member := seedUser(t, users, "member@example.com")
	team, _ := svc.Create(context.Background(), "Team", owner)
	_ = teams.AddMember(context.Background(), team.ID, member, domain.RoleMember)

	err := svc.Invite(context.Background(), InviteInput{
		TeamID: team.ID, InviterID: owner, Email: "member@example.com",
	})
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("expected conflict for duplicate member, got %v", err)
	}
}

func TestTeam_InviteEmailFailure(t *testing.T) {
	teams := newFakeTeamRepo()
	users := newFakeUserRepo()
	mailer := &fakeMailer{err: errors.New("smtp down")}
	svc := NewTeamService(teams, users, mailer)

	owner := seedUser(t, users, "owner@example.com")
	seedUser(t, users, "newbie@example.com")
	team, _ := svc.Create(context.Background(), "Team", owner)

	err := svc.Invite(context.Background(), InviteInput{
		TeamID: team.ID, InviterID: owner, Email: "newbie@example.com",
	})
	if !errors.Is(err, ErrEmailFailed) {
		t.Fatalf("expected ErrEmailFailed, got %v", err)
	}
	// Участник всё равно должен быть добавлен.
	if _, err := teams.GetMember(context.Background(), team.ID, 2); err != nil {
		t.Fatalf("member should be added despite email failure: %v", err)
	}
}

func TestTeam_InviteRejectsOwnerRole(t *testing.T) {
	svc := NewTeamService(newFakeTeamRepo(), newFakeUserRepo(), &fakeMailer{})
	err := svc.Invite(context.Background(), InviteInput{
		TeamID: 1, InviterID: 1, Email: "x@example.com", Role: domain.RoleOwner,
	})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("inviting as owner must be rejected, got %v", err)
	}
}
