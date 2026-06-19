package repository

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"

	"test-task/internal/domain"
)

type AnalyticsRepo struct {
	db *sqlx.DB
}

func NewAnalyticsRepo(db *sqlx.DB) *AnalyticsRepo {
	return &AnalyticsRepo{db: db}
}

// TeamStats — запрос (а): JOIN 4 таблиц + агрегация.
// Для каждой команды: название, число участников и число задач, переведённых
// в статус done за последние 7 дней. "Переведённых" определяем по task_history
// (field='status', new_value='done'), а не по tasks.updated_at — иначе любое
// более позднее изменение задачи исказило бы окно.
func (r *AnalyticsRepo) TeamStats(ctx context.Context) ([]domain.TeamStats, error) {
	const q = `
SELECT
    t.id   AS team_id,
    t.name AS team_name,
    COUNT(DISTINCT tm.user_id) AS member_count,
    COUNT(DISTINCT CASE WHEN th.id IS NOT NULL THEN th.task_id END) AS done_last_7_days
FROM teams t
LEFT JOIN team_members tm ON tm.team_id = t.id
LEFT JOIN tasks tk        ON tk.team_id = t.id
LEFT JOIN task_history th ON th.task_id = tk.id
                          AND th.field = 'status'
                          AND th.new_value = 'done'
                          AND th.changed_at >= (NOW() - INTERVAL 7 DAY)
GROUP BY t.id, t.name
ORDER BY t.id`
	rows := []domain.TeamStats{}
	if err := r.db.SelectContext(ctx, &rows, q); err != nil {
		return nil, fmt.Errorf("team stats: %w", err)
	}
	return rows, nil
}

// TopCreators — запрос (б): оконная функция ROW_NUMBER().
// Топ-3 пользователя по числу созданных задач в каждой команде за последний месяц.
func (r *AnalyticsRepo) TopCreators(ctx context.Context) ([]domain.TopCreator, error) {
	const q = `
SELECT team_id, team_name, user_id, user_name, task_count, rnk
FROM (
    SELECT
        tk.team_id            AS team_id,
        t.name                AS team_name,
        tk.created_by         AS user_id,
        u.name                AS user_name,
        COUNT(*)              AS task_count,
        ROW_NUMBER() OVER (
            PARTITION BY tk.team_id
            ORDER BY COUNT(*) DESC, tk.created_by ASC
        ) AS rnk
    FROM tasks tk
    JOIN teams t ON t.id = tk.team_id
    JOIN users u ON u.id = tk.created_by
    WHERE tk.created_at >= (NOW() - INTERVAL 1 MONTH)
    GROUP BY tk.team_id, t.name, tk.created_by, u.name
) ranked
WHERE rnk <= 3
ORDER BY team_id, rnk`
	rows := []domain.TopCreator{}
	if err := r.db.SelectContext(ctx, &rows, q); err != nil {
		return nil, fmt.Errorf("top creators: %w", err)
	}
	return rows, nil
}

// IntegrityIssues — запрос (в): условие по связанным таблицам.
// Задачи, где назначенный исполнитель не состоит в команде задачи.
func (r *AnalyticsRepo) IntegrityIssues(ctx context.Context) ([]domain.IntegrityIssue, error) {
	const q = `
SELECT
    tk.id          AS task_id,
    tk.title       AS task_title,
    tk.team_id     AS team_id,
    tk.assignee_id AS assignee_id
FROM tasks tk
WHERE tk.assignee_id IS NOT NULL
  AND NOT EXISTS (
        SELECT 1 FROM team_members tm
        WHERE tm.team_id = tk.team_id
          AND tm.user_id = tk.assignee_id
  )
ORDER BY tk.id`
	rows := []domain.IntegrityIssue{}
	if err := r.db.SelectContext(ctx, &rows, q); err != nil {
		return nil, fmt.Errorf("integrity issues: %w", err)
	}
	return rows, nil
}
