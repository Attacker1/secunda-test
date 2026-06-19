package domain

import "errors"

// Доменные ошибки. Transport-слой мапит их на HTTP-статусы.
var (
	ErrNotFound          = errors.New("resource not found")
	ErrConflict          = errors.New("resource already exists")
	ErrInvalidCredential = errors.New("invalid email or password")
	ErrForbidden         = errors.New("operation not permitted")
	ErrValidation        = errors.New("validation failed")
	ErrUnauthorized      = errors.New("unauthorized")
)
