-- Todorio · 0001_init · PostgreSQL 14+
-- Ядро схемы: пользователи, пространства, списки, задачи, взаимодействия, настройки.

CREATE TABLE users (
    id              BIGSERIAL PRIMARY KEY,
    username        TEXT NOT NULL UNIQUE,
    password_hash   TEXT NOT NULL,                  -- argon2id
    role            TEXT NOT NULL DEFAULT 'user',   -- root | admin | user | viewer
    status          TEXT NOT NULL DEFAULT 'pending',-- pending | active | blocked | rejected
    must_change_password BOOLEAN NOT NULL DEFAULT FALSE,
    totp_secret     TEXT,
    display_name    TEXT,
    avatar_path     TEXT,                           -- NULL = авто-инициалы
    locale          TEXT,                           -- NULL = автоподбор по Accept-Language
    -- Тема: NULL = дефолт сервера (задаёт root), пользователь может переопределить
    theme_color     TEXT CHECK (theme_color IN ('red','blue','green','yellow','gray')),
    theme_scheme    TEXT CHECK (theme_scheme IN ('light','dark')),
    theme_visual    TEXT CHECK (theme_visual IN ('rich','lite')),
    notify_prefs    JSONB NOT NULL DEFAULT '{}',    -- типы уведомлений, звук, тихие часы
    permissions     JSONB NOT NULL DEFAULT '{}',    -- точечные разрешения при одобрении
    onboarding      JSONB NOT NULL DEFAULT '{}',    -- прогресс обучающих квестов
    last_seen_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    archived_at     TIMESTAMPTZ                     -- архив, автоочистка через 30 дней
);

CREATE TABLE sessions (
    id          TEXT PRIMARY KEY,                   -- случайный токен (cookie HttpOnly+Secure+SameSite)
    user_id     BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at  TIMESTAMPTZ NOT NULL,
    user_agent  TEXT
);

CREATE TABLE spaces (
    id          BIGSERIAL PRIMARY KEY,
    name        TEXT NOT NULL,
    owner_id    BIGINT NOT NULL REFERENCES users(id),
    settings    JSONB NOT NULL DEFAULT '{}',        -- workflow, поля, рейтинги, Пульс, объявления
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    archived_at TIMESTAMPTZ
);

CREATE TABLE space_members (
    space_id    BIGINT NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
    user_id     BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role        TEXT NOT NULL DEFAULT 'member',     -- owner | member | viewer
    PRIMARY KEY (space_id, user_id)
);

CREATE TABLE lists (
    id          BIGSERIAL PRIMARY KEY,
    space_id    BIGINT NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    is_private  BOOLEAN NOT NULL DEFAULT FALSE,     -- приватный список внутри общего пространства
    settings    JSONB NOT NULL DEFAULT '{}',
    position    INTEGER NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    archived_at TIMESTAMPTZ
);

CREATE TABLE list_members (
    list_id     BIGINT NOT NULL REFERENCES lists(id) ON DELETE CASCADE,
    user_id     BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    permission  TEXT NOT NULL DEFAULT 'viewer',     -- owner | editor | viewer
    PRIMARY KEY (list_id, user_id)
);

CREATE TABLE tasks (
    id           BIGSERIAL PRIMARY KEY,
    list_id      BIGINT NOT NULL REFERENCES lists(id) ON DELETE CASCADE,
    parent_id    BIGINT REFERENCES tasks(id) ON DELETE CASCADE, -- подзадача
    blocked_by   BIGINT REFERENCES tasks(id) ON DELETE SET NULL,-- зависимость
    title        TEXT NOT NULL,
    description  TEXT NOT NULL DEFAULT '',
    status       TEXT NOT NULL DEFAULT 'new',       -- из workflow пространства
    priority     TEXT,                              -- low | normal | high | urgent
    assignee_id  BIGINT REFERENCES users(id) ON DELETE SET NULL, -- блокировка/удаление → NULL (unassigned)
    due_at       TIMESTAMPTZ,
    recurrence   JSONB,                             -- правило повторения
    progress     SMALLINT,                          -- ручной прогресс 0..100 (NULL = авто по подзадачам)
    weight       SMALLINT NOT NULL DEFAULT 1,       -- вес для взвешенного прогресса/рейтинга
    custom_fields JSONB NOT NULL DEFAULT '{}',      -- значения полей пространства (в т.ч. метки)
    created_by   BIGINT NOT NULL REFERENCES users(id),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ,
    archived_at  TIMESTAMPTZ
);
CREATE INDEX idx_tasks_list ON tasks(list_id) WHERE archived_at IS NULL;
CREATE INDEX idx_tasks_assignee ON tasks(assignee_id) WHERE archived_at IS NULL;
CREATE INDEX idx_tasks_due ON tasks(due_at) WHERE archived_at IS NULL;

