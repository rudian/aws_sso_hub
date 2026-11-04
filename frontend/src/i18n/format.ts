// Minimal ICU-style formatter: {name} and {n, plural, one {...} other {...}} ('#' = the number, nested {x} allowed
// inside branches). zh templates have no plural. Unknown key handling lives in index.ts.

export type FormatParams = Record<string, string | number>

function matchBrace(s: string, open: number): number {
  let depth = 0
  for (let i = open; i < s.length; i++) {
    const c = s[i]
    if (c === '{') depth++
    else if (c === '}') {
      depth--
      if (depth === 0) return i
    }
  }
  return -1
}

function parseBranches(s: string): Map<string, string> {
  const out = new Map<string, string>()
  let i = 0
  while (i < s.length) {
    while (i < s.length && /\s/.test(s[i])) i++
    if (i >= s.length) break
    let j = i
    while (j < s.length && s[j] !== '{' && !/\s/.test(s[j])) j++
    const selector = s.slice(i, j)
    while (j < s.length && /\s/.test(s[j])) j++
    if (s[j] !== '{') break
    const end = matchBrace(s, j)
    if (end < 0) break
    out.set(selector, s.slice(j + 1, end))
    i = end + 1
  }
  return out
}

function render(tpl: string, params: FormatParams, rules: Intl.PluralRules, hash?: number): string {
  let out = ''
  let i = 0
  while (i < tpl.length) {
    const c = tpl[i]
    if (c === '{') {
      const end = matchBrace(tpl, i)
      if (end < 0) {
        out += tpl.slice(i)
        break
      }
      out += renderArg(tpl.slice(i + 1, end), params, rules)
      i = end + 1
    } else if (c === '#' && hash !== undefined) {
      out += String(hash)
      i++
    } else {
      out += c
      i++
    }
  }
  return out
}

function renderArg(inner: string, params: FormatParams, rules: Intl.PluralRules): string {
  const comma = inner.indexOf(',')
  if (comma < 0) {
    const name = inner.trim()
    const v = params[name]
    return v === undefined ? `{${name}}` : String(v)
  }
  const name = inner.slice(0, comma).trim()
  const rest = inner.slice(comma + 1)
  const comma2 = rest.indexOf(',')
  const type = (comma2 < 0 ? rest : rest.slice(0, comma2)).trim()
  if (type !== 'plural' || comma2 < 0) return `{${inner}}`
  const branches = parseBranches(rest.slice(comma2 + 1))
  const raw = params[name]
  const n = typeof raw === 'number' ? raw : Number(raw)
  if (!Number.isFinite(n)) return `{${name}}`
  const body = branches.get(`=${n}`) ?? branches.get(rules.select(n)) ?? branches.get('other') ?? ''
  return render(body, params, rules, n)
}

export function formatMessage(template: string, params: FormatParams | undefined, lang: string): string {
  if (!template.includes('{')) return template
  let rules: Intl.PluralRules
  try {
    rules = new Intl.PluralRules(lang)
  } catch {
    rules = new Intl.PluralRules('en')
  }
  return render(template, params ?? {}, rules)
}
