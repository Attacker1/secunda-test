package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"test-task/internal/domain"
	"test-task/internal/service"
)

// AnalyticsService — три обязательных аналитических запроса.
type AnalyticsService interface {
	TeamStats(ctx context.Context) ([]domain.TeamStats, error)
	TopCreators(ctx context.Context) ([]domain.TopCreator, error)
	IntegrityIssues(ctx context.Context) ([]domain.IntegrityIssue, error)
}

type Handler struct {
	auth      *service.AuthService
	teams     *service.TeamService
	tasks     *service.TaskService
	analytics AnalyticsService
	log       *slog.Logger
}

func NewHandler(
	auth *service.AuthService,
	teams *service.TeamService,
	tasks *service.TaskService,
	analytics AnalyticsService,
	log *slog.Logger,
) *Handler {
	return &Handler{auth: auth, teams: teams, tasks: tasks, analytics: analytics, log: log}
}

// auth

type registerRequest struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	Password string `json:"password"`
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	user, err := h.auth.Register(r.Context(), service.RegisterInput{
		Email: req.Email, Name: req.Name, Password: req.Password,
	})
	if err != nil {
		writeError(w, h.log, err)
		return
	}
	writeJSON(w, http.StatusCreated, user)
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	token, user, err := h.auth.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		writeError(w, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"token": token, "user": user})
}

// teams

type createTeamRequest struct {
	Name string `json:"name"`
}

func (h *Handler) CreateTeam(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFromCtx(r.Context())
	var req createTeamRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	team, err := h.teams.Create(r.Context(), req.Name, userID)
	if err != nil {
		writeError(w, h.log, err)
		return
	}
	writeJSON(w, http.StatusCreated, team)
}

func (h *Handler) ListTeams(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFromCtx(r.Context())
	teams, err := h.teams.ListForUser(r.Context(), userID)
	if err != nil {
		writeError(w, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, teams)
}

type inviteRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

func (h *Handler) InviteToTeam(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFromCtx(r.Context())
	teamID, err := pathInt(r, "id")
	if err != nil {
		writeError(w, h.log, err)
		return
	}
	var req inviteRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	err = h.teams.Invite(r.Context(), service.InviteInput{
		TeamID: teamID, InviterID: userID, Email: req.Email, Role: domain.Role(req.Role),
	})
	if err != nil {
		// ErrEmailFailed маппится на 202: участник добавлен, письмо не ушло.
		writeError(w, h.log, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

// tasks

type createTaskRequest struct {
	TeamID      int64  `json:"team_id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Status      string `json:"status"`
	AssigneeID  *int64 `json:"assignee_id"`
}

func (h *Handler) CreateTask(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFromCtx(r.Context())
	var req createTaskRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	task, err := h.tasks.Create(r.Context(), service.CreateTaskInput{
		TeamID: req.TeamID, Title: req.Title, Description: req.Description,
		Status: domain.TaskStatus(req.Status), AssigneeID: req.AssigneeID, ActorID: userID,
	})
	if err != nil {
		writeError(w, h.log, err)
		return
	}
	writeJSON(w, http.StatusCreated, task)
}

func (h *Handler) ListTasks(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFromCtx(r.Context())
	q := r.URL.Query()

	teamID, err := strconv.ParseInt(q.Get("team_id"), 10, 64)
	if err != nil || teamID <= 0 {
		writeError(w, h.log, validationErr("team_id is required"))
		return
	}
	f := domain.TaskFilter{TeamID: teamID}
	if s := q.Get("status"); s != "" {
		st := domain.TaskStatus(s)
		if !st.Valid() {
			writeError(w, h.log, validationErr("invalid status"))
			return
		}
		f.Status = &st
	}
	if a := q.Get("assignee_id"); a != "" {
		aid, err := strconv.ParseInt(a, 10, 64)
		if err != nil {
			writeError(w, h.log, validationErr("invalid assignee_id"))
			return
		}
		f.AssigneeID = &aid
	}
	f.Limit = atoiDefault(q.Get("limit"), 0)
	f.Offset = atoiDefault(q.Get("offset"), 0)

	tasks, err := h.tasks.List(r.Context(), f, userID)
	if err != nil {
		writeError(w, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, tasks)
}

type updateTaskRequest struct {
	Title       *string `json:"title"`
	Description *string `json:"description"`
	Status      *string `json:"status"`
	AssigneeID  *int64  `json:"assignee_id"`
	UnsetAssign bool    `json:"unset_assignee"`
}

func (h *Handler) UpdateTask(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFromCtx(r.Context())
	taskID, err := pathInt(r, "id")
	if err != nil {
		writeError(w, h.log, err)
		return
	}
	var req updateTaskRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	in := service.UpdateTaskInput{
		TaskID: taskID, Title: req.Title, Description: req.Description,
		AssigneeID: req.AssigneeID, UnsetAssign: req.UnsetAssign, ActorID: userID,
	}
	if req.Status != nil {
		st := domain.TaskStatus(*req.Status)
		in.Status = &st
	}
	task, err := h.tasks.Update(r.Context(), in)
	if err != nil {
		writeError(w, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, task)
}

func (h *Handler) TaskHistory(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFromCtx(r.Context())
	taskID, err := pathInt(r, "id")
	if err != nil {
		writeError(w, h.log, err)
		return
	}
	history, err := h.tasks.History(r.Context(), taskID, userID)
	if err != nil {
		writeError(w, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, history)
}

// analytics

func (h *Handler) TeamStats(w http.ResponseWriter, r *http.Request) {
	rows, err := h.analytics.TeamStats(r.Context())
	if err != nil {
		writeError(w, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (h *Handler) TopCreators(w http.ResponseWriter, r *http.Request) {
	rows, err := h.analytics.TopCreators(r.Context())
	if err != nil {
		writeError(w, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (h *Handler) IntegrityIssues(w http.ResponseWriter, r *http.Request) {
	rows, err := h.analytics.IntegrityIssues(r.Context())
	if err != nil {
		writeError(w, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

// helpers

func pathInt(r *http.Request, key string) (int64, error) {
	v := chi.URLParam(r, key)
	id, err := strconv.ParseInt(v, 10, 64)
	if err != nil || id <= 0 {
		return 0, validationErr("invalid " + key)
	}
	return id, nil
}

func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

func validationErr(msg string) error {
	return &fieldError{msg: msg}
}

type fieldError struct{ msg string }

func (e *fieldError) Error() string { return "validation failed: " + e.msg }
func (e *fieldError) Is(target error) bool {
	return target == domain.ErrValidation
}
