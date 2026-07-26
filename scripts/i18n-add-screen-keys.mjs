#!/usr/bin/env node
// Adds the locale keys of the screens shipped in phases 1-4 to en-US and ru-RU.
//
// These screens (Members & permissions, Focus statistics, list rename/archive/reorder, the
// Workflow status editor) went out with inline fallbacks only. No key ever reached
// web/src/locales/*, and t() in i18n.ts ends with `return key`, so the UI printed nav.members,
// focus.stats_title and friends verbatim.
//
// Only the two base locales are filled in here: everything else resolves through the
// [locale, lang, "en-US"] chain, so the other eleven locales show English instead of a raw key
// until a translator gets to them. Guessing eleven translations of a hint text would be worse.
//
// Existing keys are never overwritten -- the script reports them and leaves them alone.
//
// Usage:
//   node scripts/i18n-add-screen-keys.mjs
//   node scripts/i18n-add-screen-keys.mjs --check   # verify only, exit 1 if keys are missing

import { readFileSync, writeFileSync, existsSync } from "node:fs"
import { dirname, join } from "node:path"
import { fileURLToPath } from "node:url"

const ROOT = join(dirname(fileURLToPath(import.meta.url)), "..")
const LOCALES = join(ROOT, "web", "src", "locales")

// key -> { en, ru }
// Placeholders are only used where the call site actually interpolates them:
// workflow.too_long replaces {max} itself. The rest are deliberately placeholder-free, because a
// {n} that nobody substitutes would be printed literally.
const KEYS = {
  "nav.members": { en: "Members", ru: "Участники" },

  "members.title": { en: "Members & access", ru: "Участники и доступ" },
  "members.loading": { en: "Loading…", ru: "Загрузка…" },
  "members.empty": { en: "Nobody here yet", ru: "Пока никого нет" },
  "members.you": { en: "that's you", ru: "это вы" },
  "members.add": { en: "Add", ru: "Добавить" },
  "members.remove": { en: "Revoke access", ru: "Убрать доступ" },
  "members.remove_confirm": { en: "Revoke access?", ru: "Убрать доступ?" },
  "members.read_only": { en: "read only", ru: "только чтение" },
  "members.read_only_hint": {
    en: "Global role \u201cviewer\u201d — writing is refused everywhere",
    ru: "Глобальная роль «читатель» — запись запрещена везде",
  },
  "members.username_placeholder": { en: "Username", ru: "Логин пользователя" },
  "members.no_spaces": { en: "Create a space first", ru: "Сначала создайте пространство" },
  "members.space": { en: "Space", ru: "Пространство" },
  "members.space_hint": {
    en: "An owner runs the space, a member creates lists, a viewer only looks.",
    ru: "Владелец управляет пространством, участник создаёт списки, читатель только смотрит.",
  },
  "members.list": { en: "List", ru: "Список" },
  "members.list_hint": {
    en: "List access is granted separately from the space: an editor changes tasks, a viewer only looks.",
    ru: "Доступ к списку выдаётся отдельно от пространства: редактор меняет задачи, читатель только смотрит.",
  },
  "members.pick_list": { en: "— pick a list —", ru: "— выберите список —" },
  "members.private": { en: "private", ru: "приватный" },
  "members.unassigned": { en: "Nobody assigned", ru: "Без исполнителя" },
  "members.role.owner": { en: "owner", ru: "владелец" },
  "members.role.member": { en: "member", ru: "участник" },
  "members.role.editor": { en: "editor", ru: "редактор" },
  "members.role.viewer": { en: "viewer", ru: "читатель" },

  "share.title": { en: "Public links", ru: "Публичные ссылки" },

  "focus.stats_title": { en: "Focus statistics", ru: "Статистика фокуса" },
  "focus.week": { en: "Week", ru: "Неделя" },
  "focus.month": { en: "Month", ru: "Месяц" },
  "focus.total": { en: "Total", ru: "Всего" },
  "focus.sessions": { en: "Sessions", ru: "Сессий" },
  "focus.average": { en: "Average", ru: "В среднем" },
  "focus.minutes_short": { en: "min", ru: "мин" },
  "focus.stats_hint": {
    en: "Only finished focus sessions are counted. A session you are in right now joins the total once you stop it.",
    ru: "Считаются только завершённые сессии фокуса. Текущая попадёт в сумму после остановки.",
  },

  "lists.rename": { en: "Rename", ru: "Переименовать" },
  "lists.archive_confirm": {
    en: "Archive this list? You can restore it from the archive later.",
    ru: "Архивировать список? Позже его можно вернуть из архива.",
  },
  "lists.archive_cascade_hint": {
    en: "The tasks inside go to the archive together with the list. Restoring the list brings them back.",
    ru: "Задачи внутри уходят в архив вместе со списком. При восстановлении списка они вернутся.",
  },
  "lists.reorder_hint": {
    en: "Drag a list to reorder it. The order is shared with everyone who can see the space.",
    ru: "Перетащите список, чтобы изменить порядок. Порядок общий для всех, кто видит пространство.",
  },
  "lists.reorder_partial": {
    en: "Not every list could be moved. Reload the page to see the order the server kept.",
    ru: "Не все списки удалось переставить. Обновите страницу, чтобы увидеть сохранённый порядок.",
  },

  "workflow.title": { en: "Task statuses", ru: "Статусы задач" },
  "workflow.empty": { en: "No custom statuses yet.", ru: "Своих статусов пока нет." },
  "workflow.builtin": { en: "Built-in status", ru: "Встроенный статус" },
  "workflow.builtin_hint": {
    en: "Built-in statuses cannot be removed: the server adds them back, and done is what closes a task.",
    ru: "Встроенные статусы убрать нельзя: их добавляет сервер, а done закрывает задачу.",
  },
  "workflow.custom_hint": {
    en: "Custom statuses come after the built-in ones. They become the kanban columns in the same order.",
    ru: "Свои статусы идут после встроенных — в том же порядке они станут колонками канбан-доски.",
  },
  "workflow.new_placeholder": {
    en: "For example: design, qa, blocked",
    ru: "Например: design, qa, blocked",
  },
  "workflow.move_up": { en: "Up", ru: "Выше" },
  "workflow.move_down": { en: "Down", ru: "Ниже" },
  "workflow.duplicate": { en: "That status already exists", ru: "Такой статус уже есть" },
  "workflow.too_long": {
    en: "That name is too long. {max} characters at most.",
    ru: "Слишком длинное название. Максимум {max} символов.",
  },
  "workflow.remove_hint": {
    en: "Removing a status does not move the tasks in it — move them out before you delete it.",
    ru: "Если убрать статус, задачи в нём не переносятся автоматически — переведите их до удаления.",
  },
  "workflow.owner_only": {
    en: "Only the space owner can change the statuses.",
    ru: "Изменять статусы может только владелец пространства.",
  },
}

