// Minimal Markdown renderer for notes (spec section 12: "заметки (Markdown-страницы)").
//
// Written by hand rather than pulling in a Markdown library, for two reasons:
//   1. The product ships as one self-contained binary; a parser plus its sanitiser would be a
//      large share of the whole JS bundle for one screen.
//   2. Every library that renders to HTML needs a sanitiser bolted on, and getting that wrong is
//      a stored-XSS hole. This renderer never produces HTML strings at all — it builds React
//      elements, so note text is always escaped by React and cannot inject markup. There is no
//      dangerouslySetInnerHTML anywhere in this file, by design.
//
// Supported, deliberately a subset: ATX headings, unordered and ordered lists, blockquotes,
// fenced and inline code, bold, italic, strikethrough, links, references to tasks and notes,
// horizontal rules, and paragraphs. Anything unsupported degrades to plain text rather than
// disappearing.

import type React from "react"
import { REF_AT_START, RefLink, refBoundaryOk } from "./refs"

// Only http(s) and mailto links become anchors. A "javascript:" URL in a note must never be
// clickable — notes are user content and other members of the space read them.
function safeHref(url: string): string | null {
  const u = url.trim()
  if (/^https?:\/\//i.test(u) || /^mailto:/i.test(u)) return u
  return null
}

// renderInline handles the span-level syntax. It walks the string once, so a link's label can
// itself contain bold or code without the patterns fighting each other.
function renderInline(text: string, keyPrefix: string): React.ReactNode[] {
  const out: React.ReactNode[] = []
  let rest = text
  let k = 0

  // Ordered by precedence: code first, because backticks must win over emphasis inside them.
  //
  // `boundary` marks a rule that may only fire at the start of a word. References need it —
  // otherwise a colour like #336699 and the "12" in "issue#12abc" would both light up — and the
  // scanner below is the only place that knows which character preceded this position.
  const rules: Array<{
    re: RegExp
    boundary?: boolean
    render: (m: RegExpMatchArray, key: string) => React.ReactNode
  }> = [
    { re: /^`([^`]+)`/, render: (m, key) => <code key={key}>{m[1]}</code> },
    {
      re: /^\[([^\]]*)\]\(([^)\s]+)\)/,
      render: (m, key) => {
        const href = safeHref(m[2])
        // Not a safe scheme: show the label as plain text rather than a dead or dangerous link.
        if (!href) return <span key={key}>{m[1]}</span>
        return (
          <a key={key} href={href} target="_blank" rel="noreferrer noopener">
            {m[1] || href}
          </a>
        )
      },
    },
    {
      re: REF_AT_START,
      boundary: true,
      render: (m, key) => (
        <RefLink key={key} kind={m[1] === "#" ? "task" : "note"} id={Number(m[2])} />
      ),
    },
    { re: /^\*\*([^*]+)\*\*/, render: (m, key) => <strong key={key}>{m[1]}</strong> },
    { re: /^__([^_]+)__/, render: (m, key) => <strong key={key}>{m[1]}</strong> },
    { re: /^~~([^~]+)~~/, render: (m, key) => <del key={key}>{m[1]}</del> },
    { re: /^\*([^*]+)\*/, render: (m, key) => <em key={key}>{m[1]}</em> },
    { re: /^_([^_]+)_/, render: (m, key) => <em key={key}>{m[1]}</em> },
  ]

  let buffer = ""
  const flush = () => {
    if (buffer) {
      out.push(buffer)
      buffer = ""
    }
  }

  while (rest.length > 0) {
    let matched = false
    for (const rule of rules) {
      // The character immediately before this position, as far as this scan knows: an empty
      // string at the very start, or right after another inline element ended.
      if (rule.boundary && !refBoundaryOk(buffer.slice(-1))) continue
      const m = rest.match(rule.re)
      if (m) {
        flush()
        out.push(rule.render(m, `${keyPrefix}-i${k++}`))
        rest = rest.slice(m[0].length)
        matched = true
        break
      }
    }
    if (!matched) {
      // No rule applied at this position — consume one character and keep scanning. This is what
      // makes unsupported or malformed syntax degrade to plain text instead of being dropped.
      buffer += rest[0]
      rest = rest.slice(1)
    }
  }
  flush()
  return out
}

// renderMarkdown converts note text into React elements.
export function renderMarkdown(src: string): React.ReactNode {
  const lines = src.replace(/\r\n/g, "\n").split("\n")
  const blocks: React.ReactNode[] = []
  let i = 0
  let key = 0

  while (i < lines.length) {
    const line = lines[i]

    // Fenced code block. An unterminated fence runs to the end of the note rather than
    // swallowing the rest as invisible markup.
    const fence = line.match(/^```(\w*)\s*$/)
    if (fence) {
      const body: string[] = []
      i++
      while (i < lines.length && !/^```\s*$/.test(lines[i])) {
        body.push(lines[i])
        i++
      }
      i++ // skip the closing fence (or run past the end)
      blocks.push(<pre key={`b${key++}`} className="md-pre"><code>{body.join("\n")}</code></pre>)
      continue
    }

    // Horizontal rule
    if (/^\s*([-*_])\s*(\1\s*){2,}$/.test(line)) {
      blocks.push(<hr key={`b${key++}`} className="md-hr" />)
      i++
      continue
    }

    // ATX heading (# … ######). A heading needs a space after the hashes, so "#123" stays a
    // task reference and is handled inline below.
    const h = line.match(/^(#{1,6})\s+(.*)$/)
    if (h) {
      const level = h[1].length
      const Tag = (`h${Math.min(level + 1, 6)}`) as "h2"
      // Shifted by one: a note's own title is the page's h1, so its "#" starts at h2 and the
      // document keeps a sane heading order.
      blocks.push(
        <Tag key={`b${key++}`} className="md-h">{renderInline(h[2], `b${key}`)}</Tag>,
      )
      i++
      continue
    }

    // Blockquote (consecutive "> " lines)
    if (/^>\s?/.test(line)) {
      const body: string[] = []
      while (i < lines.length && /^>\s?/.test(lines[i])) {
        body.push(lines[i].replace(/^>\s?/, ""))
        i++
      }
      blocks.push(
        <blockquote key={`b${key++}`} className="md-quote">
          {renderInline(body.join(" "), `b${key}`)}
        </blockquote>,
      )
      continue
    }

    // Unordered list
    if (/^\s*[-*+]\s+/.test(line)) {
      const items: string[] = []
      while (i < lines.length && /^\s*[-*+]\s+/.test(lines[i])) {
        items.push(lines[i].replace(/^\s*[-*+]\s+/, ""))
        i++
      }
      blocks.push(
        <ul key={`b${key++}`} className="md-list">
          {items.map((it, n) => <li key={n}>{renderInline(it, `b${key}-${n}`)}</li>)}
        </ul>,
      )
      continue
    }

    // Ordered list
    if (/^\s*\d+[.)]\s+/.test(line)) {
      const items: string[] = []
      while (i < lines.length && /^\s*\d+[.)]\s+/.test(lines[i])) {
        items.push(lines[i].replace(/^\s*\d+[.)]\s+/, ""))
        i++
      }
      blocks.push(
        <ol key={`b${key++}`} className="md-list">
          {items.map((it, n) => <li key={n}>{renderInline(it, `b${key}-${n}`)}</li>)}
        </ol>,
      )
      continue
    }

    // Blank line — nothing to emit; paragraph breaks are handled by grouping below.
    if (line.trim() === "") {
      i++
      continue
    }

    // Paragraph: consecutive non-blank lines that didn't match any block rule above.
    const para: string[] = []
    while (
      i < lines.length &&
      lines[i].trim() !== "" &&
      !/^```/.test(lines[i]) &&
      !/^(#{1,6})\s+/.test(lines[i]) &&
      !/^>\s?/.test(lines[i]) &&
      !/^\s*[-*+]\s+/.test(lines[i]) &&
      !/^\s*\d+[.)]\s+/.test(lines[i])
    ) {
      para.push(lines[i])
      i++
    }
    blocks.push(
      <p key={`b${key++}`} className="md-p">{renderInline(para.join(" "), `b${key}`)}</p>,
    )
  }

  return <div className="md">{blocks}</div>
}
