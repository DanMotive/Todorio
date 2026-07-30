// Cross-references between the two things this product stores: tasks and notes.
//
// The syntax is one character plus an id - `#123` for a task, `%45` for a note. Deliberately
// not `[[wiki links]]` or `@task/123`: `#` is what every issue tracker has trained people to
// type, `%` is next to it on the keyboard and is not used by Markdown, and both survive being
// pasted into a Telegram message or an email, which a bracket syntax does not.
//
// Nothing about a reference is stored. It is plain text in the note body or the description,
// resolved at render time - so deleting a task cannot corrupt a note, and a note exported to
// Markdown is still readable text rather than a graveyard of dead ids.
//
// Resolution goes through the ordinary GET endpoints, which already check access. A reference
// to something the reader cannot see resolves to nothing and stays plain text: it must not
// become a title-shaped hole that says "there is a task here you are not allowed to open".

import { useEffect, useState } from "react"
import { api } from "./api"

export type RefKind = "task" | "note"

export const REF_MARKER: Record<RefKind, string> = { task: "#", note: "%" }

// Matches a reference at the start of the string being scanned. The caller is responsible for
// the left boundary (see refBoundaryOk) because the inline scanner in markdown.tsx walks the
// text one position at a time and knows what came before; the lookahead here only stops
// `#12abc` from being read as a reference to 12.
export const REF_AT_START = /^([#%])(\d{1,9})(?![\w])/

// A reference has to start a word, or `foo#12` and a colour like `#123456` become links.
export function refBoundaryOk(prevChar: string): boolean {
  return prevChar === "" || !/[\w#%]/.test(prevChar)
}

type Resolved = { kind: RefKind; id: number; title: string } | null

// Module-level cache: a note listing the same task five times must not make five requests, and
// re-rendering the note must not repeat them. Kept for the life of the page - a title changing
// under a reader mid-session is not worth an invalidation protocol.
const cache = new Map<string, Resolved>()
const inflight = new Map<string, Promise<Resolved>>()

export async function resolveRef(kind: RefKind, id: number): Promise<Resolved> {
  const key = kind + ":" + id
  const cached = cache.get(key)
  if (cached !== undefined) return cached
  const pending = inflight.get(key)
  if (pending) return pending
  const request = (async (): Promise<Resolved> => {
    const url = kind === "task" ? `/api/tasks/${id}` : `/api/notes/${id}`
    const r = await api.get(url).catch(() => null)
    const row = kind === "task" ? r?.task : r?.note
    // 403 and 404 land here identically, which is the point: the reader learns only that this
    // is not a reference they can follow, not whether the id exists.
    const out: Resolved = row && typeof row.title === "string" ? { kind, id, title: row.title } : null
    cache.set(key, out)
    inflight.delete(key)
    return out
  })()
  inflight.set(key, request)
  return request
}

// Opening happens through an event rather than a prop chain: references are rendered deep
// inside note bodies, comment feeds and descriptions, and threading an onOpen callback through
// every one of those would touch every screen. The app already uses this pattern for the focus
// timer ("todorio:focus-changed").
export function openRef(kind: RefKind, id: number) {
  window.dispatchEvent(new CustomEvent("todorio:open-ref", { detail: { kind, id } }))
}

// RefLink renders one reference. Before it resolves - and forever, if it does not - it shows
// the raw token, so text never flickers between two different lengths and an unresolvable
// reference reads exactly as what the author typed.
export function RefLink({ kind, id }: { kind: RefKind; id: number }) {
  const key = kind + ":" + id
  const [row, setRow] = useState<Resolved>(cache.get(key) ?? null)
  const [settled, setSettled] = useState(cache.get(key) !== undefined)

  useEffect(() => {
    let alive = true
    resolveRef(kind, id).then((r) => {
      if (!alive) return
      setRow(r)
      setSettled(true)
    })
    return () => { alive = false }
  }, [kind, id])

  const token = REF_MARKER[kind] + id
  if (settled && !row) return <span className="ref-plain">{token}</span>
  return (
    <button type="button" className="linklike ref-link" title={row ? row.title : token}
      // Note bodies and task rows are themselves clickable in places; a reference is its own
      // action and must not also open whatever it happens to sit inside.
      onClick={(e) => { e.stopPropagation(); openRef(kind, id) }}>
      {REF_MARKER[kind]}{row ? row.title : id}
    </button>
  )
}

// App listens for todorio:open-ref and converts it into the canonical task/note route.
// Keeping reference resolution and navigation separate means read-only rendering can still use
// this module without importing modal or router code.
