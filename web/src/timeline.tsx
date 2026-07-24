// Timeline / Gantt view (spec section 12).
//
// Deliberately built from CSS grid + absolutely positioned bars rather than a charting
// library: the project ships as a single self-contained binary and adding a heavyweight Gantt
// dependency for one view would bloat the bundle for everyone. Dependency arrows are drawn in
// one SVG overlay sized to the chart body.
//
// No emoji anywhere — status is carried by bar color and the same drawn icons used elsewhere.

import { useCallback, useEffect, useMemo, useRef, useState } from "react"
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

// Duration of a bar in whole days, inclusive of both ends — a task starting and ending on the
// same day is a 1-day bar, not a 0-day one. Shared by the resting geometry and the live drag
// preview so a dragged bar is never a different width than the one it started as.
function spanDaysOf(it: Pick<TimelineItem, "start" | "end">): number {
  const s = startOfDay(new Date(it.start)).getTime()
  const e = startOfDay(new Date(it.end)).getTime()
  return Math.max(1, Math.round((e - s) / DAY_MS) + 1)
}

// Geometry for a bar spanning [startDay, endDay] (both inclusive, as day offsets from the
// chart's `from` anchor), clamped to the visible window. Shared by the resting position
// (derived from real dates) and the live drag preview (derived from in-progress pointer math)
// so dragging never disagrees with the final rendered position by a rounding hair.
function daysToGeom(startDay: number, endDay: number, totalDays: number, dayW: number) {
  const spanDays = Math.max(1, endDay - startDay + 1)
  const left = Math.max(0, startDay)
  const right = Math.min(totalDays, startDay + spanDays)
  return {
    left: left * dayW,
    width: Math.max(dayW * 0.6, (right - left) * dayW),
    clippedStart: startDay < 0,
    clippedEnd: startDay + spanDays > totalDays,
  }
}

// Critical path via a classic CPM forward/backward pass over whatever's currently loaded in
// the chart — not the whole project, since that's all that's available client-side and
// widening the fetch just to compute this would be surprising.
//
// tasks.blocked_by is a free-form array with no DB constraint against cycles (see ops.go's own
// "dependency cycles" integrity check, which exists for exactly this reason). A naive
// longest-path walk would recurse forever on one, so this topologically sorts with Kahn's
// algorithm first: anything left over once the queue drains is part of a cycle and is excluded
// from the CPM math rather than risking an infinite loop or a silently wrong slack value.
function computeCriticalPath(items: TimelineItem[], links: { from: number; to: number }[]) {
  const ids = new Set(items.map((it) => it.id))
  const dur = new Map(items.map((it) => [it.id, spanDaysOf(it)]))
  const succ = new Map<number, number[]>(items.map((it) => [it.id, []]))
  const pred = new Map<number, number[]>(items.map((it) => [it.id, []]))
  for (const lk of links) {
    if (!ids.has(lk.from) || !ids.has(lk.to)) continue
    succ.get(lk.from)!.push(lk.to)
    pred.get(lk.to)!.push(lk.from)
  }

  const indeg = new Map([...ids].map((id) => [id, pred.get(id)!.length]))
  const queue: number[] = [...ids].filter((id) => indeg.get(id) === 0)
  const order: number[] = []
  while (queue.length) {
    const id = queue.shift()!
    order.push(id)
    for (const next of succ.get(id)!) {
      indeg.set(next, indeg.get(next)! - 1)
      if (indeg.get(next) === 0) queue.push(next)
    }
  }
  const acyclic = new Set(order)

  const ES = new Map<number, number>()
  const EF = new Map<number, number>()
  for (const id of order) {
    const preds = pred.get(id)!.filter((p) => acyclic.has(p))
    const es = preds.length ? Math.max(...preds.map((p) => EF.get(p)!)) : 0
    ES.set(id, es)
    EF.set(id, es + dur.get(id)!)
  }
  const total = order.length ? Math.max(...order.map((id) => EF.get(id)!)) : 0

  const LS = new Map<number, number>()
  const LF = new Map<number, number>()
  for (let i = order.length - 1; i >= 0; i--) {
    const id = order[i]
    const succs = succ.get(id)!.filter((s) => acyclic.has(s))
    const lf = succs.length ? Math.min(...succs.map((s) => LS.get(s)!)) : total
    LF.set(id, lf)
    LS.set(id, lf - dur.get(id)!)
  }

  const critical = new Set<number>()
  for (const id of order) {
    if (ES.get(id) === LS.get(id)) critical.add(id)
  }
  return { critical, hasCycle: acyclic.size < ids.size }
}

