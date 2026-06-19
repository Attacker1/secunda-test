package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"test-task/internal/domain"
	"test-task/internal/email"
)

type TeamService struct {
	teams  TeamRepository
	users  UserRepository
	mailer email.Sender
}

func NewTeamService(teams TeamRepository, users UserRepository, mailer email.Sender) *TeamService {
	return &TeamService{teams: teams, users: users, mailer: mailer}
}

func (s *TeamService) Create(ctx context.Context, name string, ownerID int64) (*domain.Team, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("%w: team name is required", domain.ErrValidation)
	}
	id, err := s.teams.CreateWithOwner(ctx, name, ownerID)
	if err != nil {
		return nil, err
	}
	return &domain.Team{ID: id, Name: name, CreatedBy: ownerID}, nil
}

func (s *TeamService) ListForUser(ctx context.Context, userID int64) ([]domain.Team, error) {
	return s.teams.ListByUser(ctx, userID)
}

type InviteInput struct {
	TeamID    int64
	InviterID int64
	Email     string
	Role      domain.Role
}

// Invite добавляет пользователя в команду. Разрешено только owner/admin.
// После добавления отправляет приглашение через email-сервис (circuit breaker);
// сбой отправки не откатывает членство — возвращается как предупреждение.
func (s *TeamService) Invite(ctx context.Context, in InviteInput) error {
	if in.Role == "" {
		in.Role = domain.RoleMember
	}
	if !in.Role.Valid() || in.Role == domain.RoleOwner {
		return fmt.Errorf("%w: invalid role", domain.ErrValidation)
	}

	// Проверка прав приглашающего.
	inviter, err := s.teams.GetMember(ctx, in.TeamID, in.InviterID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return fmt.Errorf("%w: not a team member", domain.ErrForbidden)
		}
		return err
	}
	if !inviter.Role.CanManageMembers() {
		return fmt.Errorf("%w: only owner or admin can invite", domain.ErrForbidden)
	}

	invitee, err := s.users.GetByEmail(ctx, strings.ToLower(strings.TrimSpace(in.Email)))
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return fmt.Errorf("%w: user with this email not found", domain.ErrValidation)
		}
		return err
	}

	if err := s.teams.AddMember(ctx, in.TeamID, invitee.ID, in.Role); err != nil {
		if errors.Is(err, domain.ErrConflict) {
			return fmt.Errorf("%w: user is already a team member", domain.ErrConflict)
		}
		return err
	}

	// Отправка письма не критична для целостности — ошибку отдаём как
	// ErrEmailFailed, чтобы транспорт мог вернуть 202/частичный успех.
	if err := s.mailer.SendInvite(ctx, invitee.Email, fmt.Sprintf("team #%d", in.TeamID)); err != nil {
		return fmt.Errorf("%w: %v", ErrEmailFailed, err)
	}
	return nil
}

// ErrEmailFailed сигнализирует, что участник добавлен, но письмо не ушло.
var ErrEmailFailed = errors.New("invite saved but email delivery failed")
