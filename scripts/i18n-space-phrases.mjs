#!/usr/bin/env node
// Normalises the dynamic phrases of the Spaces screen across every locale.
//
// House rule for these phrases: two parts, separated by a sentence
// terminator -- "First part. Second part".
//   part 1 -- the current state, or what is about to happen
//   part 2 -- what to do next, or what happens next
//
// Keys covered: spaces.empty, spaces.lists_empty, spaces.archive_confirm.
// Before this script only en-US and ru-RU carried them at all. The other
// eleven base locales resolved through the [locale, lang, "en-US"] chain in
// web/src/i18n.ts and therefore printed English inside an otherwise
// translated screen -- including the archive confirmation dialog.
//
// Usage:
//   node scripts/i18n-space-phrases.mjs           # rewrite the locale files
//   node scripts/i18n-space-phrases.mjs --check   # verify only, exit 1 on drift

import { readFileSync, writeFileSync, existsSync } from "node:fs"
import { dirname, join } from "node:path"
import { fileURLToPath } from "node:url"

const ROOT = join(dirname(fileURLToPath(import.meta.url)), "..")
const LOCALES = join(ROOT, "web", "src", "locales")

// Slang overlays inherit from their base locale, so they are only touched when
// they already carry an own value for one of these keys.
const OVERLAYS = new Set(["ru-RU-it", "en-US-it"])

