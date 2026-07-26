// Public share links and space export / import.
//
// Both features were already complete on the server and completely unreachable from the browser:
//
//   * share_links + /s/{token} have existed for a long time, and PublicListPage already renders a
//     shared list — but nothing could *create* a link, list the active ones, or revoke one. A link
//     could only be minted by calling the API by hand, which also meant a link handed out by
//     mistake could never be taken back through the UI.
//   * GET /api/spaces/{id}/export and POST /api/spaces/import were unused by any screen. For a
//     self-hosted product whose first promise is "your data on your server", being unable to get
//     that data out from the browser is the worst possible gap.
//
// Kept in its own module for the same reason members.tsx is: views.tsx is already ~100 KB.

import { useRef, useState, useEffect } from "react"
import { api } from "./api"
import { tr } from "./i18n"

// tr() with an inline fallback — see the same helper in members.tsx. The keys are not in
// web/src/locales/* yet and tr() returns "" for an unknown key, which would render blank buttons.
const t = (key: string, fallback: string) => tr(key) || fallback

export type ShareLink = {
  id: number
  token: string
  expires_at: string | null
  has_password: boolean
  created_at: string
}

const EXPIRY_CHOICES = [
  { days: 0, label: () => t("share.expiry.never", "бессрочно") },
  { days: 1, label: () => t("share.expiry.1", "1 день") },
  { days: 7, label: () => t("share.expiry.7", "7 дней") },
  { days: 30, label: () => t("share.expiry.30", "30 дней") },
  { days: 90, label: () => t("share.expiry.90", "90 дней") },
]

function publicUrl(token: string) {
  // Must match the route main.tsx recognises: /s/{token}.
  return `${window.location.origin}/s/${token}`
}

async function copyToClipboard(text: string): Promise<boolean> {
  // navigator.clipboard only exists in a secure context, and plenty of self-hosted installs run
  // on plain HTTP inside a LAN. Falling back to a prompt() the user can copy from is far better
  // than a button that silently does nothing.
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(text)
      return true
    }
  } catch { /* fall through to the manual path */ }
  window.prompt(t("share.copy_manual", "Скопируйте ссылку:"), text)
  return false
}

