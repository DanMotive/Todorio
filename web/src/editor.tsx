// MiniEditor: a textarea, a strip of Markdown buttons, and an inline picker for references.
//
// Not a WYSIWYG editor, and not a third-party one. Notes are stored as Markdown text and
// rendered by our own renderer (markdown.tsx, which builds React elements rather than HTML for
// XSS reasons); a rich-text widget would have to round-trip through HTML and would drag in a
// sanitiser, which is precisely the dependency that renderer exists to avoid.
//
// So: the text stays plain text and visible, and the buttons only insert the same characters a
// user could type by hand. The part that genuinely cannot be typed by hand is the reference -
// nobody knows that the task they mean is id 417 - and that is what the picker is for. Type `#`
// or `%` and the search runs; pick a row and the token is inserted.
//
// The button labels are the Markdown syntax itself (`**`, `` ` ``, `- [ ]`, `#`, `%`) rather
// than words. That is not a shortcut around localisation: the label of a bold button in a
// Markdown editor is the thing it inserts, it is the same in all 13 locales, and it teaches the
// syntax to someone who does not know it yet.

import { useEffect, useRef, useState } from "react"
import type React from "react"
import { api, type SearchResult } from "./api"
import { REF_MARKER, type RefKind } from "./refs"

type Picker = { kind: RefKind; from: number; query: string }

// Long enough that the first keystroke after `#` does not fire a search, short enough that the
// list appears while the user is still typing. Matches the server's own two-character minimum.
const MIN_QUERY = 2
const DEBOUNCE_MS = 200
const MAX_HITS = 6

