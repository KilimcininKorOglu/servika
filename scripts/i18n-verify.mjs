// i18n completeness verifier — checks every language directory under
// frontend/src/lib/locales/ against en/ (the source of truth).
//
//   node scripts/i18n-verify.mjs            # verify every language dir
//   node scripts/i18n-verify.mjs de fr      # verify only the named languages
//
// Three deterministic checks per language, per namespace:
//   1. namespace parity  — every en/<NS>.json exists in the language dir
//   2. key parity        — same key set as en, after collapsing i18next plural
//                          suffixes to their base (a language needs only the
//                          plural categories its CLDR rules define; _other is
//                          the one universal category, so it is the minimum)
//   3. placeholder parity — the SET of {{tokens}} used across the whole
//                          namespace matches en (aggregated per namespace, so a
//                          placeholder may legitimately move between split
//                          segments like countPre/countPost when word order
//                          differs, but a dropped or misspelled token is caught)
//
// Exits 1 on any problem so it can gate a commit / CI.

import { readFileSync, readdirSync, existsSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, join } from 'node:path'

const HERE = dirname(fileURLToPath(import.meta.url))
const LOCALES = join(HERE, '..', 'frontend', 'src', 'lib', 'locales')
const SOURCE = 'en'
const PLURAL_SUFFIXES = ['_zero', '_one', '_two', '_few', '_many', '_other']

// Flatten a nested object into dot-path → string-value pairs (only leaf strings).
function flatten(obj, prefix, out) {
  for (const k of Object.keys(obj)) {
    const v = obj[k]
    const path = prefix ? `${prefix}.${k}` : k
    if (v && typeof v === 'object' && !Array.isArray(v)) flatten(v, path, out)
    else out[path] = v
  }
  return out
}

// Collapse an i18next plural key to its base: "footer_one" → "footer|plural".
function baseKey(key) {
  for (const s of PLURAL_SUFFIXES) {
    if (key.endsWith(s)) return key.slice(0, -s.length) + '|plural'
  }
  return key
}

// All {{tokens}} in a string (ignores {{- raw}} prefix and formatting suffixes).
function tokensIn(value) {
  if (typeof value !== 'string') return []
  const found = []
  const re = /\{\{-?\s*([\w.]+)/g
  let m
  while ((m = re.exec(value)) !== null) found.push(m[1])
  return found
}

function namespaceTokenSet(flat) {
  const set = new Set()
  for (const key of Object.keys(flat)) for (const tok of tokensIn(flat[key])) set.add(tok)
  return set
}

function readJSON(path) {
  return JSON.parse(readFileSync(path, 'utf8'))
}

const sourceDir = join(LOCALES, SOURCE)
const sourceNamespaces = readdirSync(sourceDir).filter((f) => f.endsWith('.json'))

const requested = process.argv.slice(2)
const allLangs = readdirSync(LOCALES).filter(
  (d) => d !== SOURCE && existsSync(join(LOCALES, d)) && readdirSync(join(LOCALES, d)).length >= 0,
)
const langs = requested.length ? requested : allLangs

let problems = 0
const problem = (msg) => {
  console.error(`✗ ${msg}`)
  problems++
}

// Precompute source: per-namespace base-key set and token set.
const src = {}
for (const ns of sourceNamespaces) {
  const flat = flatten(readJSON(join(sourceDir, ns)), '', {})
  src[ns] = {
    bases: new Set(Object.keys(flat).map(baseKey)),
    tokens: namespaceTokenSet(flat),
    // plural bases: which collapsed bases came from a plural key (need _other).
    pluralBases: new Set(
      Object.keys(flat).filter((k) => baseKey(k).endsWith('|plural')).map(baseKey),
    ),
  }
}

for (const lang of langs) {
  const dir = join(LOCALES, lang)
  if (!existsSync(dir)) {
    problem(`${lang}: language directory does not exist`)
    continue
  }
  for (const ns of sourceNamespaces) {
    const nsPath = join(dir, ns)
    if (!existsSync(nsPath)) {
      problem(`${lang}/${ns}: missing namespace file`)
      continue
    }
    let flat
    try {
      flat = flatten(readJSON(nsPath), '', {})
    } catch (e) {
      problem(`${lang}/${ns}: invalid JSON — ${e.message}`)
      continue
    }
    const langBases = new Set(Object.keys(flat).map(baseKey))
    const langKeys = new Set(Object.keys(flat))

    // key parity (base level)
    const missing = [...src[ns].bases].filter((b) => !langBases.has(b))
    const extra = [...langBases].filter((b) => !src[ns].bases.has(b))
    if (missing.length) problem(`${lang}/${ns}: missing keys — ${missing.join(', ')}`)
    if (extra.length) problem(`${lang}/${ns}: unexpected keys — ${extra.join(', ')}`)

    // every plural base needs at least the universal _other category
    for (const pb of src[ns].pluralBases) {
      if (!langBases.has(pb)) continue // already reported as missing
      const stem = pb.slice(0, -'|plural'.length)
      if (!langKeys.has(`${stem}_other`)) {
        problem(`${lang}/${ns}: plural key "${stem}" is missing the required _other form`)
      }
    }

    // placeholder parity (namespace-aggregated set)
    const langTokens = namespaceTokenSet(flat)
    const missTok = [...src[ns].tokens].filter((t) => !langTokens.has(t))
    const extraTok = [...langTokens].filter((t) => !src[ns].tokens.has(t))
    if (missTok.length) problem(`${lang}/${ns}: missing placeholders — {{${missTok.join('}}, {{')}}}`)
    if (extraTok.length) problem(`${lang}/${ns}: unexpected placeholders — {{${extraTok.join('}}, {{')}}}`)
  }
}

if (problems === 0) {
  console.log(`✓ OK — ${langs.length} language(s) match en (${sourceNamespaces.length} namespaces each).`)
  process.exit(0)
}
console.error(`\nFAILED — ${problems} problem(s).`)
process.exit(1)