/** Active public links for one list, plus creation and revocation. List owners only. */
export function ShareLinksPanel({ listId }: { listId: number }) {
  const [links, setLinks] = useState<ShareLink[] | null>(null)
  const [notOwner, setNotOwner] = useState(false)
  const [err, setErr] = useState("")
  const [busy, setBusy] = useState(false)
  const [days, setDays] = useState(0)
  const [password, setPassword] = useState("")
  const [copied, setCopied] = useState<number | null>(null)

  async function load() {
    setErr("")
    setNotOwner(false)
    try {
      const r = await api.get(`/api/lists/${listId}/share`)
      setLinks(r.links || [])
    } catch (e) {
      const status = (e as { status?: number }).status
      // 403 here is the normal answer for an editor or viewer, not a failure worth shouting
      // about — but it must not be rendered as "no links exist", which would suggest the list
      // is private when it may well be shared with the world.
      if (status === 403) setNotOwner(true)
      else setErr((e as Error).message)
      setLinks([])
    }
  }

  useEffect(() => { setLinks(null); load() }, [listId])

  async function create() {
    setBusy(true)
    setErr("")
    try {
      const r = await api.post(`/api/lists/${listId}/share`, {
        expires_in_days: days > 0 ? days : null,
        password: password ? password : null,
      })
      setPassword("")
      await load()
      // Copying immediately is the point of creating a link, so do it without a second click.
      if (r?.token) {
        const ok = await copyToClipboard(publicUrl(r.token))
        if (ok) {
          setCopied(r.id)
          setTimeout(() => setCopied(null), 2000)
        }
      }
    } catch (e) {
      setErr((e as Error).message)
    } finally {
      setBusy(false)
    }
  }

  async function revoke(link: ShareLink) {
    if (!window.confirm(t("share.revoke_confirm",
      "Отозвать ссылку? Все, кому она была отправлена, потеряют доступ."))) return
    setBusy(true)
    setErr("")
    try {
      await api.del(`/api/shares/${link.id}`)
      await load()
    } catch (e) {
      setErr((e as Error).message)
    } finally {
      setBusy(false)
    }
  }

  if (links === null) return <div className="muted">{t("share.loading", "Загрузка…")}</div>

  if (notOwner) {
    return (
      <div className="muted" style={{ fontSize: 13 }}>
        {t("share.owner_only", "Публичными ссылками управляет только владелец списка.")}
      </div>
    )
  }

  return (
    <div>
      <p className="muted" style={{ fontSize: 13, marginTop: 0 }}>
        {t("share.hint", "Ссылка открывает список на чтение без входа — видны названия задач и сроки, без комментариев и вложений.")}
      </p>

      {err && <div className="muted" style={{ color: "var(--danger, #c33)", marginBottom: 8 }}>{err}</div>}

      {links.length === 0 && !err && (
        <div className="muted" style={{ marginBottom: 10 }}>{t("share.empty", "Публичных ссылок нет")}</div>
      )}

      {links.map((link) => {
        const expired = !!link.expires_at && new Date(link.expires_at).getTime() < Date.now()
        return (
          <div key={link.id} className="row" style={{ gap: 8, alignItems: "center", padding: "6px 0", flexWrap: "wrap" }}>
            <code style={{ fontSize: 12, overflow: "hidden", textOverflow: "ellipsis", maxWidth: 260 }}>
              /s/{link.token}
            </code>
            {link.has_password && (
              <span className="badge" title={t("share.password_set_hint",
                "Пароль задан при создании и не может быть показан снова")}>
                {t("share.password_set", "с паролем")}
              </span>
            )}
            {expired ? (
              <span className="badge">{t("share.expired", "истекла")}</span>
            ) : link.expires_at ? (
              <span className="muted" style={{ fontSize: 12 }}>
                {t("share.until", "до")} {new Date(link.expires_at).toLocaleDateString()}
              </span>
            ) : (
              <span className="muted" style={{ fontSize: 12 }}>{t("share.expiry.never", "бессрочно")}</span>
            )}

            <div className="row" style={{ gap: 6, marginLeft: "auto" }}>
              <button className="btn" disabled={busy} onClick={async () => {
                const ok = await copyToClipboard(publicUrl(link.token))
                if (ok) {
                  setCopied(link.id)
                  setTimeout(() => setCopied(null), 2000)
                }
              }}>
                {copied === link.id ? t("share.copied", "Скопировано") : t("share.copy", "Копировать")}
              </button>
              <button className="ctrl-btn" disabled={busy} title={t("share.revoke", "Отозвать")}
                onClick={() => revoke(link)}>
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><line x1="18" y1="6" x2="6" y2="18" /><line x1="6" y1="6" x2="18" y2="18" /></svg>
              </button>
            </div>
          </div>
        )
      })}

      <div className="row" style={{ gap: 6, marginTop: 12, flexWrap: "wrap", alignItems: "center" }}>
        <select className="input" style={{ width: "auto" }} value={days} disabled={busy}
          onChange={(e) => setDays(Number(e.target.value))}>
          {EXPIRY_CHOICES.map((c) => <option key={c.days} value={c.days}>{c.label()}</option>)}
        </select>
        <input className="input" style={{ maxWidth: 200 }} type="password" value={password} disabled={busy}
          autoComplete="new-password"
          placeholder={t("share.password_placeholder", "Пароль (необязательно)")}
          onChange={(e) => setPassword(e.target.value)} />
        <button className="btn" disabled={busy} onClick={create}>
          {t("share.create", "Создать ссылку")}
        </button>
      </div>
    </div>
  )
}