CREATE TABLE task_versions (
    id          BIGSERIAL PRIMARY KEY,
    task_id     BIGINT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    snapshot    JSONB NOT NULL,                     -- состояние задачи до изменения
    changed_by  BIGINT NOT NULL REFERENCES users(id),
    changed_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE comments (
    id          BIGSERIAL PRIMARY KEY,
    task_id     BIGINT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    author_id   BIGINT NOT NULL REFERENCES users(id),
    body        TEXT NOT NULL,
    is_system   BOOLEAN NOT NULL DEFAULT FALSE,     -- системные события в ленте
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    edited_at   TIMESTAMPTZ
);

CREATE TABLE reactions (
    id          BIGSERIAL PRIMARY KEY,
    target_type TEXT NOT NULL CHECK (target_type IN ('task','comment')),
    target_id   BIGINT NOT NULL,
    user_id     BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    emoji       TEXT NOT NULL,                      -- из фиксированного набора
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (target_type, target_id, user_id, emoji)
);

CREATE TABLE attachments (
    id          BIGSERIAL PRIMARY KEY,
    target_type TEXT NOT NULL CHECK (target_type IN ('task','comment','avatar','branding')),
    target_id   BIGINT,
    uploader_id BIGINT NOT NULL REFERENCES users(id),
    file_path   TEXT NOT NULL,                      -- /var/lib/todorio/uploads/...
    mime_type   TEXT NOT NULL,                      -- image/jpeg|png|webp|gif
    size_bytes  BIGINT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE notifications (
    id          BIGSERIAL PRIMARY KEY,
    user_id     BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind        TEXT NOT NULL,  -- mention | comment | reaction | assigned | due_changed | status_changed | overdue | announcement
    payload     JSONB NOT NULL DEFAULT '{}',
    read_at     TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_notifications_unread ON notifications(user_id) WHERE read_at IS NULL;

CREATE TABLE announcements (
    id          BIGSERIAL PRIMARY KEY,
    space_id    BIGINT REFERENCES spaces(id) ON DELETE CASCADE, -- NULL = весь сервер (только root)
    author_id   BIGINT NOT NULL REFERENCES users(id),
    level       TEXT NOT NULL DEFAULT 'normal' CHECK (level IN ('normal','important','emergency')),
    body        TEXT NOT NULL,
    requires_ack BOOLEAN NOT NULL DEFAULT FALSE,
    expires_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE templates (
    id          BIGSERIAL PRIMARY KEY,              -- создаёт/публикует только root
    name        TEXT NOT NULL,
    body        JSONB NOT NULL,                     -- структура списка/задач
    audience    JSONB NOT NULL DEFAULT '{}',        -- все | роли | админы
    auto_apply  BOOLEAN NOT NULL DEFAULT FALSE,     -- создавать новому одобренному пользователю
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE stat_captions (
    id          BIGSERIAL PRIMARY KEY,
    locale      TEXT NOT NULL,
    category    TEXT NOT NULL,  -- success | perfect_day | overdue | inactive | focus | project | neutral
    part        SMALLINT NOT NULL CHECK (part IN (1,2)),
    text        TEXT NOT NULL
);

CREATE TABLE stat_caption_picks (                    -- фиксация подписи на день+пользователя
    user_id     BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    day         DATE NOT NULL,
    caption_1   BIGINT NOT NULL REFERENCES stat_captions(id),
    caption_2   BIGINT NOT NULL REFERENCES stat_captions(id),
    PRIMARY KEY (user_id, day)
);

CREATE TABLE audit_log (
    id          BIGSERIAL PRIMARY KEY,
    actor_id    BIGINT REFERENCES users(id),
    action      TEXT NOT NULL,
    target      TEXT,
    details     JSONB NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE system_settings (                       -- политики/лимиты/брендинг/локали: root-панель и CLI пишут сюда
    key         TEXT PRIMARY KEY,                    -- например policy.registration.mode, limits.uploads.max_file_size_mb, branding.site_name
    value       JSONB NOT NULL,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
