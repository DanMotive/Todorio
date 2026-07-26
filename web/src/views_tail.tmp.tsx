ks/${menu.task.id}`, patch).catch((e) => setCreateError((e as Error).message))
            if (patch.status === "done") window.dispatchEvent(new CustomEvent("todorio:focus-changed"))
            setMenu(null)
            load()
          }}
          onOpenFull={() => { setOpen(menu.task); setMenu(null) }} />
      )}
      {open && <TaskModal task={open} me={me} spaceId={spaceId} onClose={() => setOpen(null)} onChanged={load} />}
    </div>
  )
}

const KIND_ICON: Record<string, React.ReactNode> = {
  approved: <IconCheckCircle size={15} />, task_assigned: <IconPin size={15} />,
  comment: <IconMessage size={15} />, reaction: <IconStar size={15} />,
  overdue: <IconClock size={15} />, space_added: <IconGrid size={15} />, list_shared: <IconList size={15} />,
  status_changed: <IconRefresh size={15} />, due_changed: <IconClock size={15} />,
  due_soon: <IconClock size={15} />, due_today: <IconAlertCircle size={15} />,
  archive_expiring: <IconArchive size={15} />,
}

export function NotificationsPage({ onRead }: { onRead: () => void }) {
  const [items, setItems] = useState<any[]>([])
  const load = () => api.get("/api/notifications").then((r) => setItems(r.notifications)).catch(() => {})
  useEffect(() => { load() }, [])

  return (
    <div className="card">
      <div className="row">
        <h2 className="grow">{tr("notif.title")}</h2>
        <button className="nav-btn" onClick={async () => { await api.post("/api/notifications/read"); load(); onRead() }}>
          {tr("notif.read_all")}
        </button>
      </div>
      {items.length === 0 && <p className="muted">{tr("notif.empty")}</p>}
      {items.map((n) => (
        <div key={n.id} className="task-row" style={{ opacity: n.read_at ? 0.55 : 1 }}>
          <span className="task-title row" style={{ gap: 6 }}>
            {KIND_ICON[n.kind] || null}
            <span>
              {tr("notif.kind." + n.kind)}
              {n.kind === "due_soon" && n.payload?.days
                ? tr("notif.days_suffix").replace("{days}", String(n.payload.days)) : ""}
              {n.kind === "archive_expiring" && n.payload?.days_left
                ? tr("notif.days_suffix").replace("{days}", String(n.payload.days_left)) : ""}
              {n.payload?.title ? ` · «${n.payload.title}»` : ""}
              {n.payload?.task_title ? ` · «${n.payload.task_title}»` : ""}
              {n.payload?.by ? ` · ${tr("notif.by")} @${n.payload.by}` : ""}
              {n.payload?.emoji ? ` ${n.payload.emoji}` : ""}
            </span>
          </span>
          <span className="muted">{new Date(n.created_at).toLocaleString(getFormattingLocale())}</span>
        </div>
      ))}
    </div>
  )
}

export function AdminPage({ me }: { me: Me }) {
  const { confirm, confirmElement } = useConfirm()
  const [users, setUsers] = useState<any[]>([])
  const [tempPass, setTempPass] = useState<{ user: string; pass: string } | null>(null)
  const load = () => api.get("/api/admin/users").then((r) => setUsers(r.users)).catch(() => {})
  useEffect(() => { load() }, [])

  return (
    <div className="card">
      {confirmElement}
      <h2>{trFormal("admin.users")}</h2>
      {tempPass && (
        <div className="card" style={{ borderColor: "var(--accent)", marginBottom: 12 }}>
          {trFormal("admin.temp_pass_for")} <b>@{tempPass.user}</b>: <code>{tempPass.pass}</code>
          <div className="muted">{trFormal("admin.shown_once")}</div>
        </div>
      )}
      {users.map((u) => (
        <div key={u.id} className="task-row" style={{ cursor: "default" }}>
          <span className="task-title">
            @{u.username} <span className="muted">· {u.role} · {u.status}</span>
          </span>
          {u.status === "pending" && (
            <>
              <button className="btn" onClick={async () => { await api.post(`/api/admin/users/${u.id}/approve`, { role: "user" }); load() }}>
                {trFormal("admin.approve")}
              </button>
              <button className="nav-btn" onClick={async () => { await api.post(`/api/admin/users/${u.id}/status`, { status: "rejected" }); load() }}>
                {trFormal("admin.reject")}
              </button>
            </>
          )}
          {u.status === "active" && u.role !== "root" && (
            <>
              <button className="nav-btn" onClick={() => confirm({
                title: trFormal("confirm.block_title").replace("{user}", u.username),
                body: trFormal("confirm.block_body"),
                confirmLabel: trFormal("admin.block"), danger: true,
                action: async () => { await api.post(`/api/admin/users/${u.id}/status`, { status: "blocked" }); load() },
              })}>
                {trFormal("admin.block")}
              </button>
              <button className="nav-btn" onClick={() => confirm({
                title: trFormal("confirm.reset_pw_title").replace("{user}", u.username),
                body: trFormal("confirm.reset_pw_body"),
                confirmLabel: trFormal("admin.reset_password"), danger: true,
                action: async () => {
                  const r = await api.post(`/api/admin/users/${u.id}/reset-password`)
                  setTempPass({ user: u.username, pass: r.temp_password })
                },
              })}>
                {trFormal("admin.reset_password")}
              </button>
            </>
          )}
          {u.status === "blocked" && (
            <button className="btn" onClick={async () => { await api.post(`/api/admin/users/${u.id}/status`, { status: "active" }); load() }}>
              {trFormal("admin.unblock")}
            </button>
          )}
        </div>
      ))}
    </div>
  )
}