type DragMode = "move" | "resize-start" | "resize-end"
// curStartDay/curEndDay are the live values as the pointer moves, read directly by endDrag —
// deliberately not sourced from React state, so completing the drag never depends on a state
// update having been committed and re-rendered before pointerup fires.
type DragState = {
  id: number
  mode: DragMode
  pointerId: number
  startClientX: number
  origStartDay: number
  origEndDay: number
  curStartDay: number
  curEndDay: number
}
type Preview = { id: number; startDay: number; endDay: number }

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
  // Critical path (spec section 12 Gantt requirement) is opt-in: with no dependencies wired up
  // it degenerates to "just the single longest bar", which would read as arbitrary noise on a
  // chart nobody asked to analyse that way.
  const [showCritical, setShowCritical] = useState(false)
  const [preview, setPreview] = useState<Preview | null>(null)
  const dragRef = useRef<DragState | null>(null)
  // Set right before a real (nonzero) drag's PATCH fires, so the click event that the browser
  // still dispatches right after pointerup on the same element doesn't also pop the task modal.
  const draggedRef = useRef(false)

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

  const reload = useCallback(() => {
    setError("")
    const q = new URLSearchParams({ from: iso(from), to: iso(to) })
    if (listId) q.set("list_id", String(listId))
    return api.get(`/api/spaces/${spaceId}/timeline?${q}`)
      .then(setData)
      .catch((e) => setError((e as Error).message))
  }, [spaceId, listId, from, to])

  useEffect(() => { reload() }, [reload])

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
  const links = data?.links ?? []
  // Row index per task id — needed both for bar placement and for arrow endpoints.
  const rowOf = useMemo(() => {
    const m = new Map<number, number>()
    items.forEach((it, i) => m.set(it.id, i))
    return m
  }, [items])

  const cpm = useMemo(() => computeCriticalPath(items, links), [items, links])
  const anyEditable = items.some((it) => it.can_edit)

  function dayOffsetsOf(it: TimelineItem) {
    const startDay = Math.round((startOfDay(new Date(it.start)).getTime() - from.getTime()) / DAY_MS)
    const endDay = startDay + spanDaysOf(it) - 1
    return { startDay, endDay }
  }

  function barGeom(it: TimelineItem) {
    const { startDay, endDay } = dayOffsetsOf(it)
    return daysToGeom(startDay, endDay, totalDays, dayW)
  }

  // ---------- drag / resize (spec section 12: reschedule a task without opening it) ----------
  //
  // Only the body of an already-fully-scheduled bar can be moved (both start_at and due_at
  // real) — moving a bar with only a deadline would otherwise have to invent a start_at (or
  // vice versa) purely because the user grabbed the middle instead of an edge, which is exactly
  // the kind of guessed-and-presented-as-fact date the /api/spaces/{id}/timeline handler itself
  // refuses to do. The edges stay live either way: dragging an edge is an unambiguous "set this
  // one date" action regardless of whether the bar was implied before.
  function beginDrag(e: React.PointerEvent, it: TimelineItem, mode: DragMode) {
    if (!it.can_edit || e.button > 0) return
    e.stopPropagation()
    e.preventDefault()
    const { startDay, endDay } = dayOffsetsOf(it)
    dragRef.current = {
      id: it.id, mode, pointerId: e.pointerId, startClientX: e.clientX,
      origStartDay: startDay, origEndDay: endDay, curStartDay: startDay, curEndDay: endDay,
    }
    setPreview({ id: it.id, startDay, endDay })
    ;(e.currentTarget as Element).setPointerCapture(e.pointerId)
  }

  function onDragMove(e: React.PointerEvent) {
    const d = dragRef.current
    if (!d || e.pointerId !== d.pointerId) return
    const deltaDays = Math.round((e.clientX - d.startClientX) / dayW)
    let newStartDay = d.origStartDay
    let newEndDay = d.origEndDay
    if (d.mode === "move") {
      newStartDay = d.origStartDay + deltaDays
      newEndDay = d.origEndDay + deltaDays
    } else if (d.mode === "resize-start") {
      newStartDay = Math.min(d.origStartDay + deltaDays, d.origEndDay)
    } else {
      newEndDay = Math.max(d.origEndDay + deltaDays, d.origStartDay)
    }
    if (newStartDay === d.curStartDay && newEndDay === d.curEndDay) return
    d.curStartDay = newStartDay
    d.curEndDay = newEndDay
    setPreview({ id: d.id, startDay: newStartDay, endDay: newEndDay })
  }

  async function endDrag(e: React.PointerEvent) {
    const d = dragRef.current
    if (!d || e.pointerId !== d.pointerId) return
    dragRef.current = null
    setPreview(null)
    if (d.curStartDay === d.origStartDay && d.curEndDay === d.origEndDay) return
    draggedRef.current = true
    const startDate = new Date(from.getFullYear(), from.getMonth(), from.getDate() + d.curStartDay)
    const endDate = new Date(from.getFullYear(), from.getMonth(), from.getDate() + d.curEndDay)
    // Same round-trip the task modal's own date fields use (a YYYY-MM-DD string parsed as UTC
    // midnight, then re-serialised) — a bar dragged here shows the identical date if the task
    // is then opened in the modal, instead of drifting by a day for anyone off UTC.
    const patch: Record<string, string> = {}
    if (d.mode !== "resize-end") patch.start_at = new Date(iso(startDate)).toISOString()
    if (d.mode !== "resize-start") patch.due_at = new Date(iso(endDate)).toISOString()
    try {
      await api.patch(`/api/tasks/${d.id}`, patch)
      await reload()
    } catch (err) {
      setError((err as Error).message)
    }
  }

  function cancelDrag() {
    dragRef.current = null
    setPreview(null)
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

        {items.length > 0 && (
          <label className="row muted" style={{ gap: 6, fontSize: 12 }}>
            <input type="checkbox" checked={showCritical} onChange={(e) => setShowCritical(e.target.checked)} />
            {tr("timeline.critical_path")}
          </label>
        )}

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
                {links.map((lk, i) => {
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
                  const critical = showCritical && cpm.critical.has(a.id) && cpm.critical.has(b.id)
                  return (
                    <g key={i} className={"timeline-link" + (critical ? " is-critical" : "")}>
                      <path d={`M ${x1} ${y1} H ${midX} V ${y2} H ${x2}`} fill="none" />
                      <path d={`M ${x2} ${y2} l -5 -3.5 l 0 7 z`} className="timeline-arrowhead" />
                    </g>
                  )
                })}
              </svg>

              {items.map((it, i) => {
                const isPreview = preview?.id === it.id
                const g = isPreview ? daysToGeom(preview!.startDay, preview!.endDay, totalDays, dayW) : barGeom(it)
                const movable = it.can_edit && !it.implied
                const critical = showCritical && cpm.critical.has(it.id)
                const cls = [
                  "timeline-bar",
                  it.done ? "is-done" : "",
                  it.overdue ? "is-overdue" : "",
                  it.implied ? "is-implied" : "",
                  critical ? "is-critical" : "",
                  isPreview ? "is-dragging" : "",
                  movable ? "is-movable" : "",
                  g.clippedStart ? "clip-start" : "",
                  g.clippedEnd ? "clip-end" : "",
                ].filter(Boolean).join(" ")
                const range = it.implied
                  ? tr("timeline.implied")
                  : `${new Date(it.start).toLocaleDateString(getFormattingLocale())} — ${new Date(it.end).toLocaleDateString(getFormattingLocale())}`
                const previewRange = isPreview
                  ? `${new Date(from.getFullYear(), from.getMonth(), from.getDate() + preview!.startDay).toLocaleDateString(getFormattingLocale())} — ${new Date(from.getFullYear(), from.getMonth(), from.getDate() + preview!.endDay).toLocaleDateString(getFormattingLocale())}`
                  : null
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
                        title={`${it.title}\n${previewRange ?? range}\n${it.progress}%${it.assignee ? "\n@" + it.assignee : ""}`}
                        onPointerDown={(e) => movable && beginDrag(e, it, "move")}
                        onPointerMove={onDragMove}
                        onPointerUp={endDrag}
                        onPointerCancel={cancelDrag}
                        onClick={() => {
                          if (draggedRef.current) { draggedRef.current = false; return }
                          onOpenTask?.(it.id)
                        }}>
                        {/* progress fill inside the bar */}
                        <span className="timeline-bar-fill" style={{ width: `${it.progress}%` }} />
                        <span className="timeline-bar-text">
                          {previewRange ?? `${it.assignee ? `@${it.assignee}` : ""}${it.progress > 0 ? ` ${it.progress}%` : ""}`}
                        </span>
                        {it.can_edit && (
                          <>
                            <div className="timeline-bar-handle left"
                              onPointerDown={(e) => beginDrag(e, it, "resize-start")}
                              onPointerMove={onDragMove} onPointerUp={endDrag} onPointerCancel={cancelDrag} />
                            <div className="timeline-bar-handle right"
                              onPointerDown={(e) => beginDrag(e, it, "resize-end")}
                              onPointerMove={onDragMove} onPointerUp={endDrag} onPointerCancel={cancelDrag} />
                          </>
                        )}
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
      {anyEditable && (
        <p className="muted" style={{ marginTop: 4, fontSize: 12 }}>{tr("timeline.drag_hint")}</p>
      )}
      {showCritical && cpm.hasCycle && (
        <p className="muted row" style={{ gap: 5, marginTop: 4, fontSize: 12 }}>
          <IconAlertCircle size={12} />
          {tr("timeline.critical_path_cycle")}
        </p>
      )}
    </div>
  )
}
