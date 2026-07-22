-- Todorio · 0002_features · TOTP, attachments, statistics, announcements, digest.
-- Also aligns the 0001 schema with the API code.

-- schema alignment
ALTER TABLE tasks RENAME COLUMN created_by TO creator_id;
ALTER TABLE tasks DROP COLUMN blocked_by;
ALTER TABLE tasks ADD COLUMN blocked_by BIGINT[] NOT NULL DEFAULT '{}';
ALTER TABLE task_versions RENAME COLUMN changed_by TO editor_id;
ALTER TABLE comments ADD COLUMN deleted_at TIMESTAMPTZ;

-- TOTP (two-factor auth for root/admins)
ALTER TABLE users ADD COLUMN totp_enabled BOOLEAN NOT NULL DEFAULT FALSE;

-- digest after an absence of >=6 hours: records the last-seen moment before the pause
ALTER TABLE users ADD COLUMN prev_seen_at TIMESTAMPTZ;

-- announcement acknowledgements/dismissals
CREATE TABLE announcement_acks (
    announcement_id BIGINT NOT NULL REFERENCES announcements(id) ON DELETE CASCADE,
    user_id         BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    acked_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (announcement_id, user_id)
);

-- Two-part statistics captions: final phrase = part 1 + part 2.
-- 15 × 15 = 225 combinations for ru-RU.
INSERT INTO stat_captions(locale, category, part, text) VALUES
('ru-RU','success',1,'Машина продуктивности'),
('ru-RU','success',1,'Герой дедлайнов'),
('ru-RU','success',1,'Тихий терминатор задач'),
('ru-RU','success',1,'Повелитель чекбоксов'),
('ru-RU','success',1,'Спринтер недели'),
('ru-RU','neutral',1,'Стабильность — признак мастерства'),
('ru-RU','neutral',1,'Работа идёт'),
('ru-RU','neutral',1,'Курс держим'),
('ru-RU','neutral',1,'Без резких движений'),
('ru-RU','neutral',1,'Планомерно и уверенно'),
('ru-RU','overdue',1,'Дедлайны шалят'),
('ru-RU','overdue',1,'График дал трещину'),
('ru-RU','overdue',1,'Время собирать хвосты'),
('ru-RU','focus',1,'Режим концентрации'),
('ru-RU','focus',1,'Глубокое погружение'),
('ru-RU','success',2,'— задачи разбегаются в ужасе'),
('ru-RU','success',2,'— ни один дедлайн не пострадал'),
('ru-RU','success',2,'— так держать!'),
('ru-RU','success',2,'— команда аплодирует стоя'),
('ru-RU','success',2,'— кофе явно был хорош'),
('ru-RU','neutral',2,'— завтра будет ещё лучше'),
('ru-RU','neutral',2,'— главное не останавливаться'),
('ru-RU','neutral',2,'— прогресс есть, и это главное'),
('ru-RU','neutral',2,'— маленькими шагами к большому'),
('ru-RU','neutral',2,'— Пульс ровный, пациент работает'),
('ru-RU','overdue',2,'— но всё ещё можно исправить'),
('ru-RU','overdue',2,'— дедлайны помнят всё'),
('ru-RU','overdue',2,'— время включать турбо-режим'),
('ru-RU','focus',2,'— ничто не отвлекает от цели'),
('ru-RU','focus',2,'— результат говорит сам за себя'),
('en-US','success',1,'Productivity machine'),
('en-US','success',1,'Deadline hero'),
('en-US','neutral',1,'Steady as she goes'),
('en-US','overdue',1,'Deadlines are drifting'),
('en-US','focus',1,'Deep focus mode'),
('en-US','success',2,'— tasks flee in terror'),
('en-US','success',2,'— not a single deadline was harmed'),
('en-US','neutral',2,'— progress is progress'),
('en-US','overdue',2,'— time to catch up'),
('en-US','focus',2,'— nothing breaks the flow');
