// Package demo — обучающее демо-пространство и онбординг-квесты.
package demo

import (
	"context"
	"fmt"

	"github.com/DanMotive/Todorio/internal/db"
)

type Quest struct {
	Title       string
	Description string
}

// Quests — задания для освоения сервиса (экскурсионный обзор в виде задач).
func Quests() []Quest {
	return []Quest{
		{"✅ Закрой эту задачу", "Нажми на кружок слева от названия — так отмечаются выполненные задачи."},
		{"📝 Создай свою первую задачу", "Открой свой список и нажми «Новая задача». Назови её как угодно!"},
		{"⏰ Поставь дедлайн", "Открой задачу и укажи срок — появится цветная плашка, а перед просрочкой придёт уведомление."},
		{"💬 Напиши комментарий", "В карточке задачи есть лента обсуждения. Упомяни коллегу через @логин — ему придёт уведомление."},
		{"🔥 Поставь реакцию", "Любой задаче или комментарию можно поставить смайл: 👍 ✅ 🎉 🔥 👀 ❓ ❗ ❌ 😭 ⭐"},
		{"🎨 Переключи тему", "В шапке сайта есть выбор из 5 цветов, светлая/тёмная схема и лёгкий режим для слабых машин."},
		{"📊 Загляни в Пульс пространства", "На странице пространства есть здоровье команды: просрочки, зависшие задачи и общий балл."},
		{"📱 Установи как приложение", "Кнопка ⬇️ в шапке ставит Todorio как PWA на телефон или в систему (нужен HTTPS)."},
	}
}

// EnsureDemo создаёт демо-пространство при первом запуске (если включено в setup).
// Повторные запуски — no-op (флаг onboarding.demo_space_id в system_settings).
func EnsureDemo(ctx context.Context, d *db.DB) error {
	if d.Setting(ctx, "onboarding.demo", "on") == "off" {
		return nil
	}
	if d.Setting(ctx, "onboarding.demo_space_id", "") != "" {
		return nil // уже создано
	}
	var rootID int64
	if err := d.Pool.QueryRow(ctx,
		`SELECT id FROM users WHERE role='root' ORDER BY id LIMIT 1`).Scan(&rootID); err != nil {
		return nil // root ещё не создан — попробуем при следующем старте
	}

	var spaceID int64
	if err := d.Pool.QueryRow(ctx,
		`INSERT INTO spaces(name, owner_id) VALUES($1,$2) RETURNING id`,
		"🎓 Добро пожаловать в Todorio", rootID).Scan(&spaceID); err != nil {
		return err
	}
	if _, err := d.Pool.Exec(ctx,
		`INSERT INTO space_members(space_id,user_id,role) VALUES($1,$2,'owner')`, spaceID, rootID); err != nil {
		return err
	}
	var listID int64
	if err := d.Pool.QueryRow(ctx,
		`INSERT INTO lists(space_id, name) VALUES($1,$2) RETURNING id`,
		spaceID, "🎯 Квесты освоения").Scan(&listID); err != nil {
		return err
	}
	if _, err := d.Pool.Exec(ctx,
		`INSERT INTO list_members(list_id,user_id,permission) VALUES($1,$2,'owner')`, listID, rootID); err != nil {
		return err
	}
	for _, q := range Quests() {
		if _, err := d.Pool.Exec(ctx, `
			INSERT INTO tasks(list_id, title, description, priority, assignee_id, creator_id)
			VALUES($1,$2,$3,'normal',$4,$4)`, listID, q.Title, q.Description, rootID); err != nil {
			return err
		}
	}
	return d.SetSetting(ctx, "onboarding.demo_space_id", fmt.Sprintf(`"%d"`, spaceID))
}
