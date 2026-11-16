import clsx from 'clsx'
import { ArrowLeftRight, ArrowUpRight, Check, ChevronDown, Copy, Ellipsis, EyeOff, Eye, FolderGit2, FolderInput, FolderOpen, GitBranch, GitCompare, GripVertical, LogIn, Plug, Terminal, Trash2, Unlink, UserPen, X } from 'lucide-react'
import { memo } from 'react'
import type { KeyboardEvent, PointerEvent, ReactNode } from 'react'
import { BindingSource, OwnerStatus, ProfileKind, RemoteKind, SessionState, TestStatus } from '../../api/models'
import type { OwnerCandidate, Portal, Profile, Repo } from '../../api/models'
import { AcctChip, Badge, Button, Checkbox, Dot, IconButton, Menu, ProviderMark, Skeleton, Spinner, Tooltip } from '../../components'
import type { MenuItem } from '../../components/Menu'
import { RepoName } from '../../components/RepoName'
import { useI18n } from '../../i18n'
import { isMac } from '../../lib/platform'
import { formatRelative, formatRemaining } from '../../lib/time'
import { canBind, portalOf, rowKind, sessionNeedsLogin, splitMiddle, suggestedProfile, testText } from '../repos/model'
import type { RowKind } from '../repos/model'
import { MovedAsk } from './MovedAsk'
import { isCodeCommit, providerKind } from './model'

export interface ProjectRowCallbacks {
  select(id: string, checked: boolean): void
  /** Row click, Enter, or "Open". */
  open(repo: Repo): void
  /** `anchor` is the control that opened the popover (focus returns to it). */
  bind(repo: Repo, profile: string, options: string[], anchor?: HTMLElement): void
  choose(repo: Repo, initial?: string, anchor?: HTMLElement): void
  test(repo: Repo): void
  revealFolder(repo: Repo): void
  unbind(repo: Repo): void
  copyRemote(repo: Repo): void
  openTerminal(repo: Repo): void
  removeExtra(repo: Repo): void
  toHttps(repo: Repo): void
  compare(repo: Repo): void
  signIn(portal: string): void
  recheck(repo: Repo): void
  /** Open the "move to project" menu for this row. */
  move(repo: Repo): void
  hide(repo: Repo): void
  unhide(repo: Repo): void
  reattach(repo: Repo): void
  decline(repo: Repo): void
  /** The grip was pressed: start a pointer drag. */
  dragStart(repo: Repo, e: PointerEvent<HTMLElement>): void
  /** A key the row does not handle itself: roving grid navigation lives on the page. */
  rowKey(repo: Repo, e: KeyboardEvent<HTMLElement>): void
  focusRow(repo: Repo, sectionId: string): void
}

export interface ProjectRowProps {
  repo: Repo
  selected: boolean
  /** This row is the one tab stop of its table. */
  tabStop: boolean
  /** The hidden list: dimmed, only "Unhide". */
  hiddenList: boolean
  /** The section the row is drawn in (the key of its tab stop). */
  sectionId: string
  profiles: Profile[]
  portals: Portal[]
  now: number
  cliReady: boolean
  testing: boolean
  comparing: boolean
  flash: boolean
  dragging: boolean
  /** The moved-folder question is being answered. */
  asking: boolean
  cb: ProjectRowCallbacks
}

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

/** A status line with a dot, as the mockup draws "Connection OK / checked 2 min ago". */
function DotHint({ kind, title, detail, tone }: { kind: 'ok' | 'warn' | 'danger' | 'off'; title: ReactNode; detail?: ReactNode; tone?: 'warn' | 'danger' }) {
  const color = tone === 'warn' ? 'var(--warn)' : tone === 'danger' ? 'var(--danger)' : 'var(--text-2)'
  return (
    <div className="proj-status">
      <Dot kind={kind} />
      <span className="hint">
        <strong className="hint__label" style={{ color }}>
          {title}
        </strong>
        {detail && <span className="hint__detail">{detail}</span>}
      </span>
    </div>
  )
}

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

function ProviderIcon({ repo }: { repo: Repo }) {
  const kind = providerKind(repo)
  if (kind) return <ProviderMark provider={kind} />
  return (
    <span className="prov" aria-hidden="true">
      <FolderGit2 size={15} />
    </span>
  )
}

