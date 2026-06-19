// Package domain содержит модели предметной области и доменные ошибки.
// Слой не зависит ни от БД, ни от транспорта.
package domain

import "time"

type Role string

const (
	RoleOwner  Role = "owner"
	RoleAdmin  Role = "admin"
	RoleMember Role = "member"
)

func (r Role) Valid() bool {
	switch r {
	case RoleOwner, RoleAdmin, RoleMember:
		return true
	}
	return false
}

// CanManageMembers — может ли роль приглашать участников.
func (r Role) CanManageMembers() bool {
	return r == RoleOwner || r == RoleAdmin
}

type TaskStatus string

const (
	StatusTodo       TaskStatus = "todo"
	StatusInProgress TaskStatus = "in_progress"
	StatusDone       TaskStatus = "done"
)

func (s TaskStatus) Valid() bool {
	switch s {
	case StatusTodo, StatusInProgress, StatusDone:
		return true
	}
	return false
}

type User struct {
	ID           int64     `db:"id" json:"id"`
	Email        string    `db:"email" json:"email"`
	Name         string    `db:"name" json:"name"`
	PasswordHash string    `db:"password_hash" json:"-"`
	CreatedAt    time.Time `db:"created_at" json:"created_at"`
}

type Team struct {
	ID        int64     `db:"id" json:"id"`
	Name      string    `db:"name" json:"name"`
	CreatedBy int64     `db:"created_by" json:"created_by"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

type TeamMember struct {
	TeamID   int64     `db:"team_id" json:"team_id"`
	UserID   int64     `db:"user_id" json:"user_id"`
	Role     Role      `db:"role" json:"role"`
	JoinedAt time.Time `db:"joined_at" json:"joined_at"`
}

type Task struct {
	ID          int64      `db:"id" json:"id"`
	TeamID      int64      `db:"team_id" json:"team_id"`
	Title       string     `db:"title" json:"title"`
	Description string     `db:"description" json:"description"`
	Status      TaskStatus `db:"status" json:"status"`
	AssigneeID  *int64     `db:"assignee_id" json:"assignee_id"`
	CreatedBy   int64      `db:"created_by" json:"created_by"`
	CreatedAt   time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt   time.Time  `db:"updated_at" json:"updated_at"`
}

// TaskHistory — запись аудита изменения одного поля задачи.
type TaskHistory struct {
	ID        int64     `db:"id" json:"id"`
	TaskID    int64     `db:"task_id" json:"task_id"`
	ChangedBy int64     `db:"changed_by" json:"changed_by"`
	Field     string    `db:"field" json:"field"`
	OldValue  *string   `db:"old_value" json:"old_value"`
	NewValue  *string   `db:"new_value" json:"new_value"`
	ChangedAt time.Time `db:"changed_at" json:"changed_at"`
}

type TaskComment struct {
	ID        int64     `db:"id" json:"id"`
	TaskID    int64     `db:"task_id" json:"task_id"`
	UserID    int64     `db:"user_id" json:"user_id"`
	Body      string    `db:"body" json:"body"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

type TaskFilter struct {
	TeamID     int64
	Status     *TaskStatus
	AssigneeID *int64
	Limit      int
	Offset     int
}

// DTO результатов аналитических запросов

// TeamStats — строка отчёта "участники + задачи done за 7 дней".
type TeamStats struct {
	TeamID       int64  `db:"team_id" json:"team_id"`
	TeamName     string `db:"team_name" json:"team_name"`
	MemberCount  int    `db:"member_count" json:"member_count"`
	DoneLast7Day int    `db:"done_last_7_days" json:"done_last_7_days"`
}

// TopCreator — строка отчёта "топ-3 создателя задач по командам за месяц".
type TopCreator struct {
	TeamID    int64  `db:"team_id" json:"team_id"`
	TeamName  string `db:"team_name" json:"team_name"`
	UserID    int64  `db:"user_id" json:"user_id"`
	UserName  string `db:"user_name" json:"user_name"`
	TaskCount int    `db:"task_count" json:"task_count"`
	Rank      int    `db:"rnk" json:"rank"`
}

// IntegrityIssue — задача, чей assignee не состоит в команде задачи.
type IntegrityIssue struct {
	TaskID     int64  `db:"task_id" json:"task_id"`
	TaskTitle  string `db:"task_title" json:"task_title"`
	TeamID     int64  `db:"team_id" json:"team_id"`
	AssigneeID int64  `db:"assignee_id" json:"assignee_id"`
}
