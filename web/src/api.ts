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
  del: (url: string) => fetch(url, opts("DELETE")).then(handle),
}

export type Me = {
  id: number
  username: string
  role: "root" | "admin" | "user" | "viewer"
  status: "pending" | "active" | "blocked" | "rejected"
  must_change_password: boolean
}

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
  completed_at: string | null
  subtasks_done: number
  subtasks_total: number
}

export type Space = { id: number; name: string; my_role: string }
export type List = {
  id: number; name: string; is_private: boolean; my_permission: string
  task_count: number; done_count: number
}
export type Pulse = {
  score: number; mood: string; total: number; open: number; done: number
  signals: { overdue: number; unassigned: number; no_deadline: number; blocked: number; stale: number }
}

export const REACTIONS = ["\u{1F44D}", "\u2705", "\u{1F389}", "\u{1F525}", "\u{1F440}", "\u2753", "\u2757", "\u274C", "\u{1F62D}", "\u2B50"]
