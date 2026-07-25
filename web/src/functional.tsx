// UI for the features added in the functional stage: team workload, CSV/Trello import,
// note -> tasks, and a personal Telegram bot token.
//
// These live in their own module rather than in extras.tsx because they are the only consumers
// of the endpoints added in that stage; keeping them separate means the existing files change
// by a few lines each instead of growing another 400.
//
// Only api/i18n/icons are imported here on purpose: extras.tsx imports NoteTasksBlock from this
// file, so importing extras.tsx back would make the two modules circular.
import { useEffect, useState } from "react"
import { api, type List, type Note } from "./api"
import { tr } from "./i18n"
import {
  IconBarChart, IconUpload, IconCheckCircle, IconAlertCircle,
  IconClock, IconSlash, IconList, IconTrash, IconKey,
} from "./icons"

// The JSON api helper always sends application/json. CSV and Trello uploads are raw bodies of a
// different type, so they go through fetch directly - with the same cookie mode and the same
// {error} unwrapping the helper does, so failures read the same everywhere.
async function postRaw(url: string, body: string, contentType: string) {
  const r = await fetch(url, {
    method: "POST",
    credentials: "same-origin",
    headers: { "Content-Type": contentType },
    body,
  })
  const text = await r.text()
  let data: { error?: string; imported?: Record<string, number>; space_id?: number } | null = null
  try { data = text ? JSON.parse(text) : null } catch { /* not json */ }
  if (!r.ok) throw new Error(data?.error || r.statusText)
  return data
}

// ---------- team workload ----------

type WorkloadRow = {
  user_id: number | null
  username: string | null
  name: string | null
  open_count: number; open_weight: number
  overdue_count: number; overdue_weight: number
  due_soon_count: number; due_soon_weight: number
  blocked_count: number
}
type Workload = {
  days: number
  members: WorkloadRow[]
  unassigned: WorkloadRow | null
  totals: WorkloadRow
  even_share: number
}

const WORKLOAD_RANGES = [7, 14, 30]

// One person's bar. Overdue is drawn as a separate leading segment rather than a colour applied
// to the whole bar: someone with one late task out of ten is not in the same state as someone
// whose whole queue is late, and a single colour cannot tell those apart.
function WorkloadBar({ row, max }: { row: WorkloadRow; max: number }) {
  const pct = (v: number) => `${Math.round((v / max) * 100)}%`
  const label = row.name || (row.username ? "@" + row.username : tr("workload.unassigned"))
  return (
    <div style={{ marginBottom: 8 }}>
      <div className="row" style={{ justifyContent: "space-between", fontSize: 13 }}>
        <span>{label}</span>
        <span className="muted row" style={{ gap: 6, display: "inline-flex" }}>
          <span className="row" style={{ gap: 3 }} title={tr("workload.open")}>
            <IconList size={13} /> {row.open_count}
          </span>
          {row.overdue_count > 0 && (
            <span className="row" style={{ gap: 3, color: "var(--due-overdue)" }} title={tr("workload.overdue")}>
              <IconAlertCircle size={13} /> {row.overdue_count}
            </span>
          )}
          {row.due_soon_count > 0 && (
            <span className="row" style={{ gap: 3 }} title={tr("workload.due_soon")}>
              <IconClock size={13} /> {row.due_soon_count}
            </span>
          )}
          {row.blocked_count > 0 && (
            <span className="row" style={{ gap: 3 }} title={tr("workload.blocked")}>
              <IconSlash size={13} /> {row.blocked_count}
            </span>
          )}
          <b>{row.open_weight}</b>
        </span>
      </div>
      <div className="row" style={{ height: 8, borderRadius: 4, background: "rgba(128,128,128,.15)", overflow: "hidden", gap: 0 }}>
        <div style={{ height: 8, width: pct(row.overdue_weight), background: "var(--due-overdue)" }} />
        <div style={{ height: 8, width: pct(Math.max(row.open_weight - row.overdue_weight, 0)), background: "var(--accent)" }} />
      </div>
    </div>
  )
}

