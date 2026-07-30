// Two read-only screens for data the server already recorded and nothing ever displayed.
//
//   * GET /api/admin/audit — migration 0013 added admin_audit and audit() writes to it on every
//     approval, role change, block, password reset, permanent delete and policy change. But no
//     screen ever read it back. An audit trail that cannot be read does not answer the question it
//     exists for ("who blocked this account, and when"), so it was doing no work at all.
//   * GET /api/focus/stats — focus sessions have always been stored with durations, and the
//     sidebar timer could start and stop them, but the totals were never shown anywhere. The user
//     could track time and never see the result.
//
// Both are new files rather than additions to views.tsx / extras.tsx, which are already ~100 KB
// and ~69 KB.

import { useEffect, useState } from "react"
import { api, type FocusStats } from "./api"
import { trOr } from "./i18n"

export type AuditEntry = {
  id: number
  actor_id: number | null
  actor_username: string
  action: string
  target_type: string
  target_id: number | null
  details: Record<string, unknown> | null
  ip: string
  created_at: string
}

// Mirrors the action constants in internal/api/audit.go. Kept as an explicit list so the filter
// offers exactly the actions the server can write, instead of a free-text field where a typo
// silently returns nothing.
const ACTIONS: Array<[string, string]> = [
  ["user.approve", "одобрение пользователя"],
  ["user.status", "смена статуса"],
  ["user.reset_password", "сброс пароля"],
  ["task.delete_permanent", "удаление задачи навсегда"],
  ["list.delete_permanent", "удаление списка навсегда"],
  ["space.delete_permanent", "удаление пространства навсегда"],
  ["setting.change", "изменение настройки"],
  ["locale.toggle", "переключение языка"],
]

const LIMITS = [50, 100, 200, 500]

const actionLabel = (action: string) => {
  const found = ACTIONS.find(([key]) => key === action)
  // An unknown action still renders as its raw name: a future action must never be invisible in
  // the log just because this list is out of date. setting.change uses an underscore in the
  // historical locale key, while the remaining action keys mirror their server names directly.
  const localeKey = action === "setting.change" ? "audit.action.setting_change" : `audit.action.${action}`
  return found ? trOr(localeKey, found[1]) : action
}

function formatWhen(value: string) {
  const d = new Date(value)
  if (isNaN(d.getTime())) return value
  return d.toLocaleString()
}

/** Admin-only viewer for the administrative audit trail. */
export function AuditLogCard() {
  const [entries, setEntries] = useState<AuditEntry[] | null>(null)
  const [action, setAction] = useState("")
  const [limit, setLimit] = useState(100)
  const [actorFilter, setActorFilter] = useState("")
  const [err, setErr] = useState("")
  const [busy, setBusy] = useState(false)

  async function load() {
    setBusy(true)
    setErr("")
    try {
      const q = new URLSearchParams({ limit: String(limit) })
      if (action) q.set("action", action)
      const r = await api.get(`/api/admin/audit?${q.toString()}`)
      setEntries(r.entries || [])
    } catch (e) {
      // Shown, never swallowed: an empty audit log and an unreadable audit log mean opposite
      // things, and confusing them would be worse here than anywhere else in the product.
      setEntries([])
      setErr((e as Error).message)
    } finally {
      setBusy(false)
    }
  }

  useEffect(() => { load() }, [action, limit])

  // Filtering by actor is done here, over the page already fetched, rather than through the
  // server's ?actor= parameter — that one takes a numeric user id, which an admin reading a log
  // of usernames does not have at hand.
  const shown = (entries || []).filter((e) =>
    !actorFilter || e.actor_username.toLowerCase().includes(actorFilter.trim().toLowerCase()))

  return (
    <div className="card">
      <div className="row" style={{ gap: 8, alignItems: "center", flexWrap: "wrap" }}>
        <b>{trOr("audit.title", "Журнал действий администраторов")}</b>
        <button className="ctrl-btn" disabled={busy} title={trOr("audit.refresh", "Обновить")}
          style={{ marginLeft: "auto" }} onClick={load}>
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><polyline points="23 4 23 10 17 10" /><path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10" /></svg>
        </button>
      </div>

      <p className="muted" style={{ fontSize: 13 }}>
        {trOr("audit.hint", "Записывается кто, что и когда сделал. Пароли и токены в журнал не попадают — только факт изменения.")}
      </p>

      <div className="row" style={{ gap: 6, flexWrap: "wrap", marginBottom: 10 }}>
        <select className="input" style={{ width: "auto" }} value={action}
          onChange={(e) => setAction(e.target.value)}>
          <option value="">{trOr("audit.all_actions", "Все действия")}</option>
          {ACTIONS.map(([key, label]) => (
            <option key={key} value={key}>{trOr(`audit.action.${key}`, label)}</option>
          ))}
        </select>
        <select className="input" style={{ width: "auto" }} value={limit}
          onChange={(e) => setLimit(Number(e.target.value))}>
          {LIMITS.map((n) => (
            <option key={n} value={n}>{n} {trOr("audit.records", "записей")}</option>
          ))}
        </select>
        <input className="input" style={{ maxWidth: 180 }} value={actorFilter}
          placeholder={trOr("audit.actor_placeholder", "Логин администратора")}
          onChange={(e) => setActorFilter(e.target.value)} />
      </div>

      {err && <div className="muted" style={{ color: "var(--danger, #c33)", marginBottom: 8 }}>{err}</div>}
      {entries === null && <div className="muted">{trOr("audit.loading", "Загрузка…")}</div>}
      {entries !== null && shown.length === 0 && !err && (
        <div className="muted">{trOr("audit.empty", "Записей нет")}</div>
      )}

      {shown.map((e) => {
        const hasDetails = e.details && Object.keys(e.details).length > 0
        return (
          <div key={e.id} style={{ padding: "8px 0", borderTop: "1px solid var(--border, #eee)" }}>
            <div className="row" style={{ gap: 8, alignItems: "baseline", flexWrap: "wrap" }}>
              <span className="muted" style={{ fontSize: 12, minWidth: 140 }}>{formatWhen(e.created_at)}</span>
              {/* actor_username is a snapshot taken at the time of the action, so it stays
                  readable after the account is renamed or deleted. */}
              <b>@{e.actor_username}</b>
              <span>{actionLabel(e.action)}</span>
              {e.target_type && (
                <span className="badge">
                  {e.target_type}{e.target_id !== null ? ` #${e.target_id}` : ""}
                </span>
              )}
              {e.ip && <span className="muted" style={{ fontSize: 12, marginLeft: "auto" }}>{e.ip}</span>}
            </div>
            {hasDetails && (
              <details style={{ marginTop: 4 }}>
                <summary className="muted" style={{ fontSize: 12, cursor: "pointer" }}>
                  {trOr("audit.details", "Подробности")}
                </summary>
                <pre style={{ fontSize: 12, overflowX: "auto", margin: "6px 0 0" }}>
                  {JSON.stringify(e.details, null, 2)}
                </pre>
              </details>
            )}
          </div>
        )
      })}
    </div>
  )
}