export function MiniEditor({
  value, onChange, rows = 6, placeholder, autoFocus, onSubmit, textareaProps,
}: {
  value: string
  onChange: (next: string) => void
  rows?: number
  placeholder?: string
  autoFocus?: boolean
  // Ctrl/Cmd+Enter. The comment box submits on it; the note editor has a Save button and
  // passes nothing.
  onSubmit?: () => void
  textareaProps?: React.TextareaHTMLAttributes<HTMLTextAreaElement>
}) {
  const ta = useRef<HTMLTextAreaElement>(null)
  const [picker, setPicker] = useState<Picker | null>(null)
  const [hits, setHits] = useState<SearchResult[]>([])

  // Every mutation goes through here so the caret ends up where a person would expect it -
  // inside the emphasis they just opened, or after the reference they just picked. React
  // re-renders with the new value first, hence the frame delay.
  function apply(next: string, caret: number) {
    onChange(next)
    requestAnimationFrame(() => {
      const el = ta.current
      if (!el) return
      el.focus()
      el.setSelectionRange(caret, caret)
    })
  }

  function surround(before: string, after: string = before) {
    const el = ta.current
    if (!el) return
    const start = el.selectionStart
    const end = el.selectionEnd
    const selected = value.slice(start, end)
    const next = value.slice(0, start) + before + selected + after + value.slice(end)
    // With a selection, put the caret after the whole thing; with none, between the markers so
    // the user can just keep typing.
    apply(next, selected ? start + before.length + selected.length + after.length : start + before.length)
  }

  function prefixLine(prefix: string) {
    const el = ta.current
    if (!el) return
    const caret = el.selectionStart
    const lineStart = value.lastIndexOf("\n", caret - 1) + 1
    const next = value.slice(0, lineStart) + prefix + value.slice(lineStart)
    apply(next, caret + prefix.length)
  }

  // Opening the picker from a button inserts the marker too, so the text the user is looking at
  // matches what the picker is about to complete.
  function startPicker(kind: RefKind) {
    const el = ta.current
    const caret = el ? el.selectionStart : value.length
    const marker = REF_MARKER[kind]
    const next = value.slice(0, caret) + marker + value.slice(caret)
    setPicker({ kind, from: caret, query: "" })
    apply(next, caret + marker.length)
  }

  // Typing `#`/`%` opens the picker as well - the buttons are for discovery, this is for people
  // who already know. The marker has to start a word (the same rule the renderer applies), and
  // a second space closes the picker: at that point it is prose, not a lookup.
  function detectPicker(next: string, caret: number) {
    const before = next.slice(0, caret)
    const m = before.match(/(^|[^\w#%])([#%])([^\n#%]{0,40})$/)
    if (!m || m[3].includes("  ")) {
      setPicker(null)
      return
    }
    const kind: RefKind = m[2] === "#" ? "task" : "note"
    setPicker({ kind, from: caret - m[3].length - 1, query: m[3] })
  }

  useEffect(() => {
    if (!picker) {
      setHits([])
      return
    }
    const q = picker.query.trim()
    if (q.length < MIN_QUERY) {
      setHits([])
      return
    }
    let alive = true
    const timer = window.setTimeout(() => {
      api.get("/api/search?q=" + encodeURIComponent(q))
        .then((r) => {
          if (!alive) return
          const all: SearchResult[] = r?.results || []
          setHits(all.filter((x) => x.type === picker.kind).slice(0, MAX_HITS))
        })
        .catch(() => { if (alive) setHits([]) })
    }, DEBOUNCE_MS)
    return () => {
      alive = false
      window.clearTimeout(timer)
    }
  }, [picker?.kind, picker?.query])

  function choose(kind: RefKind, id: number) {
    const el = ta.current
    const caret = el ? el.selectionStart : value.length
    const from = picker ? picker.from : caret
    const token = REF_MARKER[kind] + id + " "
    const next = value.slice(0, from) + token + value.slice(caret)
    setPicker(null)
    apply(next, from + token.length)
  }

  const buttons: Array<{ label: string; hint: string; run: () => void }> = [
    { label: "B", hint: "**bold**", run: () => surround("**") },
    { label: "I", hint: "*italic*", run: () => surround("*") },
    { label: "S", hint: "~~strike~~", run: () => surround("~~") },
    { label: "<>", hint: "`code`", run: () => surround("`") },
    { label: "\u2022", hint: "- list", run: () => prefixLine("- ") },
    { label: "[ ]", hint: "- [ ] task line", run: () => prefixLine("- [ ] ") },
    { label: "\u201C", hint: "> quote", run: () => prefixLine("> ") },
    { label: "link", hint: "[text](https://)", run: () => surround("[", "](https://)") },
    { label: "#", hint: "#task", run: () => startPicker("task") },
    { label: "%", hint: "%note", run: () => startPicker("note") },
  ]

  return (
    <div className="mini-editor">
      <div className="row" style={{ gap: 4, flexWrap: "wrap", marginBottom: 4 }}>
        {buttons.map((b) => (
          <button key={b.hint} type="button" className="nav-btn" title={b.hint} aria-label={b.hint}
            style={{ padding: "2px 7px", fontSize: 12, minWidth: 26 }}
            // Buttons inside a form must never submit it, and the textarea must keep focus so
            // the caret position used above is still the one the user sees.
            onMouseDown={(e) => e.preventDefault()}
            onClick={b.run}>
            {b.label}
          </button>
        ))}
      </div>

      <textarea {...textareaProps} ref={ta} className="input" rows={rows} value={value}
        placeholder={placeholder} autoFocus={autoFocus}
        onChange={(e) => {
          onChange(e.target.value)
          detectPicker(e.target.value, e.target.selectionStart)
        }}
        onKeyDown={(e) => {
          if (e.key === "Escape" && picker) {
            // Close the picker without closing the modal underneath.
            e.stopPropagation()
            setPicker(null)
            return
          }
          if (e.key === "Enter" && (e.ctrlKey || e.metaKey) && onSubmit) {
            e.preventDefault()
            onSubmit()
          }
        }} />

      {picker && hits.length > 0 && (
        <div className="ref-picker" style={{ marginTop: 4, display: "flex", flexDirection: "column", gap: 2 }}>
          {hits.map((hit) => (
            <button key={hit.type + hit.id} type="button" className="nav-btn"
              style={{ textAlign: "left", justifyContent: "flex-start", fontSize: 13 }}
              onMouseDown={(e) => e.preventDefault()}
              onClick={() => choose(picker.kind, hit.id)}>
              <span className="muted" style={{ marginRight: 6 }}>{REF_MARKER[picker.kind]}{hit.id}</span>
              {"title" in hit ? hit.title : ""}
            </button>
          ))}
        </div>
      )}
    </div>
  )
}
