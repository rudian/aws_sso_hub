import clsx from 'clsx'
import { ArrowLeftRight, Check, ChevronDown, Ellipsis, FolderOpen, GitCompare, LogIn, Plug, Terminal, Trash2, Unlink, X, Copy, UserPen } from 'lucide-react'
import { memo } from 'react'
import type { ReactNode } from 'react'
import { BindingSource, OwnerStatus, ProfileKind, RemoteKind, TestStatus } from '../../api/models'
import type { OwnerCandidate, Portal, Profile, Repo } from '../../api/models'
import { AcctChip, Badge, Button, Checkbox, Dot, IconButton, Menu, Skeleton, Spinner, Tooltip } from '../../components'
import type { MenuItem } from '../../components/Menu'
import { useI18n } from '../../i18n'
import { formatRelative } from '../../lib/time'
import { canBind, canSelect, portalOf, rowKind, sessionNeedsLogin, splitMiddle, suggestedProfile, testText } from './model'
import type { RowKind } from './model'

export interface RowCallbacks {
  select(id: string, checked: boolean): void
  /** Open the bind popover. `options` are the profiles the user may pick from. */
  bind(repo: Repo, profile: string, options: string[]): void
  /** Open the bind popover with every profile to choose from. */
  choose(repo: Repo, initial?: string): void
  test(repo: Repo): void
  openFolder(repo: Repo): void
  unbind(repo: Repo): void
  copyRemote(repo: Repo): void
  openTerminal(repo: Repo): void
  removeExtra(repo: Repo): void
  toHttps(repo: Repo): void
  compare(repo: Repo): void
  signIn(portal: string): void
  recheck(repo: Repo): void
}

export interface RepoRowProps {
  repo: Repo
  selected: boolean
  profiles: Profile[]
  portals: Portal[]
  now: number
  cliReady: boolean
  testing: boolean
  comparing: boolean
  flash: boolean
  cb: RowCallbacks
}

/** Status text: a bold first line, an optional detail line (dropped in narrow windows) and optional actions that always stay. */
function Hint({ tone, title, children, action }: { tone?: 'warn' | 'danger' | 'ok'; title?: ReactNode; children?: ReactNode; action?: ReactNode }) {
  const color = tone === 'warn' ? 'var(--warn)' : tone === 'danger' ? 'var(--danger)' : undefined
  return (
    <div className="hint">
      {title && (
        <strong className="hint__label" style={color ? { color } : undefined}>
          {title}
        </strong>
      )}
      {children && <span className={title ? 'hint__detail' : undefined}>{children}</span>}
      {action}
    </div>
  )
}

/** A path or remote with the middle ellipsised and the full value in the tooltip. */
function MiddleText({ value, className }: { value: string; className?: string }) {
  const { head, tail } = splitMiddle(value)
  return (
    <span className={clsx('midtrunc', className)} title={value}>
      {head && <span className="midtrunc__head">{head}</span>}
      <span className="midtrunc__tail">{tail}</span>
    </span>
  )
}

function candidateProfiles(c: OwnerCandidate): string[] {
  const all = c.profiles ?? []
  return all.length > 0 ? all : [c.profile]
}

