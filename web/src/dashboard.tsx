// Space dashboard: how a space is doing over a week or a month, on one screen.
//
// The parts were all reachable already - pulse for right now, workload for who is loaded, stats
// for what was finished - but each on its own tab, each with its own period, so comparing them
// meant remembering numbers while clicking. This reads one aggregated response
// (GET /api/spaces/{id}/dashboard) and lays the same facts out side by side.
//
// A new file rather than more of views.tsx / extras.tsx, which are already ~20 KB and ~69 KB,
// following the precedent set by insights.tsx and timeline.tsx.

import { useEffect, useState } from "react"
import { api } from "./api"
import { tr } from "./i18n"

// tr() with an inline fallback - the same helper, for the same reason, as insights.tsx,
// members.tsx and sharing.tsx: the screen is legible before its keys land in all 13 locales.
const t = (key: string, fallback: string) => tr(key) || fallback

type Summary = {
  open_count: number
  overdue_count: number
  done_in_period: number
  avg_close_hours: number
  open_weight: number
}

type StatusRow = { status: string; count: number }

type PersonRow = {
  user_id: number | null
  username: string | null
  name: string | null
  open_count: number
  overdue_count: number
}

type DayRow = { day: string; created: number; done: number }

type OverdueRow = {
  id: number
  title: string
  list_id: number
  list_name: string
  due_at: string
  days_late: number
  assignee: string | null
}

export type Dashboard = {
  period: "week" | "month"
  days: number
  summary: Summary
  by_status: StatusRow[]
  by_assignee: PersonRow[]
  series: DayRow[]
  top_overdue: OverdueRow[]
}

/** The label the space tab strip uses. Exported so views.tsx does not need its own tr() key. */
export const dashboardTabLabel = () => t("dashboard.title", "Дашборд")

// Built-in statuses get their existing translated names; anything else is a custom workflow
// status and is shown exactly as the space defined it. Inventing a label for a word the owner
// chose themselves would be worse than showing it verbatim.
const STATUS_KEYS: Record<string, string> = {
  open: "task.status.open",
  in_progress: "task.status.in_progress",
  review: "task.status.review",
  done: "task.status.done",
}
const statusLabel = (status: string) =>
  STATUS_KEYS[status] ? tr(STATUS_KEYS[status]) || status : status

// Status colours reuse the board tokens, so a status is the same colour here as on the kanban.
const statusColor = (status: string) =>
  STATUS_KEYS[status] ? `var(--st-${status === "in_progress" ? "progress" : status})` : "var(--accent)"

function formatHours(hours: number) {
  if (!hours || hours <= 0) return "—"
  if (hours < 1) return `${Math.max(1, Math.round(hours * 60))} ${t("focus.minutes_short", "мин")}`
  if (hours < 48) return `${Math.round(hours)} ${t("dashboard.hours_short", "ч")}`
  return `${Math.round(hours / 24)} ${t("dashboard.days_short", "дн")}`
}

// "07.21" - short enough that a month of them fits under the chart without rotating labels.
function shortDay(iso: string) {
  const parts = iso.split("-")
  return parts.length === 3 ? `${parts[2]}.${parts[1]}` : iso
}

function Metric({ value, caption, tone }: { value: string; caption: string; tone?: string }) {
  return (
    <div style={{ minWidth: 92 }}>
      <div style={{ fontSize: 22, fontWeight: 600, color: tone }}>{value}</div>
      <div className="muted" style={{ fontSize: 12 }}>{caption}</div>
    </div>
  )
}

