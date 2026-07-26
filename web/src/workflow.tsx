// Custom workflow statuses (spec section: per-space workflow).
//
// The storage and the read side already existed: statuses live in
// spaces.settings -> {"workflow":{"statuses":[...]}}, GET /api/spaces/{id}/workflow merges them with
// the four built-ins, and every selector in the app already renders whatever that endpoint returns.
// The only missing piece was a way to write them — so this writes through PATCH /api/spaces/{id},
// the same settings patch the Pulse form uses, and nothing else had to change.
//
// Only the custom statuses are stored. The built-ins are re-added server-side by mergeStatuses, so
// persisting them here would duplicate a decision the backend already owns.
import { useEffect, useState } from "react"
import { api, DEFAULT_STATUSES, type Workflow } from "./api"
import { tr, trOr } from "./i18n"
import { IconX } from "./icons"

// trOr(), not `tr(key) || fallback`: t() in i18n.ts ends with `return key`, so an unresolved key
// comes back truthy and the || fallback was dead code — this screen used to print workflow.title
// and friends verbatim. The fallbacks now really are fallbacks; en-US and ru-RU carry every key.
const t = trOr

const MAX_LEN = 24

export function WorkflowEditor({ spaceId, isOwner }: { spaceId: number; isOwner: boolean }) {
  const [defaults, setDefaults] = useState<string[]>(DEFAULT_STATUSES)
  const [custom, setCustom] = useState<string[]>([])
  const [draft, setDraft] = useState("")
  const [error, setError] = useState("")
  const [busy, setBusy] = useState(false)
  const [loaded, setLoaded] = useState(false)

  // `defaults` is taken from the response rather than hardcoded: the client constant and the Go
  // one could drift, and the server's answer is the one that decides what 'done' means.
  const load = () =>
    api.get(`/api/spaces/${spaceId}/workflow`)
      .then((r: Workflow & { defaults?: string[] }) => {
        const base = r.defaults && r.defaults.length > 0 ? r.defaults : DEFAULT_STATUSES
        setDefaults(base)
        setCustom((r.statuses || []).filter((s) => !base.includes(s)))
        setLoaded(true)
      })
      .catch((err) => { setError((err as Error).message); setLoaded(true) })
  useEffect(() => { load() }, [spaceId])

  // Optimistic, with a rollback: leaving a status visible that the server refused to store would
  // be a UI claiming a change that does not exist.
  async function save(next: string[]) {
    const previous = custom
    setError("")
    setBusy(true)
    setCustom(next)
    try {
      await api.patch(`/api/spaces/${spaceId}`, { settings: { workflow: { statuses: next } } })
    } catch (err) {
      setCustom(previous)
      setError((err as Error).message)
    } finally {
      setBusy(false)
    }
  }

  function add(e: React.FormEvent) {
    e.preventDefault()
    const name = draft.trim()
    if (!name) return
    if (name.length > MAX_LEN) {
      // The limit is interpolated at the call site: tr()/trOr() return the string as stored, they
      // do not format, so the locale value carries a {max} placeholder.
      setError(
        t("workflow.too_long", "Слишком длинное название. Максимум {max} символов.")
          .replace("{max}", String(MAX_LEN)),
      )
      return
    }
    // Case-insensitive, because "QA" and "qa" would render as two columns holding the same work.
    // mergeStatuses only dedupes exact matches, so this check has to happen here.
    if ([...defaults, ...custom].some((s) => s.toLowerCase() === name.toLowerCase())) {
      setError(t("workflow.duplicate", "Такой статус уже есть"))
      return
    }
    setDraft("")
    save([...custom, name])
  }

  // Order matters: it is the left-to-right order of the kanban columns, so it needs to be editable
  // without deleting and re-adding a status (which would strand the tasks sitting in it).
  function move(index: number, delta: number) {
    const target = index + delta
    if (target < 0 || target >= custom.length) return
    const next = [...custom]
    const [moved] = next.splice(index, 1)
    next.splice(target, 0, moved)
    save(next)
  }

  if (!loaded) return <p className="muted">{tr("search.searching")}</p>

  return (
    <div>
      <div className="section-title">{t("workflow.title", "Статусы задач")}</div>

      <div className="muted" style={{ fontSize: 12, marginBottom: 8 }}>
        {t("workflow.builtin_hint", "Встроенные статусы убрать нельзя: их добавляет сервер, а done закрывает задачу.")}
      </div>
      <div className="row" style={{ gap: 6, flexWrap: "wrap", marginBottom: 14 }}>
        {defaults.map((s) => (
          <span key={s} className="badge" title={t("workflow.builtin", "Встроенный статус")}>
            {DEFAULT_STATUSES.includes(s) ? tr("task.status." + s) : s}
          </span>
        ))}
      </div>

      <div className="muted" style={{ fontSize: 12, marginBottom: 6 }}>
        {t("workflow.custom_hint", "Свои статусы идут после встроенных — в том же порядке они станут колонками канбан-доски.")}
      </div>
      {custom.length === 0 && (
        <p className="muted" style={{ fontSize: 13 }}>{t("workflow.empty", "Своих статусов пока нет.")}</p>
      )}
      {custom.map((s, i) => (
        <div key={s} className="row" style={{ gap: 6, marginBottom: 4, fontSize: 13 }}>
          <span className="grow">{s}</span>
          {isOwner && (
            <>
              <button className="nav-btn" style={{ padding: "1px 7px" }} disabled={busy || i === 0}
                title={t("workflow.move_up", "Выше")} onClick={() => move(i, -1)}>↑</button>
              <button className="nav-btn" style={{ padding: "1px 7px" }} disabled={busy || i === custom.length - 1}
                title={t("workflow.move_down", "Ниже")} onClick={() => move(i, 1)}>↓</button>
              <button className="nav-btn" style={{ padding: "1px 6px", color: "var(--due-overdue)" }}
                disabled={busy} title={tr("common.click_to_remove")}
                onClick={() => save(custom.filter((x) => x !== s))}>
                <IconX size={11} />
              </button>
            </>
          )}
        </div>
      ))}

      {isOwner && (
        <>
          <form className="row" style={{ marginTop: 10 }} onSubmit={add}>
            <input className="input grow" maxLength={MAX_LEN} value={draft}
              placeholder={t("workflow.new_placeholder", "Например: design, qa, blocked")}
              onChange={(e) => setDraft(e.target.value)} />
            <button className="btn" type="submit" disabled={busy}>{tr("common.create")}</button>
          </form>
          {/* Removing a status does not touch the tasks that are already in it — statuses are plain
              strings on the task row. Those tasks keep displaying the raw name until someone moves
              them, which is recoverable but surprising, so it is said out loud. */}
          <div className="muted" style={{ fontSize: 12, marginTop: 6 }}>
            {t("workflow.remove_hint", "Если убрать статус, задачи в нём не переносятся автоматически — переведите их до удаления.")}
          </div>
        </>
      )}
      {!isOwner && (
        <div className="muted" style={{ fontSize: 12, marginTop: 8 }}>
          {t("workflow.owner_only", "Изменять статусы может только владелец пространства.")}
        </div>
      )}
      {error && <p className="error-text">{error}</p>}
    </div>
  )
}
