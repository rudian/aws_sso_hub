import { Banner } from '../components/Banner'
import { Button } from '../components/Button'
import { ToolStatus } from '../api/models'
import { Repos, System } from '../api/services'
import { toUiError } from '../api/errors'
import { errorText, useI18n } from '../i18n'
import { useAccounts } from '../state/accounts'
import { useEnv } from '../state/env'
import { useExternalChanges } from '../state/external'
import { useRepos } from '../state/repos'
import { useActions } from './AppActions'

export type BannerChoice = 'configParse' | 'cli' | 'globalDefault' | 'cliOutdated' | 'external' | 'repair'

export interface BannerFacts {
  /** The AWS config cannot be parsed (Accounts only). */
  configParse?: boolean
  /** AWS CLI missing, v1 or too old. */
  cliBlocking?: boolean
  /** Unbound repositories fall back to the global Git default (Repositories only). */
  globalDefault?: boolean
  cliOutdated?: boolean
  external?: boolean
  repair?: boolean
}

/**
 * One banner at most, by priority: config parse error, CLI missing/v1/too old, global Git default,
 * CLI outdated, external file change, bindings that need repair. Health lists the rest.
 */
export function pickBanner(f: BannerFacts): BannerChoice | null {
  if (f.configParse) return 'configParse'
  if (f.cliBlocking) return 'cli'
  if (f.globalDefault) return 'globalDefault'
  if (f.cliOutdated) return 'cliOutdated'
  if (f.external) return 'external'
  if (f.repair) return 'repair'
  return null
}

export interface GlobalBannerProps {
  /** Which page-specific conditions this page may promote. */
  scope?: 'accounts' | 'repos'
}

/** Every page renders this directly under its .page__header. At most one banner, chosen by {@link pickBanner}. */
export function GlobalBanner({ scope }: GlobalBannerProps = {}) {
  const { t } = useI18n()
  const { report } = useEnv()
  const { navigate, toast } = useActions()
  const external = useExternalChanges()
  const accounts = useAccounts()
  const repos = useRepos()

  const cli = report?.awsCli
  const min = cli?.min ?? '2.22.0'
  const cliBlocking = !!cli && (cli.status === ToolStatus.ToolMissing || cli.status === ToolStatus.ToolV1 || cli.status === ToolStatus.ToolTooOld)
  const issues = accounts.state?.issues ?? []
  const globalDefault = repos.state?.globalDefault

  const choice = pickBanner({
    configParse: scope === 'accounts' && !!accounts.state?.readOnly,
    cliBlocking,
    globalDefault: scope === 'repos' && !!globalDefault,
    cliOutdated: !!cli && cli.status === ToolStatus.ToolOutdated,
    external: external.kinds.length > 0,
    repair: (repos.state?.needsRepair ?? 0) > 0,
  })

  let banner: React.ReactNode = null
  if (choice === 'configParse') {
    banner = (
      <Banner
        kind="danger"
        title={t('accounts.readOnly.title')}
        actions={
          <Button
            size="sm"
            onClick={() =>
              void System.OpenAWSConfig().catch((e) => {
                const text = errorText(toUiError(e), t)
                toast({ kind: 'danger', title: text.title, body: text.body })
              })
            }
          >
            {t('common.openFile')}
          </Button>
        }
      >
        {issues.length > 0 && (
          <>
            <div>{t('error.configParse', { file: issues[0].file, line: issues[0].line })}</div>
            {issues.some((i) => i.message) && (
              <details className="banner__details">
                <summary>{t('common.details')}</summary>
                <div className="mono">
                  {issues.map((i, k) => (
                    <div key={k}>
                      {i.file}:{i.line} {i.message}
                    </div>
                  ))}
                </div>
              </details>
            )}
          </>
        )}
      </Banner>
    )
  } else if (choice === 'cli' && cli) {
    const text = cli.status === ToolStatus.ToolMissing ? t('banner.cliMissing') : cli.status === ToolStatus.ToolV1 ? t('banner.cliV1') : t('banner.cliTooOld', { version: cli.version ?? '', min: cli.min ?? '2.9.0' })
    banner = (
      <Banner kind="danger" actions={<Button size="sm" onClick={() => navigate({ page: 'tutorial', section: 'faq' })}>{t('banner.cliMissing.action')}</Button>}>
        {text}
      </Banner>
    )
  } else if (choice === 'globalDefault' && globalDefault) {
    banner = (
      <Banner
        kind="warn"
        title={t('repos.banner.globalDefault.title', { name: globalDefault.profile })}
        actions={
          <Button size="sm" variant="secondary" onClick={() => navigate({ page: 'git' })}>
            {t('repos.banner.globalDefault.action')}
          </Button>
        }
      >
        {t('repos.banner.globalDefault.body', { name: globalDefault.profile })}
      </Banner>
    )
  } else if (choice === 'cliOutdated' && cli) {
    banner = <Banner kind="warn">{t('banner.cliOld', { version: cli.version ?? '', min })}</Banner>
  } else if (choice === 'external') {
    const kind = external.kinds[0]
    const text = kind === 'aws-credentials' ? t('banner.configChanged.credentials') : kind === 'git-global' ? t('banner.configChanged.git') : t('banner.configChanged')
    banner = (
      <Banner
        kind="info"
        actions={
          <Button
            size="sm"
            onClick={() => {
              if (kind === 'git-global') void repos.reload()
              else void accounts.reload()
              external.dismiss(kind)
            }}
          >
            {t('banner.configChanged.action')}
          </Button>
        }
      >
        {text}
      </Banner>
    )
  } else if (choice === 'repair') {
    banner = (
      <Banner
        kind="warn"
        actions={
          <Button
            size="sm"
            onClick={async () => {
              try {
                await Repos.RepairBindings()
              } catch (e) {
                const text = errorText(toUiError(e), t)
                toast({ kind: 'danger', title: text.title, body: text.body })
              }
            }}
          >
            {t('banner.repair.action')}
          </Button>
        }
      >
        {t('banner.repair', { n: repos.state?.needsRepair ?? 0 })}
      </Banner>
    )
  }
  if (!banner) return null
  return <div className="global-banner">{banner}</div>
}
