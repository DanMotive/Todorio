// Members & permissions UI.
//
// The server has had space_members (owner | member | viewer) and list_members
// (owner | editor | viewer) since the first migration, and every access check in the API routes
// through them — but no screen ever touched them. Sharing a space or a list was only possible by
// calling the API by hand, which made the product's core promise (working together) unreachable
// from the browser. This module is that missing screen.
//
// It deliberately lives in its own file rather than inside views.tsx: that file is already ~100 KB
// and holds eight unrelated screens, and growing it further is what makes the frontend hard to
// keep up with the backend in the first place.

import { useEffect, useState } from "react"
import { api, type Me } from "./api"
import { Avatar } from "./settings"
import { tr } from "./i18n"

export type Member = {
  user_id: number
  username: string
  display_name: string | null
  // The account's *global* role. spaceRole/listPermission on the server cap a globally read-only
  // account at viewer no matter what the scoped role says, so the roster shows it rather than
  // promising an "editor" whose every write would be rejected.
  global_role: string
  status: string
  role: string
  read_only: boolean
}

export type AssignableUser = { id: number; username: string; display_name: string | null }

// t() is tr() with an inline fallback. New keys are not in web/src/locales/* yet, and tr()
// returns an empty string for an unknown key — which would render blank buttons. The fallback
// keeps this screen usable the moment it ships; the strings move into the locale files (and the
// fallbacks come out) in the same pass that translates them.
const t = (key: string, fallback: string) => tr(key) || fallback

const SPACE_ROLES = ["owner", "member", "viewer"]
const LIST_PERMS = ["owner", "editor", "viewer"]

const ROLE_LABELS: Record<string, string> = {
  owner: t("members.role.owner", "\u0432\u043b\u0430\u0434\u0435\u043b\u0435\u0446"),
  member: t("members.role.member", "\u0443\u0447\u0430\u0441\u0442\u043d\u0438\u043a"),
  editor: t("members.role.editor", "\u0440\u0435\u0434\u0430\u043a\u0442\u043e\u0440"),
  viewer: t("members.role.viewer", "\u0447\u0438\u0442\u0430\u0442\u0435\u043b\u044c"),
}

const roleLabel = (role: string) => ROLE_LABELS[role] || role

/**
 * One roster, driven entirely by `base`: "/api/spaces/7" or "/api/lists/12". Spaces and lists
 * have the same four operations against the same URL shape and differ only in the name of the
 * role field and the set of allowed values, so they share this component instead of two
 * near-identical ones drifting apart.
 */