export function WorkloadPanel({ spaceId }: { spaceId: number }) {
  const [days, setDays] = useState(7)
  const [data, setData] = useState<Workload | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    setLoading(true)
    api.get(`/api/spaces/${spaceId}/workload?days=${days}`)
      .then(setData).catch(() => setData(null))
      .finally(() => setLoading(false))
  }, [spaceId, days])

  if (loading && !data) return <p className="muted">{tr("common.loading")}</p>
  if (!data) return <p className="muted">{tr("workload.unavailable")}</p>

  const rows = data.members
  const unassigned = data.unassigned
  // The reference line has to be inside the drawn range or it points off the end of every bar.
  const max = Math.max(
    ...rows.map((m) => m.open_weight),
    unassigned?.open_weight ?? 0,
    data.even_share,
    1,
  )

  return (
    <div>
      <div className="row" style={{ justifyContent: "space-between", marginBottom: 10 }}>
        <b className="row" style={{ gap: 5 }}><IconBarChart size={15} /> {tr("workload.title")}</b>
        <span className="row" style={{ gap: 4 }}>
          {WORKLOAD_RANGES.map((d) => (
            <button key={d} className={"nav-btn" + (days === d ? " active" : "")} onClick={() => setDays(d)}>
              {tr("workload.days").replace("{days}", String(d))}
            </button>
          ))}
        </span>
      </div>

      {rows.length === 0 && !unassigned && <p className="muted">{tr("workload.empty")}</p>}

      {/* The even-share marker is drawn once above the bars: it is the same value for everyone,
          and repeating it inside each bar would read as a per-person target, which it is not. */}
      {data.even_share > 0 && rows.length > 1 && (
        <div className="muted" style={{ fontSize: 12, marginBottom: 8 }}>
          {tr("workload.even_share").replace("{weight}", String(data.even_share))}
        </div>
      )}

      {rows.map((m) => <WorkloadBar key={m.user_id ?? "none"} row={m} max={max} />)}

      {unassigned && unassigned.open_count > 0 && (
        <div style={{ marginTop: 10, paddingTop: 10, borderTop: "1px solid rgba(128,128,128,.2)" }}>
          <WorkloadBar row={unassigned} max={max} />
          <div className="muted" style={{ fontSize: 12 }}>{tr("workload.unassigned_hint")}</div>
        </div>
      )}

      <div className="muted" style={{ fontSize: 12, marginTop: 10 }}>
        {tr("workload.totals")
          .replace("{count}", String(data.totals.open_count))
          .replace("{weight}", String(data.totals.open_weight))}
      </div>
    </div>
  )
}

// ---------- CSV / Trello import ----------

// Both formats land in a brand new space (that is what the underlying importer does), so this
// card sits next to "create a space" rather than inside one.
export function ImportCard({ onImported }: { onImported: () => void }) {
  const [kind, setKind] = useState<"csv" | "trello">("csv")
  const [name, setName] = useState("")
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState("")
  const [done, setDone] = useState<Record<string, number> | null>(null)

  async function upload(file: File) {
    setBusy(true); setErr(""); setDone(null)
    try {
      const text = await file.text()
      const q = name.trim() ? `?name=${encodeURIComponent(name.trim())}` : ""
      const r = await postRaw(
        kind === "csv" ? `/api/import/csv${q}` : `/api/import/trello${q}`,
        text,
        kind === "csv" ? "text/csv" : "application/json",
      )
      setDone(r?.imported ?? {})
      setName("")
      onImported()
    } catch (e) {
      setErr((e as Error).message)
    }
    setBusy(false)
  }

  return (
    <div style={{ marginTop: 14 }}>
      <div className="row" style={{ gap: 6, flexWrap: "wrap" }}>
        <span className="row muted" style={{ gap: 5, fontSize: 13 }}>
          <IconUpload size={14} /> {tr("import.title")}
        </span>
        <select className="input" style={{ width: "auto" }} value={kind}
          onChange={(e) => { setKind(e.target.value as "csv" | "trello"); setErr(""); setDone(null) }}>
          <option value="csv">{tr("import.kind_csv")}</option>
          <option value="trello">{tr("import.kind_trello")}</option>
        </select>
        <input className="input grow" style={{ minWidth: 140 }} placeholder={tr("import.name_placeholder")}
          value={name} onChange={(e) => setName(e.target.value)} />
        <label className="nav-btn" style={{ cursor: busy ? "default" : "pointer" }}>
          {busy ? tr("import.uploading") : tr("import.choose_file")}
          <input type="file" style={{ display: "none" }} disabled={busy}
            accept={kind === "csv" ? ".csv,text/csv" : ".json,application/json"}
            onChange={(e) => {
              const f = e.target.files?.[0]
              // Clearing the input lets the same file be picked again after a failed attempt.
              e.target.value = ""
              if (f) upload(f)
            }} />
        </label>
      </div>
      <div className="muted" style={{ fontSize: 12, marginTop: 6 }}>
        {kind === "csv" ? tr("import.hint_csv") : tr("import.hint_trello")}
      </div>
      {err && <div style={{ color: "var(--due-overdue)", fontSize: 13, marginTop: 6 }}>{err}</div>}
      {done && (
        <div className="row" style={{ gap: 5, fontSize: 13, marginTop: 6 }}>
          <IconCheckCircle size={14} />
          {tr("import.done")
            .replace("{lists}", String(done.lists ?? 0))
            .replace("{tasks}", String(done.tasks ?? 0))}
        </div>
      )}
    </div>
  )
}

