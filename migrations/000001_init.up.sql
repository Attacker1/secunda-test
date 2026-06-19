-- 1. Пользователи
CREATE TABLE users (
    id            BIGINT       NOT NULL AUTO_INCREMENT,
    email         VARCHAR(255) NOT NULL,
    name          VARCHAR(255) NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    created_at    TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_users_email (email)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4;

-- 2. Команды
CREATE TABLE teams (
    id         BIGINT       NOT NULL AUTO_INCREMENT,
    name       VARCHAR(255) NOT NULL,
    created_by BIGINT       NOT NULL,
    created_at TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    CONSTRAINT fk_teams_created_by FOREIGN KEY (created_by) REFERENCES users (id)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4;

-- 3. Связь пользователь -> команда (многие-ко-многим) + роль
CREATE TABLE team_members (
    team_id   BIGINT                              NOT NULL,
    user_id   BIGINT                              NOT NULL,
    role      ENUM ('owner', 'admin', 'member')   NOT NULL DEFAULT 'member',
    joined_at TIMESTAMP                           NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (team_id, user_id),
    CONSTRAINT fk_tm_team FOREIGN KEY (team_id) REFERENCES teams (id) ON DELETE CASCADE,
    CONSTRAINT fk_tm_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
    -- Ускоряет "список команд пользователя" (GET /teams) и проверки членства.
    KEY idx_tm_user (user_id)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4;

-- 4. Задачи
CREATE TABLE tasks (
    id          BIGINT                                       NOT NULL AUTO_INCREMENT,
    team_id     BIGINT                                       NOT NULL,
    title       VARCHAR(500)                                 NOT NULL,
    description TEXT                                         NOT NULL,
    status      ENUM ('todo', 'in_progress', 'done')         NOT NULL DEFAULT 'todo',
    assignee_id BIGINT                                       NULL,
    created_by  BIGINT                                       NOT NULL,
    created_at  TIMESTAMP                                    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMP                                    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    CONSTRAINT fk_tasks_team FOREIGN KEY (team_id) REFERENCES teams (id) ON DELETE CASCADE,
    CONSTRAINT fk_tasks_assignee FOREIGN KEY (assignee_id) REFERENCES users (id),
    CONSTRAINT fk_tasks_created_by FOREIGN KEY (created_by) REFERENCES users (id),
    -- Покрывающий индекс под основной фильтр списка: WHERE team_id [+ status].
    KEY idx_tasks_team_status (team_id, status),
    -- Под фильтр по исполнителю и аналитику integrity-check.
    KEY idx_tasks_assignee (assignee_id),
    -- Под "топ создателей": группировка по created_by в пределах команды.
    KEY idx_tasks_team_creator_created (team_id, created_by, created_at)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4;

-- 5. История изменений задач (аудит)
CREATE TABLE task_history (
    id         BIGINT       NOT NULL AUTO_INCREMENT,
    task_id    BIGINT       NOT NULL,
    changed_by BIGINT       NOT NULL,
    field      VARCHAR(64)  NOT NULL,
    old_value  TEXT         NULL,
    new_value  TEXT         NULL,
    changed_at TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    CONSTRAINT fk_th_task FOREIGN KEY (task_id) REFERENCES tasks (id) ON DELETE CASCADE,
    CONSTRAINT fk_th_user FOREIGN KEY (changed_by) REFERENCES users (id),
    -- История конкретной задачи + аналитика "done за 7 дней" (по полю status).
    KEY idx_th_task (task_id, changed_at),
    KEY idx_th_field_changed (field, changed_at)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4;

-- 6. Комментарии к задачам
CREATE TABLE task_comments (
    id         BIGINT    NOT NULL AUTO_INCREMENT,
    task_id    BIGINT    NOT NULL,
    user_id    BIGINT    NOT NULL,
    body       TEXT      NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    CONSTRAINT fk_tc_task FOREIGN KEY (task_id) REFERENCES tasks (id) ON DELETE CASCADE,
    CONSTRAINT fk_tc_user FOREIGN KEY (user_id) REFERENCES users (id),
    KEY idx_tc_task (task_id, created_at)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4;
