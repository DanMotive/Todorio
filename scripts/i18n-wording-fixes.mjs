#!/usr/bin/env node
// Per-language wording fixes for web/src/locales/*.json.
//
// These are not missing keys -- every string below is present and translated. They are wrong or
// inconsistent inside their own language, which no structural checker can see: check_i18n.py
// compares key sets and placeholders, and both sides already match.
//
// Every rule is an exact substring replacement that MUST match at least once. If a value was
// edited in the meantime the rule stops matching and the script exits non-zero instead of
// quietly doing nothing -- the failure mode that makes codemods untrustworthy.
//
// Usage:
//   node scripts/i18n-wording-fixes.mjs
//   node scripts/i18n-wording-fixes.mjs --check   # verify only, exit 1 if a fix is still pending

import { readFileSync, writeFileSync, existsSync } from "node:fs"
import { dirname, join } from "node:path"
import { fileURLToPath } from "node:url"

const ROOT = join(dirname(fileURLToPath(import.meta.url)), "..")
const LOCALES = join(ROOT, "web", "src", "locales")

// locale -> rules. A rule with `keys` is limited to those exact keys, which matters when the same
// word is correct elsewhere in the file. Without `keys` the rule applies to every value.
const RULES = {
	"uk-UA": [
		// Plain typo: a missing letter in "\u0437\u0430\u043f\u043b\u0430\u043d\u043e\u0432\u0430\u043d\u043e".
		{ from: "\u0437\u0430\u043f\u043b\u0430\u043d\u0440\u043e\u0432\u0430\u043d\u043e", to: "\u0437\u0430\u043f\u043b\u0430\u043d\u043e\u0432\u0430\u043d\u043e", why: "typo" },
	],
	"be-BY": [
		// Russian infinitive ending -\u0432\u0430\u0442\u044c; Belarusian is -\u0432\u0430\u0446\u044c. Three keys carry it.
		{ from: "\u0410\u0440\u0445\u0456\u0432\u0430\u0432\u0430\u0442\u044c", to: "\u0410\u0440\u0445\u0456\u0432\u0430\u0432\u0430\u0446\u044c", why: "Russian ending -\u0432\u0430\u0442\u044c" },
		// \u043c\u0435\u0441\u0446\u0430 is neuter in Belarusian, so the possessive has to agree.
		{ from: "\u0412\u0430\u0448\u0435 \u043c\u0435\u0441\u0446\u0430", to: "\u0412\u0430\u0448\u0430 \u043c\u0435\u0441\u0446\u0430", why: "gender agreement" },
		// Russian \u0437\u0430\u043c\u0435\u0442\u043a\u0430 left untranslated; Belarusian is \u043d\u0430\u0442\u0430\u0442\u043a\u0430.
		{ from: "\u0437\u0430\u043c\u0435\u0442\u043a\u0430", to: "\u043d\u0430\u0442\u0430\u0442\u043a\u0430", why: "Russian word" },
		{ from: "\u0417\u0430\u043c\u0435\u0442\u043a\u0430", to: "\u041d\u0430\u0442\u0430\u0442\u043a\u0430", why: "Russian word", optional: true },
		// Two spellings of "forever" in one file; \u043d\u0430\u0437\u0430\u045e\u0441\u0451\u0434\u044b is the one used in the delete dialogs.
		{ from: "\u043d\u0430\u0437\u0430\u045e\u0436\u0434\u044b", to: "\u043d\u0430\u0437\u0430\u045e\u0441\u0451\u0434\u044b", why: "inconsistent spelling" },
	],
	"tr-TR": [
		// "alan ad\u0131" is the standard Turkish term for a *domain name*, so the placeholder read
		// "New domain name" on the space creation field.
		{ from: "Yeni alan ad\u0131", to: "Yeni alan\u0131n ad\u0131", why: "reads as 'domain name'" },
	],
	"ko-KR": [
		// The file calls the entity \uc2a4\ud398\uc774\uc2a4 everywhere except the delete dialog, which switches to \uacf5\uac04.
		{ from: "\uacf5\uac04", to: "\uc2a4\ud398\uc774\uc2a4", why: "two names for one entity" },
		// workload.open is the "open" bucket, not "in progress" -- the latter is a different status.
		{ keys: ["workload.open"], from: "\uc9c4\ud589 \uc911", to: "\uc5f4\ub9bc", why: "wrong bucket name" },
	],
	"hi-IN": [
		// \u0938\u094d\u0925\u093e\u0928 means "place/location"; the product entity is \u0938\u094d\u092a\u0947\u0938 in the rest of the file.
		{ from: "\u0938\u094d\u0925\u093e\u0928", to: "\u0938\u094d\u092a\u0947\u0938", why: "two names for one entity" },
	],
	"zh-CN": [
		// \u60a8 (formal) and \u4f60 (plain) were mixed inside the same screens. The file is mostly \u4f60.
		{ from: "\u60a8", to: "\u4f60", why: "mixed formality" },
		// ASCII quotes inherited from en-US; Chinese typography uses \u300c\u300d.
		{ from: '"{name}"', to: "\u300c{name}\u300d", why: "straight quotes" },
	],
	"es-ES": [
		// "Todos" agrees with a masculine noun; both of these label lists of feminine nouns
		// (tareas / listas), so they need "Todas".
		{ keys: ["my.sub.all", "filters.all"], from: "Todos", to: "Todas", why: "gender agreement" },
	],
	"pt-BR": [
		{ keys: ["my.sub.all", "filters.all"], from: "Todos", to: "Todas", why: "gender agreement" },
	],
}

