// Package httpapi реализует HTTP-транспорт: роутер, middleware и обработчики.
package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"test-task/internal/domain"
	"test-task/internal/service"
)

type errorBody struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

func writeError(w http.ResponseWriter, log *slog.Logger, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, domain.ErrValidation):
		status = http.StatusBadRequest
	case errors.Is(err, domain.ErrInvalidCredential), errors.Is(err, domain.ErrUnauthorized):
		status = http.StatusUnauthorized
	case errors.Is(err, domain.ErrForbidden):
		status = http.StatusForbidden
	case errors.Is(err, domain.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, domain.ErrConflict):
		status = http.StatusConflict
	case errors.Is(err, service.ErrEmailFailed):
		// Участник добавлен, письмо не ушло — это не ошибка клиента.
		status = http.StatusAccepted
	}
	if status >= http.StatusInternalServerError {
		log.Error("request failed", "err", err)
		writeJSON(w, status, errorBody{Error: "internal server error"})
		return
	}
	writeJSON(w, status, errorBody{Error: err.Error()})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody{Error: "invalid JSON body"})
		return false
	}
	return true
}
