# ⚡ Todorio

**Your private workspace for tasks and teams.**

Todorio — приватный self-hosted менеджер задач для личной работы и небольших команд. Ставится на VPS одной командой, работает без внешних SaaS, почты и публичного API. Главная фишка — **«Пульс пространства»**: живая сводка здоровья проекта.

## Возможности (v1)

- 🔐 Авторизация по логину/паролю (без email), TOTP, ручное одобрение регистраций
- 👑 RBAC: root admin / admin / user / viewer + точечные разрешения
- 🗂 Пространства → списки → задачи (подзадачи, зависимости, повторы, чек-листы)
- 📊 Прогресс-бары, настраиваемые workflow-статусы, пользовательские поля, метки
- 💬 Комментарии, @упоминания, реакции 👍 ✅ 🎉 🔥 👀 ❓ ❗ ❌ 😭 ⭐, вложения-картинки
- 🔔 Уведомления в кабинете, дедлайн-плашки 🔴🟠🟡⚪, «Не беспокоить»
- 🌍 13 локалей (формат язык-страна) + IT-стили `ru-RU-it`, `en-US-it`
- 🎨 5 цветовых тем (красная/синяя/зелёная/жёлтая/серая), светлая+тёмная, режимы «красивый»/«лёгкий»
- ⚡ SSE-реалтайм, PWA (кнопка «Установить приложение»)
- 📈 Статистика, динамические подписи, рейтинги, **Пульс пространства**
- 🛠 Всё конфигурируется в root-панели **и** в терминале (`todorio server ...`)

## Установка

```bash
curl -fsSL https://raw.githubusercontent.com/DanMotive/Todorio/main/scripts/install.sh | sudo bash
sudo todorio setup
```

`setup` спросит: логин root-админа, менеджер процессов (systemd/docker/pm2), порт, HTTPS (self-signed), демо-пространство с обучающими квестами (y/n) — и сгенерирует временный пароль на 16 символов.

## Разработка

```bash
# Backend (Go 1.22+)
go run ./cmd/todorio serve --dev

# Frontend (Node 20+)
cd web && npm install && npm run dev
```

## Структура

```
cmd/todorio/     — CLI и точка входа (setup, serve, doctor, backup, server config)
internal/        — config, server (HTTP+SSE), setup
migrations/      — SQL-миграции PostgreSQL
scripts/         — install.sh
web/             — React + Vite фронтенд, темы, локали, PWA
```

## Лицензия

Apache 2.0 · Разработано **Vlad**
