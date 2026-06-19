# secunda-test

Сервис управления задачами: команды, роли, история изменений. Go + MySQL + Redis.

## Запуск

```bash
make up        # docker compose: mysql + redis + api на :8080
make down
```

Локально (нужны запущенные mysql и redis):

```bash
make run
```

Конфиг в `config.yaml`, можно переопределять через ENV (`APP_HTTP__PORT`, `APP_MYSQL__DSN`, `APP_REDIS__ADDR`, `APP_AUTH__JWT_SECRET` и т.д.).

## Тесты

```bash
make test-unit          # бизнес-логика
make test-integration   # репозитории, нужен docker (testcontainers)
make cover              # покрытие service-слоя
```

## Эндпоинты

```
POST /api/v1/register
POST /api/v1/login
POST /api/v1/teams
GET  /api/v1/teams
POST /api/v1/teams/{id}/invite
POST /api/v1/tasks
GET  /api/v1/tasks?team_id=&status=&assignee_id=&limit=&offset=
PUT  /api/v1/tasks/{id}
GET  /api/v1/tasks/{id}/history
GET  /api/v1/analytics/team-stats
GET  /api/v1/analytics/top-creators
GET  /api/v1/analytics/integrity-issues
GET  /metrics
```

Защищённые ручки требуют заголовок `Authorization: Bearer <token>`.

## Заметки

- Сложные SQL — в `internal/repository/analytics_repo.go`.
- Кеш списка задач в Redis (TTL 5 мин), инвалидация при изменениях.
- Приглашения шлёт мок email-сервиса, обёрнутый в circuit breaker.
- Rate limit 100 req/min на пользователя, graceful shutdown, метрики prometheus.
