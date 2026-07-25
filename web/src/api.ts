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
  start_at: string | null
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
  // Review workflow + watchers (migration 0009). review_state is null when never submitted.
  review_state: "pending" | "accepted" | "returned" | null
  review_by: string | null
  review_at: string | null
  review_note: string | null
  watcher_count: number
}

export type Space = { id: number; name: string; my_role: string }
export type List = {
  id: number; name: string; is_private: boolean; my_permission: string
  task_count: number; done_count: number
  // Weighted totals for the "by weight" progress mode (spec section 6). Older servers
  // won't send these, so treat them as optional and fall back to the counts above.
  weight_total?: number; weight_done?: number
}
export type PulseTask = { id: number; title: string; assignee?: string | null; progress?: number | null }
export type PulseSettings = { stale_days: number; green_at: number; yellow_at: number; standup: boolean }
export type Pulse = {
  enabled?: boolean
  score: number; mood: string; total: number; open: number; done: number
  // A signal the space owner switched off is omitted from this object entirely,
  // so every entry has to be treated as optional by consumers.
  signals: Partial<Record<"overdue" | "unassigned" | "no_deadline" | "blocked" | "stale", number>>
  settings?: PulseSettings
  next_action?: { kind: string; task_id: number; title: string }
  in_progress?: PulseTask[]
  standup?: { did: PulseTask[]; doing: PulseTask[]; blocked: PulseTask[] }
}

// Timeline / Gantt (spec section 12). `implied` marks a range the server derived rather than
// one the user entered — those bars are rendered differently so guessed dates never look like
// scheduled ones.
export type TimelineItem = {
  id: number; list_id: number; list_name: string; parent_id: number | null
  title: string; status: string; priority: string; assignee: string | null
  start: string; end: string; implied: boolean
  progress: number; overdue: boolean; done: boolean
  blocked_by: number[]; completed_at: string | null
  // Editor-or-above on this bar's own list — mirrors the server's listPermission check, so the
  // chart can offer drag/resize only where a PATCH would actually be accepted.
  can_edit: boolean
}
export type Timeline = {
  from: string; to: string
  items: TimelineItem[]
  links: Array<{ from: number; to: number }>
  unscheduled: number
}

// Inbox (spec section 12). `reason` explains why an item needs triage so the UI can group
// them instead of showing one undifferentiated pile.
export type InboxItem = {
  id: number; list_id: number; list_name: string
  space_id: number; space_name: string
  title: string; status: string; priority: string
  due_at: string | null; created_at: string
  reason: "review" | "assigned" | "unassigned" | "mentioned"
}
export type Inbox = { items: InboxItem[]; counts: Record<string, number> }

export type Workflow = { statuses: string[]; defaults: string[] }
export const DEFAULT_STATUSES = ["open", "in_progress", "review", "done"]

// The developer credit is fixed in the code, not a branding setting: the server no longer
// accepts branding.developer_name, so there is nothing for the UI to read or override.
export const DEVELOPER_NAME = "DanMotive"

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
  key: string; label: string; type: "text" | "number" | "bool" | "select" | "secret"
  default: string; options?: string[]; value: string
  // Secret settings (the Telegram bot token) always report value: "" — is_set is how the UI
  // knows whether one is actually configured without the server ever echoing it back.
  is_set?: boolean
}

// Per-user preferences, own settings page (distinct from the root-only ServerSettingsCard).
export type NotifyPrefs = {
  sound?: boolean
  dnd?: { enabled: boolean; start: string; end: string }
  types?: Record<string, boolean>
  reminders?: { before_days?: number[]; on_due_day?: boolean; daily_overdue?: boolean }
  // Master switch for Telegram delivery once linked (default true — see settings.tsx's
  // TelegramLinkRow). Meaningless while unlinked; the server never sends regardless.
  telegram?: boolean
}

export type Profile = {
  display_name: string | null
  locale: string | null
  theme_color: string | null
  theme_visual: string | null
  avatar_path: string | null
  notify_prefs: NotifyPrefs | null
}

// Four reactions only: done, blocked/no, warning, question. The server keeps the same list in
// AllowedReactions and migration 0015 cleans out anything left over from the old ten-emoji set.
export const REACTIONS = ["\u2705", "\u274C", "\u26A0\uFE0F", "\u2753"]
