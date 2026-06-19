// Package migrations встраивает SQL-файлы миграций в бинарь, чтобы их можно
// было применять как в рантайме (cmd/api), так и в интеграционных тестах.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