function serialise(obj) {
	const keys = Object.keys(obj).sort()
	const body = keys
		.map((key) => `  ${JSON.stringify(key)}: ${JSON.stringify(obj[key])}`)
		.join(",\n")
	return `{\n${body}\n}\n`
}

const check = process.argv.includes("--check")
let problems = 0
let pending = 0

for (const [locale, rules] of Object.entries(RULES)) {
	const file = join(LOCALES, `${locale}.json`)
	if (!existsSync(file)) {
		console.error(`${locale}: ${file} is missing`)
		problems++
		continue
	}
	const before = readFileSync(file, "utf8")
	const bundle = JSON.parse(before)

	for (const rule of rules) {
		const scope = rule.keys ? rule.keys : Object.keys(bundle)
		let hits = 0
		for (const key of scope) {
			const value = bundle[key]
			if (typeof value !== "string" || !value.includes(rule.from)) continue
			hits += value.split(rule.from).length - 1
			bundle[key] = value.split(rule.from).join(rule.to)
		}
		if (hits === 0) {
			// Already applied, or the value moved. `optional` marks a rule that only exists to catch
			// a capitalised variant which may legitimately be absent.
			const message = `${locale}: no match for ${JSON.stringify(rule.from)} (${rule.why})`
			if (rule.optional) console.log(`${message} -- optional, skipped`)
			else {
				console.error(`${message} -- already fixed, or the string changed; review the rule`)
				problems++
			}
			continue
		}
		console.log(`${locale}: ${hits}x ${JSON.stringify(rule.from)} -> ${JSON.stringify(rule.to)} (${rule.why})`)
	}

	const after = serialise(bundle)
	if (after === before) continue
	pending++
	if (check) {
		console.error(`${locale}: fixes still pending`)
		problems++
	} else {
		writeFileSync(file, after)
	}
}

if (problems > 0) {
	console.error(check ? "wording fixes: pending" : "wording fixes: failed")
	process.exit(1)
}
console.log(
	pending === 0 ? "wording fixes: nothing to do" : `wording fixes: ${pending} locale file(s) rewritten`,
)
console.log("next: python3 scripts/check_i18n.py && cd web && npm run build")

// Left alone on purpose, because they need a native speaker rather than a codemod:
//   kk-KZ  my.sub.mentions / profile.type.comment use "\u0410\u0442\u0430\u0443\u043b\u0430\u0440" (titles) for "mentions".
//   pt-BR  announcements are called comunicado, An\u00fancio and Avisos in three different keys.
//   bn-BD  numbers are spelled with Bengali digits in the locale but rendered with Western digits
//          by the code, so one screen shows both.
//   ru-RU  skeleton.hello is a two-sentence paragraph where all twelve other locales have one
//          short sentence.
//   task.quickadd_hint keeps the English word "tomorrow" in eight locales and translates it in the
//          Slavic ones. One group is lying to the user; which one depends on what the quick-add
//          parser in views.tsx actually accepts, and I have not read it.
