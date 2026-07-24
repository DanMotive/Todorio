// Timeline / Gantt view (spec section 12).
//
// Deliberately built from CSS grid + absolutely positioned bars rather than a charting
// library: the project ships as a single self-contained binary and adding a heavyweight Gantt
// dependency for one view would bloat the bundle for everyone. Dependency arrows are drawn in
// one SVG overlay sized to the chart body.
//
// No emoji anywhere — status is carried by bar color and the same drawn icons used elsewhere.

import { useEffect, useMemo, useState } from "react"
import { api, type Timeline, type TimelineItem } from "./api"
import { tr, getFormattingLocale } from "./i18n"
import { IconAlertCircle, IconArrowLeft, IconLock } from "./icons"

const DAY_MS = 86400000

// Zoom levels: how many pixels one day occupies. Day view is for a sprint, month view for a
// quarter — the same window can be read at either density.
const ZOOM = { day: 34, week: 12, month: 4 } as const
type Zoom = keyof typeof ZOOM

const ROW_H = 34
// Label column width. 210px is right on a desktop but swallows most of a ~390px phone
// viewport, leaving no room for the chart itself — so it shrinks on narrow screens.
const LABEL_W_WIDE = 210
const LABEL_W_NARROW = 116

function startOfDay(d: Date) {
  return new Date(d.getFullYear(), d.getMonth(), d.getDate())
}
function iso(d: Date) {
  // Local calendar date, not UTC: toISOString() would shift the day for anyone east/west of
  // UTC and silently offset the whole chart window.
  const m = String(d.getMonth() + 1).padStart(2, "0")
  const day = String(d.getDate()).padStart(2, "0")
  return `${d.getFullYear()}-${m}-${day}`
}