const PHRASES = {
	"en-US": {
		"spaces.empty": "No spaces yet. Create your first one below.",
		"spaces.lists_empty": "No lists yet. Create your first one below.",
		"spaces.archive_confirm":
			"Archive \u201c{name}\u201d? You can restore it from the archive later.",
	},
	"ru-RU": {
		"spaces.empty": "\u041f\u0440\u043e\u0441\u0442\u0440\u0430\u043d\u0441\u0442\u0432 \u043f\u043e\u043a\u0430 \u043d\u0435\u0442. \u0421\u043e\u0437\u0434\u0430\u0439\u0442\u0435 \u043f\u0435\u0440\u0432\u043e\u0435 \u043d\u0438\u0436\u0435.",
		"spaces.lists_empty": "\u0421\u043f\u0438\u0441\u043a\u043e\u0432 \u043f\u043e\u043a\u0430 \u043d\u0435\u0442. \u0421\u043e\u0437\u0434\u0430\u0439\u0442\u0435 \u043f\u0435\u0440\u0432\u044b\u0439 \u043d\u0438\u0436\u0435.",
		"spaces.archive_confirm": "\u0410\u0440\u0445\u0438\u0432\u0438\u0440\u043e\u0432\u0430\u0442\u044c \u00ab{name}\u00bb? \u041f\u043e\u0437\u0436\u0435 \u0435\u0433\u043e \u043c\u043e\u0436\u043d\u043e \u0432\u0435\u0440\u043d\u0443\u0442\u044c \u0438\u0437 \u0430\u0440\u0445\u0438\u0432\u0430.",
	},
	"uk-UA": {
		"spaces.empty": "\u041f\u0440\u043e\u0441\u0442\u043e\u0440\u0456\u0432 \u0449\u0435 \u043d\u0435\u043c\u0430\u0454. \u0421\u0442\u0432\u043e\u0440\u0456\u0442\u044c \u043f\u0435\u0440\u0448\u0438\u0439 \u043d\u0438\u0436\u0447\u0435.",
		"spaces.lists_empty": "\u0421\u043f\u0438\u0441\u043a\u0456\u0432 \u0449\u0435 \u043d\u0435\u043c\u0430\u0454. \u0421\u0442\u0432\u043e\u0440\u0456\u0442\u044c \u043f\u0435\u0440\u0448\u0438\u0439 \u043d\u0438\u0436\u0447\u0435.",
		"spaces.archive_confirm": "\u0410\u0440\u0445\u0456\u0432\u0443\u0432\u0430\u0442\u0438 \u00ab{name}\u00bb? \u041f\u043e\u0442\u0456\u043c \u0439\u043e\u0433\u043e \u043c\u043e\u0436\u043d\u0430 \u0432\u0456\u0434\u043d\u043e\u0432\u0438\u0442\u0438 \u0437 \u0430\u0440\u0445\u0456\u0432\u0443.",
	},
	"be-BY": {
		"spaces.empty": "\u041f\u0440\u0430\u0441\u0442\u043e\u0440 \u043f\u0430\u043a\u0443\u043b\u044c \u043d\u044f\u043c\u0430. \u0421\u0442\u0432\u0430\u0440\u044b\u0446\u0435 \u043f\u0435\u0440\u0448\u0443\u044e \u043d\u0456\u0436\u044d\u0439.",
		"spaces.lists_empty": "\u0421\u043f\u0456\u0441\u0430\u045e \u043f\u0430\u043a\u0443\u043b\u044c \u043d\u044f\u043c\u0430. \u0421\u0442\u0432\u0430\u0440\u044b\u0446\u0435 \u043f\u0435\u0440\u0448\u044b \u043d\u0456\u0436\u044d\u0439.",
		"spaces.archive_confirm": "\u0410\u0440\u0445\u0456\u0432\u0430\u0432\u0430\u0446\u044c \u00ab{name}\u00bb? \u041f\u0430\u0437\u043d\u0435\u0439 \u044f\u0435 \u043c\u043e\u0436\u043d\u0430 \u0432\u044f\u0440\u043d\u0443\u0446\u044c \u0437 \u0430\u0440\u0445\u0456\u0432\u0430.",
	},
	"kk-KZ": {
		"spaces.empty": "\u041a\u0435\u04a3\u0456\u0441\u0442\u0456\u043a\u0442\u0435\u0440 \u04d9\u0437\u0456\u0440\u0433\u0435 \u0436\u043e\u049b. \u0422\u04e9\u043c\u0435\u043d\u0434\u0435 \u0431\u0456\u0440\u0456\u043d\u0448\u0456\u0441\u0456\u043d \u0436\u0430\u0441\u0430\u04a3\u044b\u0437.",
		"spaces.lists_empty": "\u0422\u0456\u0437\u0456\u043c\u0434\u0435\u0440 \u04d9\u0437\u0456\u0440\u0433\u0435 \u0436\u043e\u049b. \u0422\u04e9\u043c\u0435\u043d\u0434\u0435 \u0431\u0456\u0440\u0456\u043d\u0448\u0456\u0441\u0456\u043d \u0436\u0430\u0441\u0430\u04a3\u044b\u0437.",
		"spaces.archive_confirm": "\u00ab{name}\u00bb \u043c\u04b1\u0440\u0430\u0493\u0430\u0442\u0442\u0430\u043b\u0441\u044b\u043d \u0431\u0430? \u041a\u0435\u0439\u0456\u043d \u043e\u043d\u044b \u043c\u04b1\u0440\u0430\u0493\u0430\u0442\u0442\u0430\u043d \u049b\u0430\u0439\u0442\u0430\u0440\u0443\u0493\u0430 \u0431\u043e\u043b\u0430\u0434\u044b.",
	},
	"es-ES": {
		"spaces.empty": "A\u00fan no hay espacios. Crea el primero abajo.",
		"spaces.lists_empty": "A\u00fan no hay listas. Crea la primera abajo.",
		"spaces.archive_confirm":
			"\u00bfArchivar \u201c{name}\u201d? Podr\u00e1s restaurarlo desde el archivo m\u00e1s tarde.",
	},
	"pt-BR": {
		"spaces.empty": "Ainda n\u00e3o h\u00e1 espa\u00e7os. Crie o primeiro abaixo.",
		"spaces.lists_empty": "Ainda n\u00e3o h\u00e1 listas. Crie a primeira abaixo.",
		"spaces.archive_confirm":
			"Arquivar \u201c{name}\u201d? Voc\u00ea pode restaur\u00e1-lo do arquivo depois.",
	},
	"tr-TR": {
		"spaces.empty": "Hen\u00fcz alan yok. \u0130lkini a\u015fa\u011f\u0131da olu\u015fturun.",
		"spaces.lists_empty": "Hen\u00fcz liste yok. \u0130lkini a\u015fa\u011f\u0131da olu\u015fturun.",
		"spaces.archive_confirm":
			"\u201c{name}\u201d ar\u015fivlensin mi? Daha sonra ar\u015fivden geri y\u00fckleyebilirsiniz.",
	},
	"zh-CN": {
		"spaces.empty": "\u8fd8\u6ca1\u6709\u7a7a\u95f4\u3002\u5728\u4e0b\u9762\u521b\u5efa\u7b2c\u4e00\u4e2a\u3002",
		"spaces.lists_empty": "\u8fd8\u6ca1\u6709\u6e05\u5355\u3002\u5728\u4e0b\u9762\u521b\u5efa\u7b2c\u4e00\u4e2a\u3002",
		"spaces.archive_confirm": "\u8981\u5f52\u6863\u300c{name}\u300d\u5417\uff1f\u4e4b\u540e\u53ef\u4ee5\u4ece\u5f52\u6863\u4e2d\u6062\u590d\u3002",
	},
	"ja-JP": {
		"spaces.empty": "\u30b9\u30da\u30fc\u30b9\u304c\u307e\u3060\u3042\u308a\u307e\u305b\u3093\u3002\u4e0b\u3067\u6700\u521d\u306e\u30b9\u30da\u30fc\u30b9\u3092\u4f5c\u6210\u3057\u3066\u304f\u3060\u3055\u3044\u3002",
		"spaces.lists_empty": "\u30ea\u30b9\u30c8\u304c\u307e\u3060\u3042\u308a\u307e\u305b\u3093\u3002\u4e0b\u3067\u6700\u521d\u306e\u30ea\u30b9\u30c8\u3092\u4f5c\u6210\u3057\u3066\u304f\u3060\u3055\u3044\u3002",
		"spaces.archive_confirm": "\u300c{name}\u300d\u3092\u30a2\u30fc\u30ab\u30a4\u30d6\u3057\u307e\u3059\u304b\uff1f\u5f8c\u3067\u30a2\u30fc\u30ab\u30a4\u30d6\u304b\u3089\u5fa9\u5143\u3067\u304d\u307e\u3059\u3002",
	},
	"ko-KR": {
		"spaces.empty": "\uc544\uc9c1 \uc2a4\ud398\uc774\uc2a4\uac00 \uc5c6\uc2b5\ub2c8\ub2e4. \uc544\ub798\uc5d0\uc11c \uccab \uc2a4\ud398\uc774\uc2a4\ub97c \ub9cc\ub4dc\uc138\uc694.",
		"spaces.lists_empty": "\uc544\uc9c1 \ub9ac\uc2a4\ud2b8\uac00 \uc5c6\uc2b5\ub2c8\ub2e4. \uc544\ub798\uc5d0\uc11c \uccab \ub9ac\uc2a4\ud2b8\ub97c \ub9cc\ub4dc\uc138\uc694.",
		"spaces.archive_confirm": "\u201c{name}\u201d\uc744 \ubcf4\uad00\ud558\uc2ed\uc2dc\uac00\uc694? \ub098\uc911\uc5d0 \ubcf4\uad00\ud568\uc5d0\uc11c \ubcf5\uc6d0\ud560 \uc218 \uc788\uc2b5\ub2c8\ub2e4.",
	},
	"hi-IN": {
		"spaces.empty": "\u0905\u092d\u0940 \u0915\u094b\u0908 \u0938\u094d\u092a\u0947\u0938 \u0928\u0939\u0940\u0902 \u0939\u0948\u0964 \u0928\u0940\u091a\u0947 \u092a\u0939\u0932\u093e \u0938\u094d\u092a\u0947\u0938 \u092c\u0928\u093e\u0910\u0901\u0964",
		"spaces.lists_empty": "\u0905\u092d\u0940 \u0915\u094b\u0908 \u0938\u0942\u091a\u0940 \u0928\u0939\u0940\u0902 \u0939\u0948\u0964 \u0928\u0940\u091a\u0947 \u092a\u0939\u0932\u0940 \u0938\u0942\u091a\u0940 \u092c\u0928\u093e\u0910\u0901\u0964",
		"spaces.archive_confirm": "\u201c{name}\u201d \u0938\u0902\u0917\u094d\u0930\u0939\u093f\u0924 \u0915\u0930\u0947\u0902? \u092c\u093e\u0926 \u092e\u0947\u0902 \u0938\u0902\u0917\u094d\u0930\u0939 \u0938\u0947 \u092a\u0941\u0928\u0930\u094d\u0938\u094d\u0925\u093e\u092a\u093f\u0924 \u0915\u093f\u092f\u093e \u091c\u093e \u0938\u0915\u0924\u093e \u0939\u0948\u0964",
	},
	"bn-BD": {
		"spaces.empty": "\u098f\u0996\u09a8\u09cb \u0995\u09cb\u09a8\u09cb \u09b8\u09cd\u09aa\u09c7\u09b8 \u09a8\u09c7\u0987\u0964 \u09a8\u09bf\u099a\u09c7 \u09aa\u09cd\u09b0\u09a5\u09ae\u099f\u09bf \u09a4\u09c8\u09b0\u09bf \u0995\u09b0\u09c1\u09a8\u0964",
		"spaces.lists_empty": "\u098f\u0996\u09a8\u09cb \u0995\u09cb\u09a8\u09cb \u09a4\u09be\u09b2\u09bf\u0995\u09be \u09a8\u09c7\u0987\u0964 \u09a8\u09bf\u099a\u09c7 \u09aa\u09cd\u09b0\u09a5\u09ae\u099f\u09bf \u09a4\u09c8\u09b0\u09bf \u0995\u09b0\u09c1\u09a8\u0964",
		"spaces.archive_confirm": "\u201c{name}\u201d \u0986\u09b0\u09cd\u0995\u09be\u0987\u09ad \u0995\u09b0\u09ac\u09c7\u09a8? \u09aa\u09b0\u09c7 \u0986\u09b0\u09cd\u0995\u09be\u0987\u09ad \u09a5\u09c7\u0995\u09c7 \u09ab\u09bf\u09b0\u09bf\u09df\u09c7 \u0986\u09a8\u09be \u09af\u09be\u09ac\u09c7\u0964",
	},
}

