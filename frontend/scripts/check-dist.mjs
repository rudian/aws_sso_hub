// Fails when a production bundle contains the in-page mock layer, its fixtures or its URL switches.
// Usage: node scripts/check-dist.mjs [distDir]   (default: dist)
import { readdirSync, readFileSync, existsSync } from 'node:fs'
import { join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

// Strings that only exist in the mock layer, its fixtures and the mock-mode URL parameter parser.
export const FORBIDDEN = [
  { label: 'mock layer marker', test: (s) => s.includes('awsssohub-mock-layer') },
  { label: '?mock switch', test: (s) => /\.has\(\s*["'`]mock["'`]\s*\)/.test(s) },
  { label: 'mock URL parameter parsing', test: (s) => /\.get\(\s*["'`](mockLogin|scenario|addPortalStep|onboardingStep|addPath)["'`]\s*\)/.test(s) },
  { label: 'fixture repository names', test: (s) => /legacy-api|ambiguous_api|github_site/.test(s) },
  { label: 'fixture home folder', test: (s) => s.includes('/Users/alex') },
]

/** Returns the list of problems found in the .js files under <dist>/assets (empty when clean). */
export function checkDist(dist) {
  const assets = join(dist, 'assets')
  if (!existsSync(assets)) return [`${assets} does not exist; run npm run build first`]
  const files = readdirSync(assets).filter((f) => f.endsWith('.js'))
  if (files.length === 0) return [`no .js files in ${assets}`]
  const problems = []
  for (const f of files) {
    const text = readFileSync(join(assets, f), 'utf8')
    for (const rule of FORBIDDEN) if (rule.test(text)) problems.push(`${f}: contains ${rule.label}`)
  }
  return problems
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const dist = resolve(process.argv[2] ?? 'dist')
  const problems = checkDist(dist)
  if (problems.length > 0) {
    console.error('check-dist failed:\n' + problems.map((p) => '  ' + p).join('\n'))
    process.exit(1)
  }
  console.log(`check-dist ok: no mock layer in ${dist}/assets`)
}