// ---------- note -> tasks ----------

// Shown inside the note modal. The button is deliberately not automatic on save: turning a note
// into tasks is a decision ("we agreed to do these"), and doing it silently on every edit would
// re-create tasks the user already deleted.
export function NoteTasksBlock({ note, spaceId }: { note: Note; spaceId: number }) {
  const [lists, setLists] = useState<List[]>([])
  const [listId, setListId] = useState<number | "">("")
  const [created, setCreated] = useState<Array<{ id: number; title: string; done: boolean }>>([])
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState("")
  const [msg, setMsg] = useState("")

  const loadCreated = () => api.get(`/api/notes/${note.id}/tasks`)
    .then((r) => setCreated(r.tasks)).catch(() => setCreated([]))

  useEffect(() => {
    // Only lists this user may actually write to are offered - the server enforces the same
    // rule, and listing the rest would just produce a rejected request.
    api.get(`/api/spaces/${spaceId}/lists`)
      .then((r) => {
        const writable = (r.lists as List[]).filter((l) => l.my_permission !== "viewer")
        setLists(writable)
        setListId((prev) => (prev === "" && writable.length > 0 ? writable[0].id : prev))
      })
      .catch(() => {})
    loadCreated()
  }, [note.id, spaceId])

  async function convert() {
    if (listId === "") return
    setBusy(true); setErr(""); setMsg("")
    try {
      const r = await api.post(`/api/notes/${note.id}/tasks`, { list_id: listId })
      setMsg(tr("notes.to_tasks_done").replace("{count}", String(r.created)))
      loadCreated()
    } catch (e) {
      setErr((e as Error).message)
    }
    setBusy(false)
  }

  if (lists.length === 0 && created.length === 0) return null

  return (
    <div style={{ marginTop: 12, paddingTop: 10, borderTop: "1px solid rgba(128,128,128,.2)" }}>
      {lists.length > 0 && (
        <div className="row" style={{ gap: 6, flexWrap: "wrap" }}>
          <select className="input" style={{ width: "auto" }} value={listId}
            onChange={(e) => setListId(Number(e.target.value))}>
            {lists.map((l) => <option key={l.id} value={l.id}>{l.name}</option>)}
          </select>
          <button className="nav-btn" onClick={convert} disabled={busy}>
            {tr("notes.to_tasks")}
          </button>
        </div>
      )}
      <div className="muted" style={{ fontSize: 12, marginTop: 6 }}>{tr("notes.to_tasks_hint")}</div>
      {err && <div style={{ color: "var(--due-overdue)", fontSize: 13, marginTop: 6 }}>{err}</div>}
      {msg && <div className="row" style={{ gap: 5, fontSize: 13, marginTop: 6 }}><IconCheckCircle size={14} /> {msg}</div>}
      {created.length > 0 && (
        <div style={{ marginTop: 8 }}>
          <div className="muted" style={{ fontSize: 12, marginBottom: 4 }}>{tr("notes.to_tasks_existing")}</div>
          {created.map((t) => (
            <div key={t.id} className="row" style={{ gap: 6, fontSize: 13 }}>
              <IconCheckCircle size={13} style={{ opacity: t.done ? 1 : 0.35 }} />
              <span style={{ textDecoration: t.done ? "line-through" : "none" }}>{t.title}</span>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

// ---------- personal Telegram bot ----------

type TgStatus = { enabled: boolean; linked: boolean; personal_bot?: string }

// Lives under the server-wide TelegramLinkRow in settings. Unlike that row this one is always
// visible: its whole point is that it works when the server has no bot configured at all.
export function PersonalBotCard() {
  const [status, setStatus] = useState<TgStatus | null>(null)
  const [token, setToken] = useState("")
  const [link, setLink] = useState<{ bot_username: string; deep_link: string } | null>(null)
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState("")
  const [waiting, setWaiting] = useState(false)

  const refresh = () => api.get("/api/telegram/status").then(setStatus).catch(() => {})
  useEffect(() => { refresh() }, [])

  async function save() {
    if (!token.trim()) return
    setBusy(true); setErr("")
    try {
      const r = await api.post("/api/me/telegram/bot", { token: token.trim() })
      setLink(r)
      setToken("")
      refresh()
    } catch (e) {
      setErr((e as Error).message)
    }
    setBusy(false)
  }

  // The server listens to the freshly saved bot for up to 30 seconds, so this request is simply
  // held open until /start arrives. A "not yet" answer is a normal 200, not an error.
  async function confirm() {
    setWaiting(true); setErr("")
    try {
      const r = await api.post("/api/me/telegram/bot/confirm")
      if (r.linked) { setLink(null); refresh() }
      else setErr(tr("profile.bot_not_started"))
    } catch (e) {
      setErr((e as Error).message)
    }
    setWaiting(false)
  }

  async function remove() {
    await api.del("/api/me/telegram/bot").catch(() => {})
    setLink(null)
    refresh()
  }

  const has = !!status?.personal_bot

  return (
    <div style={{ marginBottom: 14 }}>
      <div className="row" style={{ gap: 5, fontSize: 13, marginBottom: 6 }}>
        <IconKey size={14} /> {tr("profile.bot_title")}
      </div>

      {has && !link && (
        <div className="row" style={{ gap: 10 }}>
          <span className="row muted" style={{ gap: 5, fontSize: 13 }}>
            <IconCheckCircle size={13} /> @{status?.personal_bot}
            {status?.linked ? "" : " - " + tr("profile.bot_saved_not_linked")}
          </span>
          {!status?.linked && (
            <button className="nav-btn" onClick={confirm} disabled={waiting}>
              {waiting ? tr("profile.bot_waiting") : tr("profile.bot_confirm")}
            </button>
          )}
          <button className="nav-btn row" style={{ gap: 4 }} onClick={remove}>
            <IconTrash size={13} /> {tr("profile.bot_remove")}
          </button>
        </div>
      )}

      {link && (
        <div>
          <a className="btn secondary row" style={{ gap: 6, display: "inline-flex", width: "fit-content", textDecoration: "none" }}
            href={link.deep_link} target="_blank" rel="noreferrer">
            {tr("profile.bot_open").replace("{bot}", link.bot_username)}
          </a>
          <div className="row" style={{ gap: 8, marginTop: 8 }}>
            <button className="btn" onClick={confirm} disabled={waiting}>
              {waiting ? tr("profile.bot_waiting") : tr("profile.bot_confirm")}
            </button>
            <span className="muted" style={{ fontSize: 12 }}>{tr("profile.bot_confirm_hint")}</span>
          </div>
        </div>
      )}

      {!has && !link && (
        <div className="row" style={{ gap: 6, flexWrap: "wrap" }}>
          <input className="input grow" style={{ minWidth: 200 }} type="password" autoComplete="off"
            placeholder={tr("profile.bot_token_placeholder")}
            value={token} onChange={(e) => setToken(e.target.value)} />
          <button className="nav-btn" onClick={save} disabled={busy || !token.trim()}>
            {busy ? tr("profile.bot_checking") : tr("common.save")}
          </button>
        </div>
      )}

      <div className="muted" style={{ fontSize: 12, marginTop: 6 }}>{tr("profile.bot_hint")}</div>
      {err && <div style={{ color: "var(--due-overdue)", fontSize: 13, marginTop: 6 }}>{err}</div>}
    </div>
  )
}
