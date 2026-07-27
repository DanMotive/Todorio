// Outgoing webhooks for a space: register a URL, see whether deliveries are arriving, delete it.
//
// The feature is inert until this screen is used — no endpoint means the server never tries to
// send anything — so the empty state has to explain what the thing is for rather than just say
// "nothing here".
//
// Own module for the same reason sharing.tsx and members.tsx are: views.tsx is already enormous.

import { useEffect, useState } from "react"
import { api } from "./api"
import { tr } from "./i18n"
import { useConfirm } from "./extras"

// tr() with an inline fallback — same helper as sharing.tsx; these keys are not in the locale
// files yet and tr() returns "" for an unknown key, which would render blank buttons.
const t = (key: string, fallback: string) => tr(key) || fallback

export type Webhook = {
  id: number
  url: string
  events: string[]
  is_active: boolean
  has_secret: boolean
  last_status: number | null
  last_error: string
  last_delivery_at: string | null
  failure_count: number
  created_at: string
}

/** Webhook endpoints for one space. Space owners only — the server enforces it as well. */
export function WebhooksCard({ spaceId }: { spaceId: number }) {
  const [hooks, setHooks] = useState<Webhook[] | null>(null)
  const [eventTypes, setEventTypes] = useState<string[]>([])
  const [notOwner, setNotOwner] = useState(false)
  const [err, setErr] = useState("")
  const [busy, setBusy] = useState(false)
  const [adding, setAdding] = useState(false)
  const [url, setUrl] = useState("")
  const [secret, setSecret] = useState("")
  const [picked, setPicked] = useState<string[]>([])
  const [testResult, setTestResult] = useState<Record<number, string>>({})
  const { confirm, confirmElement } = useConfirm()

  async function load() {
    setErr("")
    setNotOwner(false)
    try {
      const r = await api.get(`/api/spaces/${spaceId}/webhooks`)
      setHooks(r.webhooks || [])
      setEventTypes(r.event_types || [])
    } catch (e) {
      const status = (e as { status?: number }).status
      // 403 is the ordinary answer for a member who is not the owner, not a fault worth
      // reporting in red.
      if (status === 403) setNotOwner(true)
      else setErr((e as Error).message)
      setHooks([])
    }
  }

  useEffect(() => { setHooks(null); load() }, [spaceId])

  async function add() {
    setBusy(true)
    setErr("")
    try {
      await api.post(`/api/spaces/${spaceId}/webhooks`, {
        url,
        secret,
        // An empty selection means every event, which is what the server does with an empty list.
        events: picked,
      })
      setUrl("")
      setSecret("")
      setPicked([])
      setAdding(false)
      await load()
    } catch (e) {
      setErr((e as Error).message)
    } finally {
      setBusy(false)
    }
  }

  async function toggle(h: Webhook) {
    setBusy(true)
    setErr("")
    try {
      await api.patch(`/api/webhooks/${h.id}`, { is_active: !h.is_active })
      await load()
    } catch (e) {
      setErr((e as Error).message)
    } finally {
      setBusy(false)
    }
  }

  async function sendTest(h: Webhook) {
    setBusy(true)
    setErr("")
    setTestResult((prev) => ({ ...prev, [h.id]: t("webhooks.testing", "Отправляем…") }))
    try {
      const r = await api.post(`/api/webhooks/${h.id}/test`, {})
      setTestResult((prev) => ({
        ...prev,
        [h.id]: r.ok
          ? `${t("webhooks.test_ok", "Доставлено")} (HTTP ${r.status})`
          : `${t("webhooks.test_failed", "Не доставлено")}: ${r.error || `HTTP ${r.status}`}`,
      }))
      await load()
    } catch (e) {
      setTestResult((prev) => ({ ...prev, [h.id]: (e as Error).message }))
    } finally {
      setBusy(false)
    }
  }

  function remove(h: Webhook) {
    confirm({
      title: t("webhooks.delete_confirm", "Удалить эндпоинт? События перестанут на него отправляться."),
      confirmLabel: t("webhooks.delete", "Удалить"),
      danger: true,
      action: async () => {
        setBusy(true)
        setErr("")
        try {
          await api.del(`/api/webhooks/${h.id}`)
          await load()
        } catch (e) {
          setErr((e as Error).message)
        } finally {
          setBusy(false)
        }
      },
    })
  }

  if (hooks === null) return <div className="muted">{t("webhooks.loading", "Загрузка…")}</div>

  if (notOwner) {
    return (
      <div className="card">
        <b>{t("webhooks.title", "Вебхуки")}</b>
        <p className="muted" style={{ fontSize: 13, marginBottom: 0 }}>
          {t("webhooks.owner_only", "Вебхуками управляет только владелец пространства.")}
        </p>
      </div>
    )
  }

  return (
    <div className="card">
      {confirmElement}
      <b>{t("webhooks.title", "Вебхуки")}</b>
      <p className="muted" style={{ fontSize: 13 }}>
        {t("webhooks.hint", "Когда в пространстве что-то происходит, сервер отправляет POST с JSON на указанный адрес. Пока адрес не добавлен, не отправляется ничего.")}
      </p>

      {err && <div className="muted" style={{ color: "var(--danger, #c33)", marginBottom: 8 }}>{err}</div>}

      {hooks.length === 0 && (
        <div className="muted" style={{ marginBottom: 10, fontSize: 13 }}>
          {t("webhooks.empty", "Эндпоинтов нет")}
        </div>
      )}

      {hooks.map((h) => (
        <div key={h.id} style={{ padding: "8px 0", borderTop: "1px solid var(--border)" }}>
          <div className="row" style={{ gap: 8, alignItems: "center", flexWrap: "wrap" }}>
            <code style={{ fontSize: 12, overflow: "hidden", textOverflow: "ellipsis", maxWidth: 320 }}>
              {h.url}
            </code>
            {!h.is_active && <span className="badge">{t("webhooks.off", "выключен")}</span>}
            {h.has_secret && (
              <span className="badge" title={t("webhooks.signed_hint",
                "Каждая доставка подписывается заголовком X-Todorio-Signature")}>
                {t("webhooks.signed", "с подписью")}
              </span>
            )}
            <span className="muted" style={{ fontSize: 12 }}>
              {h.events.length === 0
                ? t("webhooks.all_events", "все события")
                : h.events.join(", ")}
            </span>

            <div className="row" style={{ gap: 6, marginLeft: "auto" }}>
              <button className="btn" disabled={busy} onClick={() => sendTest(h)}>
                {t("webhooks.test", "Проверить")}
              </button>
              <button className="btn" disabled={busy} onClick={() => toggle(h)}>
                {h.is_active ? t("webhooks.disable", "Выключить") : t("webhooks.enable", "Включить")}
              </button>
              <button className="ctrl-btn" disabled={busy} title={t("webhooks.delete", "Удалить")}
                onClick={() => remove(h)}>
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><line x1="18" y1="6" x2="6" y2="18" /><line x1="6" y1="6" x2="18" y2="18" /></svg>
              </button>
            </div>
          </div>

          {/* Last delivery, so a silently dead endpoint is visible instead of looking healthy. */}
          <div className="muted" style={{ fontSize: 12, marginTop: 4 }}>
            {h.last_delivery_at ? (
              <>
                {t("webhooks.last", "Последняя отправка")}: {new Date(h.last_delivery_at).toLocaleString()}
                {h.last_status ? ` — HTTP ${h.last_status}` : ""}
                {h.last_error && (
                  <span style={{ color: "var(--danger, #c33)" }}> — {h.last_error}</span>
                )}
                {h.failure_count > 0 && (
                  <span style={{ color: "var(--danger, #c33)" }}>
                    {" "}({t("webhooks.failures", "неудач подряд")}: {h.failure_count})
                  </span>
                )}
              </>
            ) : (
              t("webhooks.never", "Отправок ещё не было")
            )}
          </div>

          {testResult[h.id] && (
            <div className="muted" style={{ fontSize: 12, marginTop: 4 }}>{testResult[h.id]}</div>
          )}
        </div>
      ))}

      {!adding ? (
        <div style={{ marginTop: 10 }}>
          <button className="btn" disabled={busy} onClick={() => setAdding(true)}>
            {t("webhooks.add", "Добавить эндпоинт")}
          </button>
        </div>
      ) : (
        <div style={{ marginTop: 10, borderTop: "1px solid var(--border)", paddingTop: 10 }}>
          <input className="input" value={url} disabled={busy} placeholder="https://example.com/hook"
            onChange={(e) => setUrl(e.target.value)} />
          <input className="input" style={{ marginTop: 6 }} value={secret} disabled={busy}
            autoComplete="new-password"
            placeholder={t("webhooks.secret_placeholder", "Секрет для подписи (необязательно)")}
            onChange={(e) => setSecret(e.target.value)} />
          <p className="muted" style={{ fontSize: 12, marginTop: 6, marginBottom: 6 }}>
            {t("webhooks.secret_hint", "Секрет больше не показывается: тело каждой доставки подписывается HMAC-SHA256 и приходит в заголовке X-Todorio-Signature.")}
          </p>

          <div className="row" style={{ gap: 10, flexWrap: "wrap", marginBottom: 6 }}>
            {eventTypes.map((ev) => (
              <label key={ev} className="row" style={{ gap: 4, alignItems: "center", fontSize: 13 }}>
                <input type="checkbox" checked={picked.includes(ev)} disabled={busy}
                  onChange={(e) => setPicked(e.target.checked
                    ? [...picked, ev]
                    : picked.filter((x) => x !== ev))} />
                {ev}
              </label>
            ))}
          </div>
          <p className="muted" style={{ fontSize: 12, marginTop: 0 }}>
            {t("webhooks.all_events_hint", "Если не отмечено ничего, будут отправляться все события.")}
          </p>

          <div className="row" style={{ gap: 6 }}>
            <button className="btn" disabled={busy || !url.trim()} onClick={add}>
              {t("webhooks.save", "Сохранить")}
            </button>
            <button className="btn" disabled={busy} onClick={() => {
              setAdding(false)
              setUrl("")
              setSecret("")
              setPicked([])
            }}>
              {t("webhooks.cancel", "Отмена")}
            </button>
          </div>
        </div>
      )}
    </div>
  )
}