// Sentence terminators used by the locales above: Latin/Cyrillic, CJK, and the
// Devanagari danda.
const TERMINATORS = new Set([".", "!", "?", "\u3002", "\uff01", "\uff1f", "\u0964"])

function splitParts(value) {
	const parts = []
	let buf = ""
	for (const ch of value) {
		buf += ch
		if (TERMINATORS.has(ch)) {
			parts.push(buf.trim())
			buf = ""
		}
	}
	if (buf.trim()) parts.push(buf.trim())
	return parts.filter(Boolean)
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
let changed = 0

// Every phrase must obey the two-part rule before anything is written.
for (const [locale, phrases] of Object.entries(PHRASES)) {
	for (const [key, value] of Object.entries(phrases)) {
		const parts = splitParts(value)
		if (parts.length < 2) {
			console.error(
				`${locale} ${key}: not a two-part phrase, got ${parts.length} part(s): ${value}`,
			)
			problems++
		}
		if (value.includes("{name}") === false && key.endsWith("archive_confirm")) {
			console.error(`${locale} ${key}: lost the {name} placeholder`)
			problems++
		}
	}
}
if (problems > 0) {
	console.error("refusing to touch the locales: fix the table above first")
	process.exit(1)
}

for (const [locale, phrases] of Object.entries(PHRASES)) {
	const file = join(LOCALES, `${locale}.json`)
	if (!existsSync(file)) {
		console.error(`${locale}: ${file} is missing`)
		problems++
		continue
	}
	const before = readFileSync(file, "utf8")
	const bundle = JSON.parse(before)
	const added = []
	const updated = []
	for (const [key, value] of Object.entries(phrases)) {
		if (!(key in bundle)) added.push(key)
		else if (bundle[key] !== value) updated.push(key)
		bundle[key] = value
	}
	const after = serialise(bundle)
	if (after === before) {
		console.log(`${locale}: ok`)
		continue
	}
	changed++
	const note = [
		added.length ? `added ${added.length}` : null,
		updated.length ? `updated ${updated.length}` : null,
	]
		.filter(Boolean)
		.join(", ")
	if (check) {
		console.error(`${locale}: needs rewrite (${note || "reformatted"})`)
		problems++
	} else {
		writeFileSync(file, after)
		console.log(`${locale}: ${note || "reformatted"}`)
	}
}

// Overlays inherit unless they already override the key themselves.
for (const overlay of OVERLAYS) {
	const file = join(LOCALES, `${overlay}.json`)
	if (!existsSync(file)) continue
	const bundle = JSON.parse(readFileSync(file, "utf8"))
	const own = Object.keys(PHRASES["en-US"]).filter((key) => key in bundle)
	if (own.length === 0) {
		console.log(`${overlay}: inherits, left alone`)
		continue
	}
	console.log(
		`${overlay}: overrides ${own.join(", ")} -- rewrite by hand, the slang wording is not mine to guess`,
	)
}

if (problems > 0) {
	console.error(
		check ? "i18n space phrases: drift found" : "i18n space phrases: failed",
	)
	process.exit(1)
}
console.log(
	changed === 0
		? "i18n space phrases: nothing to do"
		: `i18n space phrases: ${changed} locale file(s) ${check ? "need work" : "rewritten"}`,
)
console.log("next: cd web && npx tsc --noEmit && npm run build")