function RepoRowImpl({ repo, selected, profiles, portals, now, cliReady, testing, comparing, flash, cb }: RepoRowProps) {
  const { t } = useI18n()
  const kind: RowKind = rowKind(repo)
  const b = repo.binding
  const byName = new Map(profiles.map((p) => [p.name, p]))
  const boundProfile = b.profile ? byName.get(b.profile) : undefined
  const cliTip = cliReady ? undefined : t('banner.cliMissing')
  const suggestion = suggestedProfile(repo)
  const selectable = canSelect(repo)

  // ---------------------------------------------------------------- account cell
  const boundChip = b.profile ? (
    b.profileKnown ? <AcctChip slot={b.colorSlot} monogram={b.monogram} name={b.profile} accountId={b.accountId} /> : <AcctChip none name={b.profile} />
  ) : null
  let account: ReactNode
  switch (kind) {
    case 'checking':
      account = <Skeleton variant="block" width={120} height={24} />
      break
    case 'ssh':
      account = <AcctChip none dashed name={t('repos.chip.cantBind')} />
      break
    case 'ambiguous':
      account = (
        <>
          <div className="owner-pick">
            {(repo.owner.candidates ?? []).map((c) => (
              <AcctChip key={c.accountId} slot={c.colorSlot} monogram={c.monogram} name={c.profile} accountId={c.accountId} dashed />
            ))}
          </div>
          <span className="src">{t('repos.owner.candidates')}</span>
        </>
      )
      break
    case 'grc':
      account = (
        <>
          {boundChip ?? <AcctChip none name={t('repos.chip.unbound')} />}
          <span className="src">{t('repos.src.grc')}</span>
        </>
      )
      break
    case 'bound':
    case 'repair':
    case 'legacy':
      account = (
        <>
          {boundChip}
          <span className="src">{t('repos.src.local')}</span>
        </>
      )
      break
    case 'url-rule':
      account = (
        <>
          {boundChip}
          <span className="src">{t('repos.src.globalUrl')}</span>
        </>
      )
      break
    default: {
      // global default / guard / none, also the rows whose owner could not be found
      const src =
        b.source === BindingSource.BindingGlobalDefault ? t('repos.src.globalDefault', { name: b.profile ?? '' }) : b.source === BindingSource.BindingGuard ? t('repos.src.guard') : t('repos.src.none')
      account = (
        <>
          <AcctChip none dashed name={t('repos.chip.unbound')} />
          <span className="src">{src}</span>
        </>
      )
    }
  }

  // ---------------------------------------------------------------- status cell
  const signInButton = (portal: string) => (
    <Button size="sm" variant="ghost" icon={LogIn} onClick={() => cb.signIn(portal)} style={{ marginTop: 2 }}>
      {t('repos.action.signIn')}
    </Button>
  )
  let status: ReactNode = null
  if (testing) {
    status = (
      <div className="hint" style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
        <Spinner />
        <span>{t('repos.test.running')}</span>
      </div>
    )
  } else {
    switch (kind) {
      case 'checking':
        status = (
          <div className="hint" style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
            <Spinner />
            <span>{t('repos.owner.checking')}</span>
          </div>
        )
        break
      case 'ssh':
        status = (
          <Hint title={t('repos.status.ssh')}>{t('repos.status.ssh.hint')}</Hint>
        )
        break
      case 'grc':
        status = <Hint>{t('repos.status.grc')}</Hint>
        break
      case 'repair':
        status = (
          <Hint tone="warn" title={t('repos.status.repair')}>
            {t('repos.status.repair.hint')}
          </Hint>
        )
        break
      case 'legacy':
        status = (
          <Hint tone="warn" title={t('repos.status.legacy')}>
            {t('repos.status.legacy.hint')}
          </Hint>
        )
        break
      case 'bound': {
        const portal = portalOf(boundProfile, portals)
        const res = repo.lastTest
        if (!b.profileKnown) {
          status = (
            <Hint tone="warn" title={t('repos.status.unknownProfile', { name: b.profile ?? '' })} />
          )
        } else if (portal && sessionNeedsLogin(portal.session.state)) {
          status = (
            <Hint tone="warn" title={t('repos.status.expired')} action={signInButton(portal.name)} />
          )
        } else if (boundProfile?.kind === ProfileKind.ProfileKindStatic) {
          status = (
            <Hint tone="warn" title={t('repos.status.staticKey')}>
              {t('repos.status.staticKey.hint', { name: boundProfile.name })}
            </Hint>
          )
        } else if (res) {
          const ok = res.status === TestStatus.TestOK
          status = (
            <div className="hint" style={{ display: 'flex', gap: 8, alignItems: 'flex-start' }}>
              <Dot kind={ok ? 'ok' : 'danger'} className="" />
              <span>
                <strong className="hint__label" style={ok ? undefined : { color: 'var(--danger)' }}>
                  {ok ? t('repos.status.ok') : t('repos.status.testFailed')}
                </strong>
                <span className="hint__detail">{ok ? t('repos.status.testedAgo', { when: formatRelative(res.at, now, t) }) : testText(res, t)}</span>
                {res.status === TestStatus.TestSignedOut && res.portal && <div>{signInButton(res.portal)}</div>}
              </span>
            </div>
          )
        }
        break
      }
      case 'url-rule':
        status = (
          <Hint tone="warn" title={t('repos.status.urlRule')}>
            {t('repos.status.urlRule.hint')}
          </Hint>
        )
        break
      case 'ambiguous': {
        const head = (repo.owner.candidates ?? []).find((c) => c.headMatch)
        status = (
          <Hint
            tone="warn"
            title={t('repos.owner.ambiguous')}
            action={
              <>
                {comparing && (
                  <div style={{ display: 'flex', gap: 6, alignItems: 'center', marginTop: 2 }}>
                    <Spinner />
                    {t('repos.owner.comparing')}
                  </div>
                )}
                {head && (
                  <div style={{ marginTop: 2, color: 'var(--ok)' }}>
                    <Check size={12} aria-hidden="true" style={{ verticalAlign: -2 }} /> {head.profile}: {t('repos.owner.headMatch')}
                  </div>
                )}
              </>
            }
          >
            {t('repos.owner.ambiguous.hint')}
          </Hint>
        )
        break
      }
      case 'not-found':
        status = (
          <Hint tone="danger" title={t('repos.owner.none')}>
            {t('repos.owner.none.hint')}
          </Hint>
        )
        break
      case 'partial': {
        const portalsToSign = repo.owner.skippedPortals ?? []
        status = (
          <Hint
            tone="warn"
            title={t('repos.owner.partial', { n: repo.owner.skipped })}
            action={
              portalsToSign.length > 0 && (
                <Button size="sm" variant="ghost" icon={LogIn} onClick={() => cb.recheck(repo)} style={{ marginTop: 2 }}>
                  {t('repos.owner.partial.action')}
                </Button>
              )
            }
          />
        )
        break
      }
      default: {
        // global default / guard / none: what owner detection found
        const o = repo.owner
        if (o.status === OwnerStatus.OwnerSingle && o.candidates?.[0]) {
          status = (
            <Hint title={t('repos.owner.single', { name: o.candidates[0].profile })}>{t('repos.owner.checkedWith', { n: o.checkedWith })}</Hint>
          )
        }
      }
    }
  }

  // ---------------------------------------------------------------- actions cell
  const moreItems: MenuItem[] = []
  if (canBind(repo) && kind !== 'checking') {
    moreItems.push({ id: 'change', label: t('repos.action.changeAccount'), icon: UserPen, disabled: !cliReady, onSelect: () => cb.choose(repo, b.profile) })
  }
  moreItems.push({ id: 'folder', label: t('repos.action.openFolder'), icon: FolderOpen, onSelect: () => cb.openFolder(repo) })
  moreItems.push({ id: 'copy', label: t('repos.action.copyRemote'), icon: Copy, onSelect: () => cb.copyRemote(repo) })
  if (b.profile && b.profileKnown) moreItems.push({ id: 'terminal', label: t('repos.action.openTerminal'), icon: Terminal, onSelect: () => cb.openTerminal(repo) })
  if (b.source === BindingSource.BindingLocal) moreItems.push({ id: 'unbind', label: t('repos.action.unbind'), icon: Unlink, danger: true, separatorBefore: true, onSelect: () => cb.unbind(repo) })
  if (repo.extra) {
    moreItems.push({
      id: 'remove',
      label: (
        <span>
          {t('repos.removeExtra')}
          <span className="muted" style={{ display: 'block', fontSize: 'var(--fs-12)', maxWidth: 240, whiteSpace: 'normal' }}>
            {t('repos.removeExtra.note')}
          </span>
        </span>
      ),
      icon: Trash2,
      separatorBefore: b.source !== BindingSource.BindingLocal,
      onSelect: () => cb.removeExtra(repo),
    })
  }
  const more = <Menu align="end" trigger={<IconButton size="sm" label={t('repos.action.more')} icon={Ellipsis} />} items={moreItems} />

  const bindButton = (profile: string, variant: 'primary' | 'secondary' = 'primary', label?: string) => (
    <Tooltip content={cliTip}>
      <Button size="sm" variant={variant} disabled={!cliReady} onClick={() => cb.bind(repo, profile, [profile])}>
        {label ?? t('repos.action.bindTo', { name: profile })}
      </Button>
    </Tooltip>
  )
  const chooseButton = (label: string) => (
    <Tooltip content={cliTip}>
      <Button size="sm" variant="secondary" disabled={!cliReady} onClick={() => cb.choose(repo)}>
        {label}
      </Button>
    </Tooltip>
  )
  const iconTest = (
    <IconButton size="sm" label={cliReady ? t('repos.action.test') : t('banner.cliMissing')} icon={Plug} disabled={!cliReady || testing} onClick={() => cb.test(repo)} />
  )
  const iconFolder = <IconButton size="sm" label={t('repos.action.openFolder')} icon={FolderOpen} onClick={() => cb.openFolder(repo)} />

  let primary: ReactNode = null
  switch (kind) {
    case 'bound':
      primary = (
        <>
          {iconTest}
          {iconFolder}
        </>
      )
      break
    case 'grc':
      primary = iconFolder
      break
    case 'checking':
      primary = null
      break
    case 'ssh':
      primary = (
        <Tooltip content={cliTip}>
          <Button size="sm" variant="secondary" icon={ArrowLeftRight} disabled={!cliReady} onClick={() => cb.toHttps(repo)}>
            {t('repos.action.toHttps')}
          </Button>
        </Tooltip>
      )
      break
    case 'url-rule':
      primary = b.profile && b.profileKnown ? bindButton(b.profile, 'secondary', t('repos.action.toLocal')) : chooseButton(t('repos.action.chooseAccount'))
      break
    case 'repair':
    case 'legacy':
      primary = b.profile && b.profileKnown ? bindButton(b.profile) : chooseButton(t('repos.action.chooseAccount'))
      break
    case 'ambiguous': {
      const cands = repo.owner.candidates ?? []
      const items: MenuItem[] = cands.map((c) => ({
        id: `cand-${c.accountId}`,
        disabled: !cliReady,
        label: (
          <span className="cluster" style={{ gap: 8, flexWrap: 'nowrap' }}>
            <AcctChip slot={c.colorSlot} monogram={c.monogram} name={c.profile} />
            <span className="muted mono" style={{ fontSize: 'var(--fs-12)' }}>
              {c.accountId}
            </span>
            {c.headMatch && (
              <Badge kind="ok" icon={Check}>
                {t('repos.owner.headMatch')}
              </Badge>
            )}
          </span>
        ),
        onSelect: () => cb.bind(repo, c.profile, candidateProfiles(c)),
      }))
      items.push({ id: 'compare', label: t('repos.owner.compare'), icon: GitCompare, separatorBefore: true, disabled: !cliReady || comparing, onSelect: () => cb.compare(repo) })
      items.push({ id: 'neither', label: t('repos.owner.neither'), icon: X, disabled: !cliReady, onSelect: () => cb.choose(repo) })
      primary = (
        <Menu
          align="end"
          items={items}
          trigger={
            <Button size="sm" variant="secondary" data-owner-trigger="">
              {t('repos.action.chooseAccount')}
              <ChevronDown className="btn__icon" aria-hidden="true" />
            </Button>
          }
        />
      )
      break
    }
    case 'not-found':
      primary = chooseButton(t('repos.action.chooseManual'))
      break
    case 'partial':
      primary = chooseButton(t('repos.action.chooseAccount'))
      break
    default:
      primary = suggestion ? bindButton(suggestion) : chooseButton(t('repos.action.chooseAccount'))
  }

  // Rows that only offer icon buttons keep Test and Open folder visible; the others fold them into the menu.
  return (
    <div
      className={clsx('row', selected && 'is-selected')}
      role="row"
      data-repo-id={repo.id}
      style={flash ? { background: 'color-mix(in srgb, var(--ok) 16%, var(--surface))' } : undefined}
    >
      <Checkbox checked={selected} disabled={!selectable} aria-label={t('repos.select', { name: repo.name })} onChange={(e) => cb.select(repo.id, e.target.checked)} />
      <div className="row__cell repo-name">
        <span className="row__primary truncate" style={{ maxWidth: '100%' }}>
          {repo.name}
        </span>
        <MiddleText className="row__secondary" value={repo.remoteKind === RemoteKind.RemoteSSH ? repo.remoteUrl : repo.displayPath} />
      </div>
      <div className="row__cell" style={{ flexDirection: 'column', alignItems: 'flex-start', gap: 4, minWidth: 0 }}>
        {account}
      </div>
      <div className="row__cell">{status}</div>
      <div className="row__cell row__actions" data-repo-actions={repo.id} style={{ gap: 4 }}>
        {primary}
        {more}
      </div>
    </div>
  )
}

export const RepoRow = memo(RepoRowImpl)