function MemberRoster({ base, roleKey, roles, meId }: {
  base: string
  roleKey: "role" | "permission"
  roles: string[]
  meId: number
}) {
  const [members, setMembers] = useState<Member[] | null>(null)
  const [canManage, setCanManage] = useState(false)
  const [username, setUsername] = useState("")
  const [newRole, setNewRole] = useState(roles[1] || roles[0])
  const [err, setErr] = useState("")
  const [busy, setBusy] = useState(false)

  async function load() {
    setErr("")
    try {
      const r = await api.get(`${base}/members`)
      setMembers(r.members || [])
      setCanManage(!!r.can_manage)
    } catch (e) {
      // Errors are shown, not swallowed. A silent catch here would render an empty roster for a
      // permission or network failure, i.e. "nobody has access" — the most misleading possible
      // answer to "who can see this?".
      setMembers([])
      setErr((e as Error).message)
    }
  }

  useEffect(() => { setMembers(null); load() }, [base])

  async function run(fn: () => Promise<unknown>) {
    setBusy(true)
    setErr("")
    try {
      await fn()
      await load()
    } catch (e) {
      setErr((e as Error).message)
    } finally {
      setBusy(false)
    }
  }

  const add = () => {
    const name = username.trim().replace(/^@/, "")
    if (!name) return
    run(async () => {
      await api.post(`${base}/members`, { username: name, [roleKey]: newRole })
      setUsername("")
    })
  }

  if (members === null) return <div className="muted">{t("members.loading", "\u0417\u0430\u0433\u0440\u0443\u0437\u043a\u0430\u2026")}</div>

  return (
    <div>
      {err && <div className="muted" style={{ color: "var(--danger, #c33)", marginBottom: 8 }}>{err}</div>}

      {members.length === 0 && !err && (
        <div className="muted" style={{ marginBottom: 10 }}>{t("members.empty", "\u041f\u043e\u043a\u0430 \u043d\u0438\u043a\u043e\u0433\u043e \u043d\u0435\u0442")}</div>
      )}

      {members.map((m) => (
        <div key={m.user_id} className="row" style={{ gap: 8, alignItems: "center", padding: "6px 0", flexWrap: "wrap" }}>
          <Avatar userId={m.user_id} name={m.display_name || m.username} size={26} />
          <span>{m.display_name || `@${m.username}`}</span>
          {m.user_id === meId && <span className="badge">{t("members.you", "\u044d\u0442\u043e \u0432\u044b")}</span>}
          {m.read_only && (
            <span className="badge" title={t("members.read_only_hint", "\u0413\u043b\u043e\u0431\u0430\u043b\u044c\u043d\u0430\u044f \u0440\u043e\u043b\u044c \u00ab\u0447\u0438\u0442\u0430\u0442\u0435\u043b\u044c\u00bb \u2014 \u0437\u0430\u043f\u0438\u0441\u044c \u0437\u0430\u043f\u0440\u0435\u0449\u0435\u043d\u0430 \u0432\u0435\u0437\u0434\u0435")}>
              {t("members.read_only", "\u0442\u043e\u043b\u044c\u043a\u043e \u0447\u0442\u0435\u043d\u0438\u0435")}
            </span>
          )}
          {m.status !== "active" && <span className="badge">{m.status}</span>}

          <div className="row" style={{ gap: 6, marginLeft: "auto" }}>
            {canManage ? (
              <select className="input" style={{ width: "auto", padding: "4px 6px", fontSize: 13 }}
                value={m.role} disabled={busy}
                onChange={(e) => run(() => api.patch(`${base}/members/${m.user_id}`, { [roleKey]: e.target.value }))}>
                {roles.map((r) => <option key={r} value={r}>{roleLabel(r)}</option>)}
              </select>
            ) : (
              <span className="muted">{roleLabel(m.role)}</span>
            )}
            {canManage && (
              <button className="ctrl-btn" disabled={busy}
                title={t("members.remove", "\u0423\u0431\u0440\u0430\u0442\u044c \u0434\u043e\u0441\u0442\u0443\u043f")}
                onClick={() => {
                  if (!window.confirm(t("members.remove_confirm", "\u0423\u0431\u0440\u0430\u0442\u044c \u0434\u043e\u0441\u0442\u0443\u043f?"))) return
                  run(() => api.del(`${base}/members/${m.user_id}`))
                }}>
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><line x1="18" y1="6" x2="6" y2="18" /><line x1="6" y1="6" x2="18" y2="18" /></svg>
              </button>
            )}
          </div>
        </div>
      ))}

      {canManage && (
        <div className="row" style={{ gap: 6, marginTop: 12, flexWrap: "wrap" }}>
          <input className="input" style={{ maxWidth: 220 }} value={username} disabled={busy}
            placeholder={t("members.username_placeholder", "\u041b\u043e\u0433\u0438\u043d \u043f\u043e\u043b\u044c\u0437\u043e\u0432\u0430\u0442\u0435\u043b\u044f")}
            onChange={(e) => setUsername(e.target.value)}
            onKeyDown={(e) => { if (e.key === "Enter") add() }} />
          <select className="input" style={{ width: "auto" }} value={newRole} disabled={busy}
            onChange={(e) => setNewRole(e.target.value)}>
            {roles.map((r) => <option key={r} value={r}>{roleLabel(r)}</option>)}
          </select>
          <button className="btn" disabled={busy || !username.trim()} onClick={add}>
            {t("members.add", "\u0414\u043e\u0431\u0430\u0432\u0438\u0442\u044c")}
          </button>
        </div>
      )}
    </div>
  )
}

