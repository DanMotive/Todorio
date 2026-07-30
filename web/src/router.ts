export type MainView = "my" | "inbox" | "spaces" | "members" | "search" | "notifications" | "admin" | "settings" | "about"

export type AppRoute =
  | { kind: "view"; view: MainView }
  | { kind: "space"; spaceId: number }
  | { kind: "list"; spaceId: number; listId: number }
  | { kind: "task"; taskId: number; background?: AppRoute }
  | { kind: "note"; noteId: number; background?: AppRoute }

const VIEWS = new Set<MainView>(["my", "inbox", "spaces", "members", "search", "notifications", "admin", "settings", "about"])

function positive(value: string): number | null {
  const n = Number(value)
  return Number.isSafeInteger(n) && n > 0 ? n : null
}

export function routePath(route: AppRoute): string {
  switch (route.kind) {
    case "view": return `/app/${route.view}`
    case "space": return `/app/spaces/${route.spaceId}`
    case "list": return `/app/spaces/${route.spaceId}/lists/${route.listId}`
    case "task": return `/app/tasks/${route.taskId}`
    case "note": return `/app/notes/${route.noteId}`
  }
}

function routeFromState(value: unknown, depth = 0): AppRoute | undefined {
  if (!value || typeof value !== "object" || depth > 2) return undefined
  const route = value as Record<string, unknown>
  if (route.kind === "view" && typeof route.view === "string" && VIEWS.has(route.view as MainView)) {
    return { kind: "view", view: route.view as MainView }
  }
  if (route.kind === "space" && typeof route.spaceId === "number" && Number.isSafeInteger(route.spaceId) && route.spaceId > 0) {
    return { kind: "space", spaceId: route.spaceId }
  }
  if (route.kind === "list" && typeof route.spaceId === "number" && typeof route.listId === "number"
    && Number.isSafeInteger(route.spaceId) && route.spaceId > 0 && Number.isSafeInteger(route.listId) && route.listId > 0) {
    return { kind: "list", spaceId: route.spaceId, listId: route.listId }
  }
  if ((route.kind === "task" || route.kind === "note") && typeof route[route.kind + "Id"] === "number") {
    const id = route[route.kind + "Id"] as number
    if (!Number.isSafeInteger(id) || id <= 0) return undefined
    const background = routeFromState(route.background, depth + 1)
    return route.kind === "task" ? { kind: "task", taskId: id, background } : { kind: "note", noteId: id, background }
  }
  return undefined
}

export function parseRoute(pathname = window.location.pathname, state: unknown = window.history.state): AppRoute {
  const background = routeFromState((state as { background?: unknown } | null)?.background)
  const parts = pathname.replace(/\/+$/, "").split("/").filter(Boolean)
  if (parts[0] !== "app") return { kind: "view", view: "my" }
  if (parts.length === 2 && VIEWS.has(parts[1] as MainView)) return { kind: "view", view: parts[1] as MainView }
  if (parts[1] === "spaces" && parts.length === 3) {
    const spaceId = positive(parts[2]); if (spaceId) return { kind: "space", spaceId }
  }
  if (parts[1] === "spaces" && parts[3] === "lists" && parts.length === 5) {
    const spaceId = positive(parts[2]); const listId = positive(parts[4])
    if (spaceId && listId) return { kind: "list", spaceId, listId }
  }
  if (parts[1] === "tasks" && parts.length === 3) {
    const taskId = positive(parts[2]); if (taskId) return { kind: "task", taskId, background }
  }
  if (parts[1] === "notes" && parts.length === 3) {
    const noteId = positive(parts[2]); if (noteId) return { kind: "note", noteId, background }
  }
  return { kind: "view", view: "my" }
}

export function pushRoute(route: AppRoute) {
  const state = route.kind === "task" || route.kind === "note" ? { background: route.background } : null
  window.history.pushState(state, "", routePath(route))
  window.dispatchEvent(new PopStateEvent("popstate", { state }))
}

export function replaceRoute(route: AppRoute) {
  const state = route.kind === "task" || route.kind === "note" ? { background: route.background } : null
  window.history.replaceState(state, "", routePath(route))
  window.dispatchEvent(new PopStateEvent("popstate", { state }))
}

export function routeView(route: AppRoute): MainView {
  if (route.kind === "view") return route.view
  if (route.kind === "space" || route.kind === "list") return "spaces"
  return route.background ? routeView(route.background) : "my"
}
