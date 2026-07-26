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
//
// Public links and export/import live in sharing.tsx and are mounted here, next to the space and
// list pickers this page already has: "who can see this" and "is this list public" are the same
// question, and answering them on two separate screens with two separate pickers would be worse.

import { useEffect, useState } from "react"
import { api, type Me } from "./api"
import { Avatar } from "./settings"
import { tr } from "./i18n"
import { ShareLinksPanel, SpaceDataCard } from "./sharing"

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
  owner: t("members.role.owner", "владелец"),
  member: t("members.role.member", "участник"),
  editor: t("members.role.editor", "редактор"),
  viewer: t("members.role.viewer", "читатель"),
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

  if (members === null) return <div className="muted">{t("members.loading", "Загрузка…")}</div>

  return (
    <div>
      {err && <div className="muted" style={{ color: "var(--danger, #c33)", marginBottom: 8 }}>{err}</div>}

      {members.length === 0 && !err && (
        <div className="muted" style={{ marginBottom: 10 }}>{t("members.empty", "Пока никого нет")}</div>
      )}

      {members.map((m) => (
        <div key={m.user_id} className="row" style={{ gap: 8, alignItems: "center", padding: "6px 0", flexWrap: "wrap" }}>
          <Avatar userId={m.user_id} name={m.display_name || m.username} size={26} />
          <span>{m.display_name || `@${m.username}`}</span>
          {m.user_id === meId && <span className="badge">{t("members.you", "это вы")}</span>}
          {m.read_only && (
            <span className="badge" title={t("members.read_only_hint", "Глобальная роль «читатель» — запись запрещена везде")}>
              {t("members.read_only", "только чтение")}
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
                title={t("members.remove", "Убрать доступ")}
                onClick={() => {
                  if (!window.confirm(t("members.remove_confirm", "Убрать доступ?"))) return
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
            placeholder={t("members.username_placeholder", "Логин пользователя")}
            onChange={(e) => setUsername(e.target.value)}
            onKeyDown={(e) => { if (e.key === "Enter") add() }} />
          <select className="input" style={{ width: "auto" }} value={newRole} disabled={busy}
            onChange={(e) => setNewRole(e.target.value)}>
            {roles.map((r) => <option key={r} value={r}>{roleLabel(r)}</option>)}
          </select>
          <button className="btn" disabled={busy || !username.trim()} onClick={add}>
            {t("members.add", "Добавить")}
          </button>
        </div>
      )}
    </div>
  )
}

/** Space roster + the roster of any one list inside it, plus public links and export/import. */
export function MembersPage({ me }: { me: Me }) {
  const [spaces, setSpaces] = useState<Array<{ id: number; name: string; my_role: string }>>([])
  const [spaceId, setSpaceId] = useState<number | null>(null)
  const [lists, setLists] = useState<Array<{ id: number; name: string; is_private: boolean }>>([])
  const [listId, setListId] = useState<number | null>(null)
  const [err, setErr] = useState("")

  function loadSpaces(select?: number) {
    api.get("/api/spaces")
      .then((r) => {
        const list = r.spaces || []
        setSpaces(list)
        if (select !== undefined && list.some((s: { id: number }) => s.id === select)) setSpaceId(select)
        else if (spaceId === null && list.length > 0) setSpaceId(list[0].id)
      })
      .catch((e) => setErr((e as Error).message))
  }

  useEffect(() => { loadSpaces() }, [])

  useEffect(() => {
    if (spaceId === null) return
    setListId(null)
    api.get(`/api/spaces/${spaceId}/lists`)
      .then((r) => setLists(r.lists || []))
      .catch(() => setLists([]))
  }, [spaceId])

  const currentSpace = spaces.find((s) => s.id === spaceId)

  return (
    <div>
      <h2 style={{ marginTop: 0 }}>{t("members.title", "Участники и доступ")}</h2>
      {err && <div className="muted" style={{ color: "var(--danger, #c33)" }}>{err}</div>}

      {spaces.length === 0 ? (
        <div className="muted">{t("members.no_spaces", "Сначала создайте пространство")}</div>
      ) : (
        <>
          <div className="card">
            <div className="row" style={{ gap: 8, alignItems: "center", flexWrap: "wrap" }}>
              <b>{t("members.space", "Пространство")}</b>
              <select className="input" style={{ width: "auto" }} value={spaceId ?? ""}
                onChange={(e) => setSpaceId(Number(e.target.value))}>
                {spaces.map((s) => <option key={s.id} value={s.id}>{s.name}</option>)}
              </select>
            </div>
            <p className="muted" style={{ fontSize: 13 }}>
              {t("members.space_hint", "Владелец управляет пространством, участник создаёт списки, читатель только смотрит.")}
            </p>
            {spaceId !== null && (
              <MemberRoster base={`/api/spaces/${spaceId}`} roleKey="role" roles={SPACE_ROLES} meId={me.id} />
            )}
          </div>

          <div className="card">
            <div className="row" style={{ gap: 8, alignItems: "center", flexWrap: "wrap" }}>
              <b>{t("members.list", "Список")}</b>
              <select className="input" style={{ width: "auto" }} value={listId ?? ""}
                onChange={(e) => setListId(e.target.value === "" ? null : Number(e.target.value))}>
                <option value="">{t("members.pick_list", "— выберите список —")}</option>
                {lists.map((l) => (
                  <option key={l.id} value={l.id}>{l.is_private ? `\u{1F512} ${l.name}` : l.name}</option>
                ))}
              </select>
            </div>
            <p className="muted" style={{ fontSize: 13 }}>
              {t("members.list_hint", "Доступ к списку выдаётся отдельно от пространства: редактор меняет задачи, читатель только смотрит.")}
            </p>
            {listId !== null && (
              <>
                <MemberRoster base={`/api/lists/${listId}`} roleKey="permission" roles={LIST_PERMS} meId={me.id} />
                <hr style={{ border: 0, borderTop: "1px solid var(--border, #ddd)", margin: "14px 0" }} />
                <b>{t("share.title", "Публичные ссылки")}</b>
                <ShareLinksPanel listId={listId} />
              </>
            )}
          </div>

          {spaceId !== null && (
            <SpaceDataCard spaceId={spaceId} isOwner={currentSpace?.my_role === "owner"}
              onImported={() => loadSpaces()} />
          )}
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
      <option value="">{t("members.unassigned", "Без исполнителя")}</option>
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