/** Export the selected space, and import an exported document as a new space. */
export function SpaceDataCard({ spaceId, isOwner, onImported }: {
  spaceId: number
  isOwner: boolean
  onImported?: () => void
}) {
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState("")
  const [result, setResult] = useState("")
  const fileRef = useRef<HTMLInputElement>(null)

  async function exportSpace() {
    setBusy(true)
    setErr("")
    setResult("")
    try {
      // Not api.get(): the endpoint answers with a file and Content-Disposition, not JSON.
      // A plain <a href> would also work, but a failure would then replace the page with raw
      // JSON instead of showing an error next to the button.
      const res = await fetch(`/api/spaces/${spaceId}/export`, {
        credentials: "same-origin",
        headers: { Accept: "application/json" },
      })
      if (!res.ok) {
        let msg = `HTTP ${res.status}`
        try {
          const j = await res.json()
          msg = j.error || j.message || msg
        } catch { /* non-JSON error body: keep the status */ }
        throw new Error(msg)
      }
      const blob = await res.blob()
      const url = URL.createObjectURL(blob)
      const a = document.createElement("a")
      a.href = url
      a.download = `todorio-space-${spaceId}.json`
      document.body.appendChild(a)
      a.click()
      a.remove()
      // Revoking immediately can cancel the download in some browsers; give it a tick.
      setTimeout(() => URL.revokeObjectURL(url), 1000)
    } catch (e) {
      setErr((e as Error).message)
    } finally {
      setBusy(false)
    }
  }

  async function importFile(file: File) {
    setBusy(true)
    setErr("")
    setResult("")
    try {
      // The server caps the request body at 32 MB. Checking here turns a confusing truncated
      // upload into a clear message before anything is sent.
      if (file.size > 32 * 1024 * 1024) {
        throw new Error(t("portability.too_big", "Файл больше 32 МБ — сервер его не примет"))
      }
      const text = await file.text()
      let doc: any
      try {
        doc = JSON.parse(text)
      } catch {
        throw new Error(t("portability.not_json", "Это не JSON-файл экспорта"))
      }
      // Validating the format client-side lets the user see what they are about to create.
      // The server checks the same things again — this is convenience, not the security boundary.
      if (doc?.format_version !== 1) {
        throw new Error(t("portability.bad_format", "Неподдерживаемый формат экспорта"))
      }
      if (!doc.space_name) {
        throw new Error(t("portability.no_name", "В файле нет названия пространства"))
      }
      const lists = Array.isArray(doc.lists) ? doc.lists : []
      const taskCount = lists.reduce((n: number, l: any) => n + (Array.isArray(l.tasks) ? l.tasks.length : 0), 0)
      const ok = window.confirm(
        `${t("portability.confirm", "Будет создано новое пространство")}: «${doc.space_name}»\n` +
        `${t("portability.lists", "Списков")}: ${lists.length}, ${t("portability.tasks", "задач")}: ${taskCount}`)
      if (!ok) return
      const r = await api.post("/api/spaces/import", doc)
      const c = r?.imported || {}
      setResult(`${t("portability.done", "Импортировано")}: ` +
        `${t("portability.lists", "Списков")} ${c.lists ?? 0}, ` +
        `${t("portability.tasks", "задач")} ${c.tasks ?? 0}, ` +
        `${t("portability.comments", "комментариев")} ${c.comments ?? 0}, ` +
        `${t("portability.notes", "заметок")} ${c.notes ?? 0}`)
      onImported?.()
    } catch (e) {
      setErr((e as Error).message)
    } finally {
      setBusy(false)
      // Clear the input so re-picking the same file fires onChange again.
      if (fileRef.current) fileRef.current.value = ""
    }
  }

  return (
    <div className="card">
      <b>{t("portability.title", "Данные пространства")}</b>
      <p className="muted" style={{ fontSize: 13 }}>
        {t("portability.hint", "Экспорт — один JSON-файл: списки, задачи, подзадачи, комментарии и заметки. Файлы вложений внутрь не попадают — только список того, что было приложено; сами файлы берёт резервная копия сервера.")}
      </p>

      {err && <div className="muted" style={{ color: "var(--danger, #c33)", marginBottom: 8 }}>{err}</div>}
      {result && <div className="muted" style={{ marginBottom: 8 }}>{result}</div>}

      <div className="row" style={{ gap: 8, flexWrap: "wrap", alignItems: "center" }}>
        <button className="btn" disabled={busy || !isOwner} onClick={exportSpace}
          title={isOwner ? "" : t("portability.export_owner_only",
            "Экспорт включает и приватные списки, поэтому доступен только владельцу пространства")}>
          {t("portability.export", "Скачать экспорт")}
        </button>

        <button className="btn" disabled={busy} onClick={() => fileRef.current?.click()}>
          {t("portability.import", "Импорт из файла…")}
        </button>
        <input ref={fileRef} type="file" accept=".json,application/json" style={{ display: "none" }}
          onChange={(e) => {
            const f = e.target.files?.[0]
            if (f) importFile(f)
          }} />
      </div>

      {!isOwner && (
        <p className="muted" style={{ fontSize: 12, marginBottom: 0 }}>
          {t("portability.export_owner_only",
            "Экспорт включает и приватные списки, поэтому доступен только владельцу пространства")}
        </p>
      )}
    </div>
  )
}