/** One horizontal bar with a label and a count - used for both statuses and people. */
function BarRow({ label, count, max, color, note }: {
  label: string
  count: number
  max: number
  color: string
  note?: string
}) {
  const pct = max > 0 ? Math.round((count / max) * 100) : 0
  return (
    <div style={{ padding: "4px 0" }}>
      <div className="row" style={{ gap: 8, alignItems: "baseline", fontSize: 13 }}>
        <span style={{ minWidth: 0, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>{label}</span>
        {note && <span className="muted" style={{ fontSize: 12 }}>{note}</span>}
        <b style={{ marginLeft: "auto" }}>{count}</b>
      </div>
      <div style={{ height: 6, borderRadius: 3, background: "var(--border)", marginTop: 3 }}>
        <div style={{ height: 6, borderRadius: 3, width: `${pct}%`, background: color, transition: "var(--transition)" }} />
      </div>
    </div>
  )
}

/**
 * Created (outline) against closed (filled) per day.
 *
 * Both series share one scale, which is the entire point: two charts with independent axes can
 * make a day that created twice what it closed look balanced.
 */
function FlowChart({ series }: { series: DayRow[] }) {
  const max = Math.max(1, ...series.map((d) => Math.max(d.created, d.done)))
  const width = Math.max(240, series.length * 26)
  const height = 96
  const step = width / series.length
  const barW = Math.min(9, step / 2.6)

  return (
    <svg viewBox={`0 0 ${width} ${height + 18}`} style={{ width: "100%", height: 120 }} role="img">
      <line x1={0} y1={height} x2={width} y2={height} stroke="var(--border)" strokeWidth={1} />
      {series.map((d, i) => {
        const cx = i * step + step / 2
        const hc = (d.created / max) * (height - 6)
        const hd = (d.done / max) * (height - 6)
        // Every seventh label on a month, every day on a week: enough to orient, never a smear.
        const showLabel = series.length <= 10 || i % 7 === 0 || i === series.length - 1
        return (
          <g key={d.day}>
            <rect x={cx - barW - 1} y={height - hc} width={barW} height={hc}
              fill="none" stroke="var(--accent)" strokeWidth={1.2} rx={1} />
            <rect x={cx + 1} y={height - hd} width={barW} height={hd}
              fill="var(--st-done)" rx={1} />
            {showLabel && (
              <text x={cx} y={height + 13} textAnchor="middle" fontSize={9} fill="var(--text-muted)">
                {shortDay(d.day)}
              </text>
            )}
            <title>{`${d.day}: +${d.created} / -${d.done}`}</title>
          </g>
        )
      })}
    </svg>
  )
}

/**
 * The space dashboard.
 *
 * onOpenTask is the same callback the timeline uses, so an overdue task listed here opens the
 * task modal in place instead of sending the reader off to find it in a list.
 */
export function DashboardPanel({ spaceId, onOpenTask }: {
  spaceId: number
  onOpenTask?: (id: number) => void
}) {
  const [period, setPeriod] = useState<"week" | "month">("week")
  const [data, setData] = useState<Dashboard | null>(null)
  const [err, setErr] = useState("")
  // Lite mode asks for fewer painted pixels; the numbers stay, the bars go.
  const lite = document.documentElement.getAttribute("data-visual") === "lite"

  useEffect(() => {
    let alive = true
    setData(null)
    setErr("")
    api.get(`/api/spaces/${spaceId}/dashboard?period=${period}`)
      .then((r) => { if (alive) setData(r) })
      .catch((e) => { if (alive) setErr((e as Error).message) })
    return () => { alive = false }
  }, [spaceId, period])

  const statusMax = Math.max(1, ...(data?.by_status || []).map((s) => s.count))
  const personMax = Math.max(1, ...(data?.by_assignee || []).map((p) => p.open_count))
  const nothing = data && data.summary.open_count === 0 && data.summary.done_in_period === 0

  return (
    <div>
      <div className="row" style={{ gap: 8, alignItems: "center", flexWrap: "wrap", marginBottom: 10 }}>
        <b>{dashboardTabLabel()}</b>
        <div className="row" style={{ gap: 6, marginLeft: "auto" }}>
          <button className={"btn" + (period === "week" ? " active" : "")}
            onClick={() => setPeriod("week")}>{tr("stats.week")}</button>
          <button className={"btn" + (period === "month" ? " active" : "")}
            onClick={() => setPeriod("month")}>{tr("stats.month")}</button>
        </div>
      </div>

      {err && <div className="muted" style={{ color: "var(--due-overdue)" }}>{err}</div>}
      {!err && data === null && <div className="muted">{tr("common.loading")}</div>}
      {!err && nothing && (
        <div className="muted">{t("dashboard.empty", "За этот период здесь пока нечего показать.")}</div>
      )}

      {!err && data && !nothing && (
        <>
          <div className="row" style={{ gap: 20, flexWrap: "wrap", marginBottom: 14 }}>
            <Metric value={String(data.summary.open_count)} caption={tr("workload.open")} />
            <Metric value={String(data.summary.overdue_count)} caption={tr("workload.overdue")}
              tone={data.summary.overdue_count > 0 ? "var(--due-overdue)" : undefined} />
            <Metric value={String(data.summary.done_in_period)}
              caption={t("dashboard.done_in_period", "закрыто за период")} />
            <Metric value={formatHours(data.summary.avg_close_hours)}
              caption={t("dashboard.avg_close", "среднее время закрытия")} />
          </div>

          <div style={{ marginBottom: 14 }}>
            <div className="row" style={{ gap: 10, alignItems: "baseline", flexWrap: "wrap" }}>
              <b style={{ fontSize: 13 }}>{t("dashboard.flow", "Создано и закрыто")}</b>
              <span className="muted" style={{ fontSize: 12 }}>
                {t("dashboard.flow_created", "создано")} · {t("dashboard.flow_done", "закрыто")}
              </span>
            </div>
            {lite ? (
              <div className="muted" style={{ fontSize: 12, marginTop: 4 }}>
                {data.series.map((d) => `${shortDay(d.day)} +${d.created}/-${d.done}`).join("   ")}
              </div>
            ) : (
              <FlowChart series={data.series} />
            )}
          </div>

          <div style={{ marginBottom: 14 }}>
            <b style={{ fontSize: 13 }}>{t("dashboard.by_status", "Открытые задачи по статусам")}</b>
            {data.by_status.length === 0 && <div className="muted" style={{ fontSize: 13 }}>{tr("workload.empty")}</div>}
            {data.by_status.map((s) => (
              <BarRow key={s.status} label={statusLabel(s.status)} count={s.count}
                max={statusMax} color={statusColor(s.status)} />
            ))}
          </div>

          <div style={{ marginBottom: 14 }}>
            <b style={{ fontSize: 13 }}>{t("dashboard.by_person", "Нагрузка по людям")}</b>
            {data.by_assignee.map((p) => (
              <BarRow key={p.user_id ?? "none"}
                label={p.name || p.username || tr("workload.unassigned")}
                count={p.open_count}
                max={personMax}
                color={p.user_id === null ? "var(--text-muted)" : "var(--accent)"}
                note={p.overdue_count > 0 ? `${tr("workload.overdue").toLowerCase()}: ${p.overdue_count}` : undefined} />
            ))}
          </div>

          {data.top_overdue.length > 0 && (
            <div>
              <b style={{ fontSize: 13 }}>{t("dashboard.top_overdue", "Дольше всех просрочены")}</b>
              {data.top_overdue.map((task) => (
                <div key={task.id} className="task-row" style={{ cursor: onOpenTask ? "pointer" : undefined }}
                  onClick={() => onOpenTask?.(task.id)}>
                  <span className="task-title">{task.title}</span>
                  <span className="muted" style={{ fontSize: 12 }}>{task.list_name}</span>
                  {task.assignee && <span className="muted" style={{ fontSize: 12 }}>{task.assignee}</span>}
                  <span style={{ color: "var(--due-overdue)", fontSize: 12, marginLeft: "auto" }}>
                    {t("dashboard.days_late", "просрочено на {n} дн.").replace("{n}", String(task.days_late))}
                  </span>
                </div>
              ))}
            </div>
          )}
        </>
      )}
    </div>
  )
}
