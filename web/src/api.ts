// Thin client for the Todorio API (cookie sessions, JSON).

async function handle(r: Response) {
  if (r.ok) return r.json()
  let msg = r.statusText
  try {
    const e = await r.json()
    if (e && e.error) msg = e.error
  } catch { /* not json */ }
  const err = new Error(msg) as Error & { status?: number }
  err.status = r.status
  throw err
}

const opts = (method: string, body?: unknown): RequestInit => ({
  method,
  credentials: "same-origin",
  headers: body !== undefined ? { "Content-Type": "application/json" } : undefined,
  body: body !== undefined ? JSON.stringify(body) : undefined,
})

export const api = {
  get: (url: string) => fetch(url, opts("GET")).then(handle),
  post: (url: string, body?: unknown) => fetch(url, opts("POST", body ?? {})).then(handle),
  patch: (url: string, body: unknown) => fetch(url, opts("PATCH", body)).then(handle),
  put: (url: string, body: unknown) => fetch(url, opts("PUT", body)).then(handle),
  del: (url: string) => fetch(url, opts("DELETE")).then(handle),
}

export type Me = {
  id: number
  username: string
  role: "root" | "admin" | "user" | "viewer"
  status: "pending" | "active" | "blocked" | "rejected"
  must_change_password: boolean
}

export type ActiveFocus = { user_id: number; username: string; avatar_path: string | null; started_at: string }

export type Task = {
  id: number
  list_id: number
  parent_id: number | null
  title: string
  description: string | null
  status: string
  priority: string
  assignee_id: number | null
  due_at: string | null
  progress: number | null
  weight: number
  blocked_by: number[]
  custom_fields: Record<string, string> | null
  recurrence: { freq: string; interval: number } | null
  completed_at: string | null
  created_at: string
  updated_at: string
  subtasks_done: number
  subtasks_total: number
  active_focus: ActiveFocus[]
}

export type Space = { id: number; name: string; my_role: string }
export type List = {
  id: number; name: string; is_private: boolean; my_permission: string
  task_count: number; done_count: number
}
export type Pulse = {
  enabled?: boolean
  score: number; mood: string; total: number; open: number; done: number
  signals: { overdue: number; unassigned: number; no_deadline: number; blocked: number; stale: number }
}

export type Workflow = { statuses: string[]; defaults: string[] }
export const DEFAULT_STATUSES = ["open", "in_progress", "review", "done"]

export type Note = {
  id: number; space_id: number; list_id: number | null
  title: string; body?: string; created_by: number; created_at: string; updated_at: string
}

export type SavedFilter = { id: number; list_id: number | null; name: string; query: Record<string, unknown> }

export type FocusStats = { total_seconds: number; sessions: number }

export type ActivityEvent = {
  type: "task_created" | "task_completed" | "comment"
  task_id: number; title: string; by: string; at: string
}

export type SearchResult =
  | { type: "task"; id: number; list_id: number; title: string }
  | { type: "note"; id: number; space_id: number; title: string }
  | { type: "comment"; id: number; task_id: number; task_title: string; snippet: string }

export type SettingDef = {
  key: string; label: string; type: "text" | "number" | "bool" | "select"
  default: string; options?: string[]; value: string
}

// Per-user preferences, own settings page (distinct from the root-only ServerSettingsCard).
export type NotifyPrefs = {
  sound?: boolean
  dnd?: { enabled: boolean; start: string; end: string }
  types?: Record<string, boolean>
  reminders?: { before_days?: number[]; on_due_day?: boolean; daily_overdue?: boolean }
}

export type Profile = {
  display_name: string | null
  locale: string | null
  theme_color: string | null
  theme_scheme: string | null
  theme_visual: string | null
  avatar_path: string | null
  notify_prefs: NotifyPrefs | null
}

export const REACTIONS = ["\u{1F44D}", "\u2705", "\u{1F389}", "\u{1F525}", "\u{1F440}", "\u2753", "\u2757", "\u274C", "\u{1F62D}", "\u2B50"]