export function TimelineView({ spaceId, listId, onOpenTask }: {
  spaceId: number
  listId?: number
  onOpenTask?: (taskId: number) => void
}) {
  const [data, setData] = useState<Timeline | null>(null)
  const [zoom, setZoom] = useState<Zoom>("week")
  const [error, setError] = useState("")
  // Window anchor: the first day of the month currently scrolled to.
  const [anchor, setAnchor] = useState(() => {
    const d = new Date()
    return new Date(d.getFullYear(), d.getMonth(), 1)
  })
  const [monthSpan, setMonthSpan] = useState(3)
  const [narrow, setNarrow] = useState(
    () => typeof window.matchMedia === "function" && window.matchMedia("(max-width: 860px)").matches)
  useEffect(() => {
    if (typeof window.matchMedia !== "function") return
    const mq = window.matchMedia("(max-width: 860px)")
    const onChange = () => setNarrow(mq.matches)
    mq.addEventListener("change", onChange)
    return () => mq.removeEventListener("change", onChange)
  }, [])
  const LABEL_W = narrow ? LABEL_W_NARROW : LABEL_W_WIDE

  const from = anchor
  const to = useMemo(
    () => new Date(anchor.getFullYear(), anchor.getMonth() + monthSpan, 1),
    [anchor, monthSpan],
  )

  useEffect(() => {
    setError("")
    const q = new URLSearchParams({ from: iso(from), to: iso(to) })
    if (listId) q.set("list_id", String(listId))
    api.get(`/api/spaces/${spaceId}/timeline?${q}`)
      .then(setData)
      .catch((e) => setError((e as Error).message))
  }, [spaceId, listId, anchor, monthSpan])

  const dayW = ZOOM[zoom]
  const totalDays = Math.max(1, Math.round((to.getTime() - from.getTime()) / DAY_MS))
  const chartW = totalDays * dayW

  // Day columns, plus where each month boundary falls so the header can label them.
  const months = useMemo(() => {
    const out: { label: string; offsetDays: number; days: number }[] = []
    let cur = new Date(from)
    while (cur < to) {
      const next = new Date(cur.getFullYear(), cur.getMonth() + 1, 1)
      const end = next < to ? next : to
      out.push({
        label: cur.toLocaleDateString(getFormattingLocale(), { month: "short", year: "2-digit" }),
        offsetDays: Math.round((cur.getTime() - from.getTime()) / DAY_MS),
        days: Math.round((end.getTime() - cur.getTime()) / DAY_MS),
      })
      cur = next
    }
    return out
  }, [from, to])

  const items = data?.items ?? []
  // Row index per task id — needed both for bar placement and for arrow endpoints.
  const rowOf = useMemo(() => {
    const m = new Map<number, number>()
    items.forEach((it, i) => m.set(it.id, i))
    return m
  }, [items])

  function barGeom(it: TimelineItem) {
    const s = startOfDay(new Date(it.start)).getTime()
    const e = startOfDay(new Date(it.end)).getTime()
    const startDay = Math.round((s - from.getTime()) / DAY_MS)
    // Inclusive of the end day, and never narrower than one day so a same-day task is visible.
    const spanDays = Math.max(1, Math.round((e - s) / DAY_MS) + 1)
    // Clamp to the window: a bar may legitimately start before or end after it.
    const left = Math.max(0, startDay)
    const right = Math.min(totalDays, startDay + spanDays)
    return {
      left: left * dayW,
      width: Math.max(dayW * 0.6, (right - left) * dayW),
      clippedStart: startDay < 0,
      clippedEnd: startDay + spanDays > totalDays,
    }
  }

  // Day ticks. At day zoom every date is labelled; at week zoom only Mondays (a number every
  // 12px would be an unreadable smear); at month zoom the month header alone carries the scale.
  const ticks = useMemo(() => {
    if (zoom === "month") return []
    const out: { offset: number; label: string; weekStart: boolean }[] = []
    for (let i = 0; i < totalDays; i++) {
      const d = new Date(from.getFullYear(), from.getMonth(), from.getDate() + i)
      const monday = d.getDay() === 1
      if (zoom === "day" || monday) {
        out.push({ offset: i * dayW, label: String(d.getDate()), weekStart: monday })
      }
    }
    return out
  }, [from, totalDays, dayW, zoom])

  const todayOffset = useMemo(() => {
    const t = startOfDay(new Date()).getTime()
    if (t < from.getTime() || t >= to.getTime()) return null
    return Math.round((t - from.getTime()) / DAY_MS) * dayW
  }, [from, to, dayW])

  return (
    <div className="timeline">
      <div className="row timeline-toolbar" style={{ gap: 8, flexWrap: "wrap", marginBottom: 8 }}>
        <button className="nav-btn" title={tr("timeline.prev")}
          onClick={() => setAnchor(new Date(anchor.getFullYear(), anchor.getMonth() - 1, 1))}>
          <IconArrowLeft size={14} />
        </button>
        <b style={{ minWidth: 150, textAlign: "center" }}>
          {from.toLocaleDateString(getFormattingLocale(), { month: "long", year: "numeric" })}
          {monthSpan > 1 && " …"}
        </b>
        <button className="nav-btn" title={tr("timeline.next")}
          onClick={() => setAnchor(new Date(anchor.getFullYear(), anchor.getMonth() + 1, 1))}>
          <span style={{ display: "inline-block", transform: "rotate(180deg)" }}><IconArrowLeft size={14} /></span>
        </button>
        <button className="nav-btn" onClick={() => {
          const d = new Date()
          setAnchor(new Date(d.getFullYear(), d.getMonth(), 1))
        }}>{tr("timeline.today")}</button>

        <label className="row muted" style={{ gap: 6, fontSize: 12, marginLeft: "auto" }}>
          {tr("timeline.zoom")}
          <select className="input" style={{ width: "auto", padding: "2px 6px", fontSize: 12 }}
            value={zoom} onChange={(e) => setZoom(e.target.value as Zoom)}>
            <option value="day">{tr("timeline.zoom_day")}</option>
            <option value="week">{tr("timeline.zoom_week")}</option>
            <option value="month">{tr("timeline.zoom_month")}</option>
          </select>
        </label>
        <label className="row muted" style={{ gap: 6, fontSize: 12 }}>
          {tr("timeline.span")}
          <select className="input" style={{ width: "auto", padding: "2px 6px", fontSize: 12 }}
            value={monthSpan} onChange={(e) => setMonthSpan(Number(e.target.value))}>
            <option value={1}>1</option>
            <option value={3}>3</option>
            <option value={6}>6</option>
            <option value={12}>12</option>
          </select>
        </label>
      </div>

      {error && <p className="error-text">{error}</p>}

      {data && items.length === 0 && !error && (
        <p className="muted">{tr("timeline.empty")}</p>
      )}

      {items.length > 0 && (
        <div className="timeline-scroll">
          <div className="timeline-inner" style={{ width: LABEL_W + chartW }}>
            {/* header: month labels over day ticks */}
            <div className="timeline-head" style={{ height: 46 }}>
              <div className="timeline-head-label" style={{ width: LABEL_W }} />
              <div style={{ position: "relative", width: chartW }}>
                {months.map((m) => (
                  <div key={m.label + m.offsetDays} className="timeline-month"
                    style={{ left: m.offsetDays * dayW, width: m.days * dayW }}>
                    {m.label}
                  </div>
                ))}
                {ticks.map((t) => (
                  <div key={t.offset} className={"timeline-tick" + (t.weekStart ? " is-week" : "")}
                    style={{ left: t.offset, width: dayW }}>
                    {t.label}
                  </div>
                ))}
              </div>
            </div>

            <div className="timeline-body" style={{ position: "relative" }}>
              {/* vertical gridlines: one per labelled tick, so a bar's extent can be read off
                  the header dates instead of being guessed. */}
              {ticks.map((t) => (
                <div key={"g" + t.offset} className={"timeline-grid" + (t.weekStart ? " is-week" : "")}
                  style={{ left: LABEL_W + t.offset }} />
              ))}

              {/* today marker spans the full height in front of the gridlines */}
              {todayOffset !== null && (
                <div className="timeline-today" style={{ left: LABEL_W + todayOffset }} />
              )}

              {/* dependency arrows */}
              <svg className="timeline-links" width={LABEL_W + chartW} height={items.length * ROW_H}>
                {(data?.links ?? []).map((lk, i) => {
                  const fromRow = rowOf.get(lk.from)
                  const toRow = rowOf.get(lk.to)
                  if (fromRow === undefined || toRow === undefined) return null
                  const a = items[fromRow], b = items[toRow]
                  const ga = barGeom(a), gb = barGeom(b)
                  const x1 = LABEL_W + ga.left + ga.width
                  const y1 = fromRow * ROW_H + ROW_H / 2
                  const x2 = LABEL_W + gb.left
                  const y2 = toRow * ROW_H + ROW_H / 2
                  // Orthogonal elbow: out of the blocker's right edge, across, into the
                  // blocked task's left edge. Reads more clearly than a diagonal at density.
                  const midX = Math.max(x1 + 8, x2 - 8)
                  return (
                    <g key={i} className="timeline-link">
                      <path d={`M ${x1} ${y1} H ${midX} V ${y2} H ${x2}`} fill="none" />
                      <path d={`M ${x2} ${y2} l -5 -3.5 l 0 7 z`} className="timeline-arrowhead" />
                    </g>
                  )
                })}
              </svg>

              {items.map((it, i) => {
                const g = barGeom(it)
                const cls = [
                  "timeline-bar",
                  it.done ? "is-done" : "",
                  it.overdue ? "is-overdue" : "",
                  it.implied ? "is-implied" : "",
                  g.clippedStart ? "clip-start" : "",
                  g.clippedEnd ? "clip-end" : "",
                ].filter(Boolean).join(" ")
                const range = it.implied
                  ? tr("timeline.implied")
                  : `${new Date(it.start).toLocaleDateString(getFormattingLocale())} — ${new Date(it.end).toLocaleDateString(getFormattingLocale())}`
                return (
                  <div key={it.id} className="timeline-row" style={{ height: ROW_H }}>
                    <div className="timeline-label" style={{ width: LABEL_W }} title={it.title}>
                      {it.overdue && <IconAlertCircle size={11} style={{ color: "var(--due-overdue)", flexShrink: 0 }} />}
                      <span className="timeline-label-text">{it.title}</span>
                      {!listId && !narrow && <span className="muted timeline-list">{it.list_name}</span>}
                    </div>
                    <div style={{ position: "relative", width: chartW, height: ROW_H }}>
                      <div className={cls}
                        style={{ left: g.left, width: g.width, top: 6 }}
                        title={`${it.title}\n${range}\n${it.progress}%${it.assignee ? "\n@" + it.assignee : ""}`}
                        onClick={() => onOpenTask?.(it.id)}>
                        {/* progress fill inside the bar */}
                        <span className="timeline-bar-fill" style={{ width: `${it.progress}%` }} />
                        <span className="timeline-bar-text">
                          {it.assignee ? `@${it.assignee}` : ""}{it.progress > 0 ? ` ${it.progress}%` : ""}
                        </span>
                      </div>
                    </div>
                  </div>
                )
              })}
            </div>
          </div>
        </div>
      )}

      {/* Tasks with no dates can't be plotted — say so instead of quietly omitting them. */}
      {!!data?.unscheduled && (
        <p className="muted row" style={{ gap: 5, marginTop: 8, fontSize: 13 }}>
          <IconLock size={12} />
          {tr("timeline.unscheduled").replace("{count}", String(data.unscheduled))}
        </p>
      )}
      {items.some((i) => i.implied) && (
        <p className="muted" style={{ marginTop: 4, fontSize: 12 }}>{tr("timeline.implied_hint")}</p>
      )}
    </div>
  )
}
