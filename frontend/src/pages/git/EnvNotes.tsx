import type { TerminalEnvOverride } from '../../api/models'
import { System } from '../../api'
import { Banner, Button } from '../../components'
import { useI18n } from '../../i18n'
import type { MessageKey } from '../../i18n'
import { useReportError } from '../repos/hooks'

const WINDOWS = typeof navigator !== 'undefined' && /Windows/i.test(navigator.userAgent)

/**
 * What the terminal environment does to identities (design 5.12): an active GIT_AUTHOR_* override is a warning
 * with "open that file"; GIT_COMMITTER_*, EMAIL and a redirected config are quieter notes; a failed probe is
 * explained in words.
 */
export function EnvNotes({ env }: { env: TerminalEnvOverride }) {
  const { t } = useI18n()
  const report = useReportError()
  const vars = env.vars ?? []
  const authorVars = vars.filter((v) => v.name === 'GIT_AUTHOR_NAME' || v.name === 'GIT_AUTHOR_EMAIL').map((v) => v.name)
  const committer = vars.filter((v) => v.name.startsWith('GIT_COMMITTER_')).map((v) => v.name)
  const files = env.files ?? []
  const open = (path: string) => System.RevealPath(path).catch(report)

  const probe = env.probeError ? ((env.probeExitCode ? 'identity.env.probe.failedCode' : `identity.env.probe.${env.probeError}`) as MessageKey) : null
  const notes: { key: string; text: string }[] = []
  if (env.committerVars && committer.length) notes.push({ key: 'committer', text: t('identity.env.committer', { vars: committer.join(', ') }) })
  if (env.fallbackEmail) notes.push({ key: 'fallback', text: t('identity.env.fallback') })
  if (env.configRedirected) notes.push({ key: 'redirected', text: t('identity.env.redirected') })
  if (probe) notes.push({ key: 'probe', text: t(probe, { code: env.probeExitCode ?? 0 }) })

  if (!env.active && notes.length === 0) return null
  return (
    <div className="stack" style={{ '--gap': 'var(--sp-2)' } as React.CSSProperties}>
      {env.active && (
        <Banner
          kind="warn"
          title={t('identity.env.title')}
          actions={files.map((f) => (
            <Button key={f.path} size="sm" onClick={() => void open(f.path)} aria-label={files.length > 1 ? t('identity.env.openNamed', { file: f.file }) : undefined}>
              {files.length > 1 ? f.file : t(WINDOWS ? 'identity.env.openWin' : 'identity.env.open')}
            </Button>
          ))}
        >
          {t('identity.env.body', { vars: (authorVars.length ? authorVars : vars.map((v) => v.name)).join(', ') })}
        </Banner>
      )}
      {notes.map((n) => (
        <div key={n.key} className="hint" role="status">
          {n.text}
        </div>
      ))}
    </div>
  )
}