function formatDuration(totalSeconds: number) {
  const h = Math.floor(totalSeconds / 3600)
  const m = Math.round((totalSeconds % 3600) / 60)
  if (h === 0 && m === 0) return `0 ${trOr("focus.minutes_short", "мин")}`
  if (h === 0) return `${m} ${trOr("focus.minutes_short", "мин")}`
  return `${h} ${trOr("focus.hours_short", "ч")} ${m} ${trOr("focus.minutes_short", "мин")}`
}

/** The caller's own focused time for the last week or month. */
export function FocusStatsCard() {
  const [period, setPeriod] = useState<"week" | "month">("week")
  const [stats, setStats] = useState<FocusStats | null>(null)
  const [err, setErr] = useState("")

  useEffect(() => {
    let alive = true
    setStats(null)
    setErr("")
    api.get(`/api/focus/stats?period=${period}`)
      .then((r) => { if (alive) setStats(r) })
      .catch((e) => { if (alive) setErr((e as Error).message) })
    return () => { alive = false }
  }, [period])

  // Nothing tracked yet: a card reading "0 min / 0 sessions" is noise on the main screen for
  // everyone who does not use the timer, so it hides itself instead.
  if (!err && stats && stats.sessions === 0) return null

  return (
    <div className="card">
      <div className="row" style={{ gap: 8, alignItems: "center", flexWrap: "wrap" }}>
        <b>{trOr("focus.stats_title", "Время в фокусе")}</b>
        <div className="row" style={{ gap: 6, marginLeft: "auto" }}>
          <button className={"btn" + (period === "week" ? " active" : "")}
            onClick={() => setPeriod("week")}>{trOr("focus.week", "Неделя")}</button>
          <button className={"btn" + (period === "month" ? " active" : "")}
            onClick={() => setPeriod("month")}>{trOr("focus.month", "Месяц")}</button>
        </div>
      </div>

      {err && <div className="muted" style={{ color: "var(--danger, #c33)" }}>{err}</div>}
      {!err && stats === null && <div className="muted">{trOr("focus.loading", "Загрузка…")}</div>}

      {!err && stats && (
        <div className="row" style={{ gap: 20, flexWrap: "wrap", marginTop: 6 }}>
          <div>
            <div style={{ fontSize: 22, fontWeight: 600 }}>{formatDuration(stats.total_seconds)}</div>
            <div className="muted" style={{ fontSize: 12 }}>{trOr("focus.total", "всего")}</div>
          </div>
          <div>
            <div style={{ fontSize: 22, fontWeight: 600 }}>{stats.sessions}</div>
            <div className="muted" style={{ fontSize: 12 }}>{trOr("focus.sessions", "сессий")}</div>
          </div>
          <div>
            <div style={{ fontSize: 22, fontWeight: 600 }}>
              {formatDuration(Math.round(stats.total_seconds / stats.sessions))}
            </div>
            <div className="muted" style={{ fontSize: 12 }}>{trOr("focus.average", "в среднем за сессию")}</div>
          </div>
        </div>
      )}

      <p className="muted" style={{ fontSize: 12, marginBottom: 0 }}>
        {/* Only closed sessions are summed server-side, so the currently running timer is
            deliberately not part of these numbers. */}
        {trOr("focus.stats_hint", "Считаются только завершённые сессии — идущий сейчас таймер сюда не входит.")}
      </p>
    </div>
  )
}