function RepoRowImpl({ repo, selected, tabStop, hiddenList, sectionId, profiles, portals, now, cliReady, testing, comparing, flash, dragging, asking, cb }: ProjectRowProps) {
  const { t } = useI18n()
  const cc = isCodeCommit(repo)
  const kind: RowKind = cc ? rowKind(repo) : 'none'
  const b = repo.binding
  const byName = new Map(profiles.map((p) => [p.name, p]))
  const boundProfile = b.profile ? byName.get(b.profile) : undefined
  const cliTip = cliReady ? undefined : t('banner.cliMissing')
  const suggestion = cc ? suggestedProfile(repo) : undefined
  const st = repo.status

  // ---------------------------------------------------------------- account cell
  const boundChip = b.profile ? (
    b.profileKnown ? <AcctChip slot={b.colorSlot} monogram={b.monogram} name={b.profile} accountId={b.accountId} /> : <AcctChip none name={b.profile} />
  ) : null
  let account: ReactNode
  if (!cc) {
    account = <span className="src">{t('projects.acct.machine')}</span>
  } else {
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
        account = boundChip
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
  }
  // The chip opens the bind popover (design 5.2); it stays a plain chip where binding does not apply.
  if (cc && canBind(repo) && kind !== 'checking' && kind !== 'ambiguous') {
    account = (
      <button type="button" className="proj-chip" data-grid-control="" aria-label={t('projects.acct.change', { name: repo.localName })} onClick={(e) => cb.choose(repo, b.profile, e.currentTarget)}>
        {account}
      </button>
    )
  }

  // ---------------------------------------------------------------- branch and sync cell
  let branch: ReactNode
  if (!st) {
    branch = <Skeleton variant="block" width={96} height={14} />
  } else {
    const sync: ReactNode[] = []
    if (st.ahead > 0) sync.push(<b key="up" className="up">↑{st.ahead}</b>)
    if (st.behind > 0) sync.push(<b key="dn">↓{st.behind}</b>)
    const parts: ReactNode[] = []
    if (sync.length > 0) parts.push(<span key="s">{sync.reduce<ReactNode[]>((acc, x, i) => (i ? [...acc, ' ', x] : [x]), [])}</span>)
    if (st.changes > 0) parts.push(<span key="c" className="chg">{t('projects.git.changed', { n: st.changes })}</span>)
    if (parts.length === 0) parts.push(<span key="clean">{t('projects.git.clean')}</span>)
    const name = st.detached ? t('projects.git.detached') : st.unborn ? t('projects.git.unborn') : st.branch
    branch = (
      <>
        <span className="branch" title={st.upstream ? `${st.branch} → ${st.upstream}` : name}>
          <GitBranch aria-hidden="true" />
          <span className="truncate">{name}</span>
        </span>
        <span className="sync">{parts.reduce<ReactNode[]>((acc, x, i) => (i ? [...acc, <span key={`d${i}`} aria-hidden="true">·</span>, x] : [x]), [])}</span>
      </>
    )
  }

  // ---------------------------------------------------------------- status cell
  const signInButton = (portal: string, again = false) => (
    <Button size="sm" variant="ghost" icon={LogIn} data-grid-control="" onClick={() => cb.signIn(portal)} style={{ marginTop: 2 }}>
      {again ? t('projects.action.signInAgain') : t('repos.action.signIn')}
    </Button>
  )
  let status: ReactNode = null
  if (testing) {
    status = (
      <div className="proj-status">
        <Spinner />
        <span className="hint">{t('repos.test.running')}</span>
      </div>
    )
  } else if (!cc) {
    if (st && st.conflicted > 0) status = <DotHint kind="danger" tone="danger" title={t('projects.status.conflicts', { n: st.conflicted })} />
    else if (st && st.behind > 0) status = <DotHint kind="ok" title={t('projects.status.behind', { n: st.behind })} />
    else if (!repo.remoteName && !repo.remoteUrl) status = <DotHint kind="off" title={t('projects.status.noRemote')} />
    else if (st) status = <DotHint kind="ok" title={t('projects.status.upToDate')} detail={t('projects.status.readAgo', { when: formatRelative(st.at, now, t) })} />
  } else {
    switch (kind) {
      case 'checking':
        status = (
          <div className="proj-status">
            <Spinner />
            <span className="hint">{t('repos.owner.checking')}</span>
          </div>
        )
        break
      case 'ssh':
        status = <Hint title={t('repos.status.ssh')}>{t('repos.status.ssh.hint')}</Hint>
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
          status = <DotHint kind="warn" tone="warn" title={t('repos.status.unknownProfile', { name: b.profile ?? '' })} />
        } else if (portal && sessionNeedsLogin(portal.session.state)) {
          status = (
            <div>
              <DotHint kind="warn" tone="warn" title={t('repos.status.expired')} />
              {signInButton(portal.name)}
            </div>
          )
        } else if (portal && portal.session.state === SessionState.SessionExpiring && portal.session.sessionEndsAt) {
          status = (
            <div>
              <DotHint kind="warn" tone="warn" title={t('projects.status.signInEnds', { when: formatRemaining(portal.session.sessionEndsAt, now, t) })} detail={t('projects.status.signInEnds.hint')} />
              {signInButton(portal.name, true)}
            </div>
          )
        } else if (boundProfile?.kind === ProfileKind.ProfileKindStatic) {
          status = <DotHint kind="warn" tone="warn" title={t('repos.status.staticKey')} detail={t('repos.status.staticKey.hint', { name: boundProfile.name })} />
        } else if (res) {
          const ok = res.status === TestStatus.TestOK
          status = (
            <div>
              <DotHint
                kind={ok ? 'ok' : 'danger'}
                tone={ok ? undefined : 'danger'}
                title={ok ? t('projects.status.ok') : t('repos.status.testFailed')}
                detail={ok ? t('projects.status.checkedAgo', { when: formatRelative(res.at, now, t) }) : testText(res, t)}
              />
              {res.status === TestStatus.TestSignedOut && res.portal && signInButton(res.portal)}
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
                <Button size="sm" variant="ghost" icon={LogIn} data-grid-control="" onClick={() => cb.recheck(repo)} style={{ marginTop: 2 }}>
                  {t('repos.owner.partial.action')}
                </Button>
              )
            }
          />
        )
        break
      }
      default: {
        const o = repo.owner
        if (o.status === OwnerStatus.OwnerSingle && o.candidates?.[0]) {
          status = (
            <DotHint kind="warn" title={t('repos.owner.single', { name: o.candidates[0].profile })} detail={t('repos.owner.checkedWith', { n: o.checkedWith })} />
          )
        }
      }
    }
  }

  // ---------------------------------------------------------------- actions cell
  const moreItems: MenuItem[] = []
  if (cc && canBind(repo) && kind !== 'checking') {
    moreItems.push({ id: 'change', label: t('repos.action.changeAccount'), icon: UserPen, disabled: !cliReady, onSelect: () => cb.choose(repo, b.profile) })
  }
  moreItems.unshift({ id: 'open', label: t('projects.action.open'), icon: ArrowUpRight, onSelect: () => cb.open(repo) })
  if (cc && b.source === BindingSource.BindingLocal && b.profileKnown) {
    moreItems.push({ id: 'test', label: t('repos.action.test'), icon: Plug, disabled: !cliReady || testing, onSelect: () => cb.test(repo) })
  }
  moreItems.push({ id: 'move', label: t('projects.action.move'), icon: FolderInput, onSelect: () => cb.move(repo) })
  moreItems.push({ id: 'reveal', label: isMac() ? t('projects.action.revealMac') : t('projects.action.revealWin'), icon: FolderOpen, separatorBefore: true, onSelect: () => cb.revealFolder(repo) })
  if (b.profile && b.profileKnown) moreItems.push({ id: 'terminal', label: t('repos.action.openTerminal'), icon: Terminal, onSelect: () => cb.openTerminal(repo) })
  if (repo.remoteUrl) moreItems.push({ id: 'copy', label: t('repos.action.copyRemote'), icon: Copy, onSelect: () => cb.copyRemote(repo) })
  if (cc && b.source === BindingSource.BindingLocal) moreItems.push({ id: 'unbind', label: t('repos.action.unbind'), icon: Unlink, danger: true, separatorBefore: true, onSelect: () => cb.unbind(repo) })
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
      separatorBefore: !(cc && b.source === BindingSource.BindingLocal),
      onSelect: () => cb.removeExtra(repo),
    })
  }
  moreItems.push({ id: 'hide', label: t('projects.action.hide'), icon: EyeOff, separatorBefore: true, onSelect: () => cb.hide(repo) })
  const more = <Menu align="end" trigger={<IconButton size="sm" label={t('repos.action.more')} icon={Ellipsis} data-grid-control="" />} items={moreItems} />

  const bindButton = (profile: string, variant: 'primary' | 'secondary' = 'primary', label?: string) => (
    <Tooltip content={cliTip}>
      <Button size="sm" variant={variant} disabled={!cliReady} data-grid-control="" onClick={(e) => cb.bind(repo, profile, [profile], e.currentTarget)}>
        {label ?? t('repos.action.bindTo', { name: profile })}
      </Button>
    </Tooltip>
  )
  const chooseButton = (label: string) => (
    <Tooltip content={cliTip}>
      <Button size="sm" variant="secondary" disabled={!cliReady} data-grid-control="" onClick={(e) => cb.choose(repo, undefined, e.currentTarget)}>
        {label}
      </Button>
    </Tooltip>
  )

  let primary: ReactNode = null
  if (cc) {
    switch (kind) {
      case 'bound':
        primary = null
        break
      case 'ssh':
        primary = (
          <Tooltip content={cliTip}>
            <Button size="sm" variant="secondary" icon={ArrowLeftRight} disabled={!cliReady} data-grid-control="" onClick={() => cb.toHttps(repo)}>
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
        const items: MenuItem[] = (repo.owner.candidates ?? []).map((c) => ({
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
              <Button size="sm" variant="secondary" data-owner-trigger="" data-grid-control="">
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
      case 'checking':
      case 'grc':
        primary = null
        break
      default:
        primary = suggestion ? bindButton(suggestion) : chooseButton(t('repos.action.chooseAccount'))
    }
  }

  const onRowClick = (e: React.MouseEvent<HTMLElement>) => {
    if (hiddenList) return
    if ((e.target as HTMLElement).closest('button, input, a, label, [role="menuitem"], [data-no-open]')) return
    cb.open(repo)
  }
  const onKeyDown = (e: KeyboardEvent<HTMLElement>) => cb.rowKey(repo, e)

  return (
    <div
      className={clsx('row proj-row', selected && 'is-selected', hiddenList && 'is-dim', dragging && 'is-dragging')}
      role="row"
      aria-selected={hiddenList ? undefined : selected}
      aria-label={repo.localName}
      data-repo-id={repo.id}
      data-flip-key={repo.id}
      tabIndex={tabStop ? 0 : -1}
      onClick={onRowClick}
      onKeyDown={onKeyDown}
      onFocus={(e) => {
        if (e.target === e.currentTarget) cb.focusRow(repo, sectionId)
      }}
      style={flash ? { background: 'color-mix(in srgb, var(--ok) 16%, var(--surface))' } : undefined}
    >
      <div role="gridcell" className="row__cell">
        {!hiddenList && <Checkbox checked={selected} aria-label={t('repos.select', { name: repo.localName })} data-grid-control="" onChange={(e) => cb.select(repo.id, e.target.checked)} />}
      </div>
      <div role="gridcell" className="row__cell repo-cell">
        {!hiddenList && (
          <span className="grip" data-no-open="" aria-hidden="true" onPointerDown={(e) => cb.dragStart(repo, e)}>
            <GripVertical size={14} />
          </span>
        )}
        <ProviderIcon repo={repo} />
        <span className="repo-cell__text">
          <span className="repo-cell__name">
            <RepoName localName={repo.localName} remoteName={repo.remoteName} className="row__primary truncate" />
          </span>
          <MiddleText className="row__secondary" value={repo.remoteKind === RemoteKind.RemoteSSH && cc ? repo.remoteUrl : repo.displayPath} />
        </span>
      </div>
      <div role="gridcell" className="row__cell proj-acct">
        {account}
      </div>
      <div role="gridcell" className="row__cell proj-branch">
        {branch}
      </div>
      <div role="gridcell" className="row__cell">
        {status}
      </div>
      <div role="gridcell" className="row__cell row__actions">
        {hiddenList ? (
          <Button size="sm" variant="secondary" icon={Eye} data-grid-control="" onClick={() => cb.unhide(repo)}>
            {t('projects.action.unhide')}
          </Button>
        ) : (
          <>
            {primary}
            {more}
          </>
        )}
      </div>
      {repo.movedFrom && !repo.projectId && !hiddenList && (
        <div role="gridcell" className="proj-row__ask">
          <MovedAsk name={repo.movedFrom.name} busy={asking} onYes={() => cb.reattach(repo)} onNo={() => cb.decline(repo)} />
        </div>
      )}
    </div>
  )
}

export const ProjectRow = memo(RepoRowImpl)
