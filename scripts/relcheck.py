#!/usr/bin/env python3
# Release checks that do not need a compiler:
#  1. every locale file is valid JSON and has the same keys as en-US
#  2. every tr()/trFormal() key used by the frontend exists in en-US.json
#  3. every /api/... call made by the frontend matches a route registered in the Go mux
#  4. braces/parens balance in the files touched in stages 4-6
import json, os, re, sys, glob

root = "/data/todorio_src/Todorio"
web = os.path.join(root, "web/src")
loc = os.path.join(web, "locales")
fails = 0

def fail(msg):
    global fails
    fails += 1
    print("FAIL: " + msg)

# ---- 1. locales ----
en = json.load(open(os.path.join(loc, "en-US.json"), encoding="utf-8"))
for path in sorted(glob.glob(os.path.join(loc, "*.json"))):
    name = os.path.basename(path)
    try:
        data = json.load(open(path, encoding="utf-8"))
    except Exception as e:
        fail("%s is not valid JSON: %s" % (name, e))
        continue
    if name.endswith("-it.json"):
        extra = set(data) - set(en)
        if extra:
            fail("%s has keys missing from en-US: %s" % (name, sorted(extra)[:5]))
        continue
    missing = set(en) - set(data)
    extra = set(data) - set(en)
    if missing:
        fail("%s missing %d keys: %s" % (name, len(missing), sorted(missing)[:5]))
    if extra:
        fail("%s has %d unknown keys: %s" % (name, len(extra), sorted(extra)[:5]))
print("locales checked: %d files, %d keys in en-US" % (len(glob.glob(os.path.join(loc, '*.json'))), len(en)))

# ---- 2. tr() keys ----
src_files = [p for p in glob.glob(os.path.join(web, "*.ts*"))]
key_re = re.compile(r'\btr(?:Formal)?\("([A-Za-z0-9_.]+)"\)')
used = set()
for p in src_files:
    used |= set(key_re.findall(open(p, encoding="utf-8").read()))
unknown = sorted(k for k in used if k not in en)
if unknown:
    fail("%d tr() keys are not in en-US.json: %s" % (len(unknown), unknown[:10]))
print("tr() keys used: %d, all present: %s" % (len(used), not unknown))

# ---- 3. frontend API calls vs Go routes ----
go_route = re.compile(r'mux\.HandleFunc\("(GET|POST|PATCH|PUT|DELETE) (/api/[^"]*)"')
routes = set()
for p in glob.glob(os.path.join(root, "internal/api/*.go")) + glob.glob(os.path.join(root, "internal/server/*.go")):
    for m, path in go_route.findall(open(p, encoding="utf-8").read()):
        routes.add((m, re.sub(r"\{[^}]*\}", "{}", path.rstrip("/"))))

call_re = re.compile(r'\bapi\.(get|post|patch|put|del)\(\s*(`[^`]*`|"[^"]*")')
meth = {"get": "GET", "post": "POST", "patch": "PATCH", "put": "PUT", "del": "DELETE"}
calls = set()
for p in src_files:
    for verb, raw in call_re.findall(open(p, encoding="utf-8").read()):
        url = raw.strip("`\"")
        url = re.sub(r"\$\{[^}]*\}", "{}", url)
        url = url.split("?")[0].rstrip("/")
        if url.startswith("/api/"):
            calls.add((meth[verb], url, os.path.basename(p)))

for m, url, where in sorted(calls):
    # A trailing {} usually comes from a template literal that only appends a query string.
    alt = url[:-2].rstrip("/") if url.endswith("{}") else url
    if (m, url) not in routes and (m, alt) not in routes:
        fail("%s calls %s %s - no such route on the server" % (where, m, url))
print("api calls checked: %d against %d routes" % (len(calls), len(routes)))

# ---- 4. brace balance ----
touched = [
    "web/src/views.tsx", "web/src/extras.tsx", "web/src/functional.tsx", "web/src/settings.tsx",
    "web/src/App.tsx", "web/src/api.ts",
    "internal/api/api.go", "internal/api/settings.go", "internal/api/unblock.go",
    "internal/api/workload.go", "internal/api/notes_tasks.go", "internal/api/import_external.go",
    "internal/api/telegram_personal.go", "internal/server/server.go", "cmd/todorio/main.go",
]
for rel in touched:
    text = open(os.path.join(root, rel), encoding="utf-8").read()
    for opener, closer in (("{", "}"), ("(", ")"), ("[", "]")):
        if text.count(opener) != text.count(closer):
            fail("%s: %s=%d but %s=%d" % (rel, opener, text.count(opener), closer, text.count(closer)))
print("brace balance checked in %d files" % len(touched))

print("\n%s" % ("ALL CHECKS PASSED" if fails == 0 else "%d CHECK(S) FAILED" % fails))
sys.exit(1 if fails else 0)
