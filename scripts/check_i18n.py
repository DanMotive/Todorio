#!/usr/bin/env python3
"""Verify i18n integrity across Todorio's locale files.

Four classes of bug this catches, all of which ship silently otherwise:

1. A tr()/trFormal()/trOr() call in the TSX with no matching key in a locale file —
   the UI renders the raw key string ("task.done") to the user.
2. A key present in en-US but missing from one of the other 12 base locales — that
   locale silently falls back, so the string appears in English mid-sentence.
3. Emoji in interface strings. The product deliberately keeps emoji out of the UI
   (they render differently per OS/browser); the fixed reaction set is user content
   and lives in Go, not here, so anything found in a locale file is a regression.
4. Keys no call site references any more — dead weight that every translator still
   has to carry. Reported for information only unless --strict is passed, because a
   call site can reach a key in ways this script cannot see statically.

trOr(key, fallback) deserves a note: t() returns the key itself when a lookup misses,
so the older tr("key") || "fallback" idiom was dead code — the left side was always
truthy. trOr() compares against the key and is the reason this checker has to know
about it: a missing key behind trOr() renders a human-readable fallback instead of a
raw key, which is nicer for users and completely invisible to the eye during review.

Comments are stripped before the scan. A comment that spells out an example call is
documentation, not a call site — web/src/wallpaper.tsx explains this very checker and
quotes tr("...") while doing so, which used to make the run demand a key literally
named "...". The failure was reported against en-US with no file or line, so the only
way to find it was to grep the tree.

Dynamic calls like tr("profile.type." + k) can't be resolved statically, so their
literal prefix is recorded and any key starting with it counts as satisfying it.

Exit code 1 on any problem, so this can gate CI or a release.
"""
import argparse
import json
import os
import re
import sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
SRC = os.path.join(ROOT, "web", "src")
LOCALES = os.path.join(SRC, "locales")

BASE = "en-US"
# The -it files are slang overlays: a partial set on purpose. Missing keys fall back
# to the base locale through t(), so they're exempt from the completeness check.
OVERLAYS = {"ru-RU-it", "en-US-it"}

EMOJI = re.compile("[\U0001F300-\U0001FAFF☀-➿]")
# tr("literal") / trFormal("literal") / trOr("literal", "fallback") — and the dynamic
# tr("prefix." + expr) form. Keep this list in sync with the helpers exported from
# web/src/i18n.ts; a helper missing here means its keys go unchecked.
CALL = re.compile(r'\b(?:tr|trFormal|trOr)\(\s*"([^"]+)"')


def strip_comments(text):
    """Blank out // line comments and /* */ blocks, keeping string literals intact.

    Naively cutting at the first // would also cut at the // inside "https://...",
    dropping the rest of that line and with it any real call sites on it, so double
    and backtick quoted strings are tracked and skipped over.

    Single quotes are deliberately not treated as string delimiters: apostrophes in
    prose ("don't") are far more common in this codebase than single-quoted strings
    containing a //, and treating one as an opening quote would swallow real code up
    to the next apostrophe. Regex literals are not parsed either — a bare // inside
    one would need writing as [/][/] to survive, which nothing here does.
    """
    out = []
    i, n = 0, len(text)
    quote = None
    while i < n:
        ch = text[i]
        if quote is not None:
            out.append(ch)
            if ch == "\\" and i + 1 < n:
                out.append(text[i + 1])
                i += 2
                continue
            if ch == quote:
                quote = None
            i += 1
            continue
        nxt = text[i + 1] if i + 1 < n else ""
        if ch == "/" and nxt == "/":
            while i < n and text[i] != "\n":
                i += 1
            continue
        if ch == "/" and nxt == "*":
            end = text.find("*/", i + 2)
            i = n if end == -1 else end + 2
            # A space, not nothing: the tokens either side of a block comment must not
            # be glued together, or tr("k") /* note */ + x would read as a dynamic call.
            out.append(" ")
            continue
        if ch == '"' or ch == "`":
            quote = ch
        out.append(ch)
        i += 1
    return "".join(out)


def load(name):
    with open(os.path.join(LOCALES, name + ".json"), encoding="utf-8") as fh:
        return json.load(fh)


def main():
    ap = argparse.ArgumentParser(description="Check Todorio locale files.")
    ap.add_argument(
        "--strict",
        action="store_true",
        help="also fail when a base-locale key is referenced by no call site",
    )
    args = ap.parse_args()

    files = sorted(
        f[:-5] for f in os.listdir(LOCALES) if f.endswith(".json")
    )
    data = {name: load(name) for name in files}
    base_keys = set(data[BASE])
    problems = []

    # 1. every base locale carries the same key set
    for name in files:
        if name in OVERLAYS:
            continue
        keys = set(data[name])
        missing = base_keys - keys
        extra = keys - base_keys
        for k in sorted(missing):
            problems.append(f"{name}: missing key {k!r}")
        for k in sorted(extra):
            problems.append(f"{name}: key {k!r} not in {BASE}")

    # 2. no emoji anywhere in interface strings
    for name in files:
        for k, v in data[name].items():
            if isinstance(v, str) and EMOJI.search(v):
                problems.append(f"{name}: emoji in {k!r}: {v!r}")

    # 3. every tr()/trFormal()/trOr() call in the TSX resolves to a real key
    used, prefixes = set(), set()
    where = {}
    for fn in sorted(os.listdir(SRC)):
        if not fn.endswith((".tsx", ".ts")):
            continue
        path = os.path.join(SRC, fn)
        with open(path, encoding="utf-8") as fh:
            text = strip_comments(fh.read())
        for m in CALL.finditer(text):
            key = m.group(1)
            after = text[m.end():m.end() + 40].lstrip()
            where.setdefault(key, fn)
            # tr("prefix." + something) — dynamic, treat as a prefix
            if after.startswith("+"):
                prefixes.add(key)
            else:
                used.add(key)

    # Name the file. A key that exists in no locale is usually a typo at one call site,
    # and "en-US: tr('task.dne') used in TSX but no such key" left the reader grepping.
    for key in sorted(used):
        if key not in base_keys:
            problems.append(
                f"{BASE}: tr({key!r}) used in {where.get(key, 'TSX')} but no such key"
            )
    for pref in sorted(prefixes):
        if not any(k.startswith(pref) for k in base_keys):
            problems.append(
                f"{BASE}: dynamic tr({pref!r} + ...) in {where.get(pref, 'TSX')} matches no key"
            )

    # 4. keys nothing references. A key can legitimately look unused when its call
    # site builds it dynamically from a value this script cannot follow, so this is
    # a report by default and only fails the run under --strict.
    unused = sorted(
        k
        for k in base_keys
        if k not in used and not any(k.startswith(p) for p in prefixes)
    )

    counts = ", ".join(
        f"{n}={len(data[n])}" for n in files if n not in OVERLAYS
    )
    print(f"base locales: {counts}")
    print("overlays: " + ", ".join(f"{n}={len(data[n])}" for n in sorted(OVERLAYS)))
    print(f"tr() literals: {len(used)}, dynamic prefixes: {len(prefixes)}")

    if unused:
        print(f"unreferenced keys: {len(unused)}")
        for k in unused:
            print("  " + k)
        if args.strict:
            problems.extend(f"{BASE}: key {k!r} referenced by no call site" for k in unused)
    else:
        print("unreferenced keys: none")

    if problems:
        print(f"\n{len(problems)} problem(s):", file=sys.stderr)
        for p in problems:
            print("  " + p, file=sys.stderr)
        return 1
    print("\ni18n OK")
    return 0


if __name__ == "__main__":
    sys.exit(main())