const TARGETS = { "en-US": "en", "ru-RU": "ru" }

function serialise(obj) {
  const keys = Object.keys(obj).sort()
  const body = keys
    .map((key) => `  ${JSON.stringify(key)}: ${JSON.stringify(obj[key])}`)
    .join(",\n")
  return `{\n${body}\n}\n`
}

const check = process.argv.includes("--check")
let problems = 0

for (const [locale, field] of Object.entries(TARGETS)) {
  const file = join(LOCALES, `${locale}.json`)
  if (!existsSync(file)) {
    console.error(`${locale}: ${file} is missing`)
    problems++
    continue
  }
  const before = readFileSync(file, "utf8")
  const bundle = JSON.parse(before)
  const added = []
  const kept = []
  for (const [key, values] of Object.entries(KEYS)) {
    if (key in bundle) {
      if (bundle[key] !== values[field]) kept.push(key)
      continue
    }
    bundle[key] = values[field]
    added.push(key)
  }
  if (kept.length > 0) {
    console.log(`${locale}: left alone (already translated): ${kept.join(", ")}`)
  }
  const after = serialise(bundle)
  if (after === before) {
    console.log(`${locale}: ok`)
    continue
  }
  if (check) {
    console.error(`${locale}: ${added.length} key(s) missing: ${added.join(", ")}`)
    problems++
  } else {
    writeFileSync(file, after)
    console.log(`${locale}: added ${added.length} key(s)`)
  }
}

if (problems > 0) {
  console.error(check ? "screen keys: missing" : "screen keys: failed")
  process.exit(1)
}
console.log("screen keys: done")
console.log("next: python3 scripts/check_i18n.py && cd web && npx tsc --noEmit && npm run build")