/** Space roster + the roster of any one list inside it. */
export function MembersPage({ me }: { me: Me }) {
  const [spaces, setSpaces] = useState<Array<{ id: number; name: string; my_role: string }>>([])
  const [spaceId, setSpaceId] = useState<number | null>(null)
  const [lists, setLists] = useState<Array<{ id: number; name: string; is_private: boolean }>>([])
  const [listId, setListId] = useState<number | null>(null)
  const [err, setErr] = useState("")

  useEffect(() => {
    api.get("/api/spaces")
      .then((r) => {
        const list = r.spaces || []
        setSpaces(list)
        if (list.length > 0) setSpaceId(list[0].id)
      })
      .catch((e) => setErr((e as Error).message))
  }, [])

  useEffect(() => {
    if (spaceId === null) return
    setListId(null)
    api.get(`/api/spaces/${spaceId}/lists`)
      .then((r) => setLists(r.lists || []))
      .catch(() => setLists([]))
  }, [spaceId])

  return (
    <div>
      <h2 style={{ marginTop: 0 }}>{t("members.title", "\u0423\u0447\u0430\u0441\u0442\u043d\u0438\u043a\u0438 \u0438 \u0434\u043e\u0441\u0442\u0443\u043f")}</h2>
      {err && <div className="muted" style={{ color: "var(--danger, #c33)" }}>{err}</div>}

      {spaces.length === 0 ? (
        <div className="muted">{t("members.no_spaces", "\u0421\u043d\u0430\u0447\u0430\u043b\u0430 \u0441\u043e\u0437\u0434\u0430\u0439\u0442\u0435 \u043f\u0440\u043e\u0441\u0442\u0440\u0430\u043d\u0441\u0442\u0432\u043e")}</div>
      ) : (
        <>
          <div className="card">
            <div className="row" style={{ gap: 8, alignItems: "center", flexWrap: "wrap" }}>
              <b>{t("members.space", "\u041f\u0440\u043e\u0441\u0442\u0440\u0430\u043d\u0441\u0442\u0432\u043e")}</b>
              <select className="input" style={{ width: "auto" }} value={spaceId ?? ""}
                onChange={(e) => setSpaceId(Number(e.target.value))}>
                {spaces.map((s) => <option key={s.id} value={s.id}>{s.name}</option>)}
              </select>
            </div>
            <p className="muted" style={{ fontSize: 13 }}>
              {t("members.space_hint", "\u0412\u043b\u0430\u0434\u0435\u043b\u0435\u0446 \u0443\u043f\u0440\u0430\u0432\u043b\u044f\u0435\u0442 \u043f\u0440\u043e\u0441\u0442\u0440\u0430\u043d\u0441\u0442\u0432\u043e\u043c, \u0443\u0447\u0430\u0441\u0442\u043d\u0438\u043a \u0441\u043e\u0437\u0434\u0430\u0451\u0442 \u0441\u043f\u0438\u0441\u043a\u0438, \u0447\u0438\u0442\u0430\u0442\u0435\u043b\u044c \u0442\u043e\u043b\u044c\u043a\u043e \u0441\u043c\u043e\u0442\u0440\u0438\u0442.")}
            </p>
            {spaceId !== null && (
              <MemberRoster base={`/api/spaces/${spaceId}`} roleKey="role" roles={SPACE_ROLES} meId={me.id} />
            )}
          </div>

          <div className="card">
            <div className="row" style={{ gap: 8, alignItems: "center", flexWrap: "wrap" }}>
              <b>{t("members.list", "\u0421\u043f\u0438\u0441\u043e\u043a")}</b>
              <select className="input" style={{ width: "auto" }} value={listId ?? ""}
                onChange={(e) => setListId(e.target.value === "" ? null : Number(e.target.value))}>
                <option value="">{t("members.pick_list", "\u2014 \u0432\u044b\u0431\u0435\u0440\u0438\u0442\u0435 \u0441\u043f\u0438\u0441\u043e\u043a —")}</option>
                {lists.map((l) => (
                  <option key={l.id} value={l.id}>{l.is_private ? `\u{1F512} ${l.name}` : l.name}</option>
                ))}
              </select>
            </div>
            <p className="muted" style={{ fontSize: 13 }}>
              {t("members.list_hint", "\u0414\u043e\u0441\u0442\u0443\u043f \u043a \u0441\u043f\u0438\u0441\u043a\u0443 \u0432\u044b\u0434\u0430\u0451\u0442\u0441\u044f \u043e\u0442\u0434\u0435\u043b\u044c\u043d\u043e \u043e\u0442 \u043f\u0440\u043e\u0441\u0442\u0440\u0430\u043d\u0441\u0442\u0432\u0430: \u0440\u0435\u0434\u0430\u043a\u0442\u043e\u0440 \u043c\u0435\u043d\u044f\u0435\u0442 \u0437\u0430\u0434\u0430\u0447\u0438, \u0447\u0438\u0442\u0430\u0442\u0435\u043b\u044c \u0442\u043e\u043b\u044c\u043a\u043e \u0441\u043c\u043e\u0442\u0440\u0438\u0442.")}
            </p>
            {listId !== null && (
              <MemberRoster base={`/api/lists/${listId}`} roleKey="permission" roles={LIST_PERMS} meId={me.id} />
            )}
          </div>
        </>
      )}
    </div>
  )
}

/**
 * Assignee dropdown for a task, backed by GET /api/lists/{id}/assignable (list members plus the
 * members of the surrounding space, minus globally read-only accounts). Exported for the task
 * modal, which can currently only assign a task to the signed-in user because the client had no
 * way to learn who else is allowed.
 */
export function AssigneePicker({ listId, value, onChange, disabled }: {
  listId: number
  value: number | null
  onChange: (userId: number | null) => void
  disabled?: boolean
}) {
  const [users, setUsers] = useState<AssignableUser[] | null>(null)

  useEffect(() => {
    let alive = true
    api.get(`/api/lists/${listId}/assignable`)
      .then((r) => { if (alive) setUsers(r.users || []) })
      .catch(() => { if (alive) setUsers([]) })
    return () => { alive = false }
  }, [listId])

  return (
    <select className="input" style={{ width: "auto" }} disabled={disabled || users === null}
      value={value ?? ""}
      onChange={(e) => onChange(e.target.value === "" ? null : Number(e.target.value))}>
      <option value="">{t("members.unassigned", "\u0411\u0435\u0437 \u0438\u0441\u043f\u043e\u043b\u043d\u0438\u0442\u0435\u043b\u044f")}</option>
      {/* A stale assignee (someone whose access was revoked) is still listed so the field never
          silently shows "unassigned" for a task that does have an assignee. */}
      {value !== null && !(users || []).some((u) => u.id === value) && (
        <option value={value}>#{value}</option>
      )}
      {(users || []).map((u) => (
        <option key={u.id} value={u.id}>{u.display_name || `@${u.username}`}</option>
      ))}
    </select>
  )
}
