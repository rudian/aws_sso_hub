import clsx from 'clsx'
import { Check, ChevronRight, ShieldCheck, Users } from 'lucide-react'
import { useEffect, useMemo } from 'react'
import { toUiError } from '../../api/errors'
import { Badge } from '../../components/Badge'
import { Banner } from '../../components/Banner'
import { Checkbox } from '../../components/Checkbox'
import { CodeBlock } from '../../components/CodeBlock'
import { Progress } from '../../components/Progress'
import { Segmented } from '../../components/Segmented'
import { errorText, makeT, useI18n } from '../../i18n'
import type { Lang } from '../../i18n'
import { useNavigate, useRoute } from '../../lib/route'
import type { TutorialSectionId } from '../../lib/route'
import { useActions } from '../../shell/AppActions'
import { GlobalBanner } from '../../shell/GlobalBanner'
import { useSettings } from '../../state/settings'
import { SECTION_ORDER, tutorialFor } from './content'
import type { TutorialSection } from './content'
import { Inline, Paragraphs } from './Inline'
import './tutorial.css'

function tutorialLang(setting: string | undefined, ui: Lang): Lang {
  return setting === 'zh-CN' || setting === 'en' ? setting : ui
}

function Toc({ sections, current, done, onPick, t }: { sections: TutorialSection[]; current: TutorialSectionId; done: Set<string>; onPick: (id: TutorialSectionId) => void; t: ReturnType<typeof makeT> }) {
  const { t: ui } = useI18n()
  const groups: Array<{ id: 'admin' | 'dev'; label: string }> = [
    { id: 'admin', label: t('tut.group.admin') },
    { id: 'dev', label: t('tut.group.dev') },
  ]
  const doneCount = sections.filter((s) => done.has(s.id)).length
  return (
    <aside className="toc" aria-label={ui('tut.toc.aria')}>
      <div className="toc__progress">
        <div className="toc__progress-row">
          <span>{t('tut.progress')}</span>
          <b className="num">
            {doneCount} / {sections.length}
          </b>
        </div>
        <Progress pct={(doneCount / sections.length) * 100} label={t('tut.progress')} />
      </div>
      {groups.map((g) => (
        <div key={g.id}>
          <div className="toc__group">{g.label}</div>
          <ol className="toc__list">
            {sections.map((s, i) =>
              s.group !== g.id ? null : (
                <li key={s.id}>
                  <button
                    type="button"
                    className={clsx('toc__item', s.id === current && 'is-active', done.has(s.id) && 'is-done')}
                    aria-current={s.id === current ? 'step' : undefined}
                    onClick={() => onPick(s.id)}
                  >
                    <span className="toc__n" aria-hidden="true">
                      {done.has(s.id) ? <Check /> : i + 1}
                    </span>
                    <span>
                      {s.title}
                      {done.has(s.id) && <span className="sr-only"> ({t('tut.sectionDone')})</span>}
                    </span>
                  </button>
                </li>
              ),
            )}
          </ol>
        </div>
      ))}
    </aside>
  )
}

export function TutorialPage() {
  const { t: ui, lang } = useI18n()
  const { settings, update } = useSettings()
  const route = useRoute()
  const navigate = useNavigate()
  const { toast } = useActions()

  const tutLang = tutorialLang(settings?.tutorialLang, lang)
  const t = useMemo(() => makeT(tutLang), [tutLang])
  const sections = useMemo(() => tutorialFor(tutLang), [tutLang])
  const done = useMemo(() => new Set(settings?.tutorialDone ?? []), [settings?.tutorialDone])

  const currentId: TutorialSectionId = route.section && SECTION_ORDER.includes(route.section) ? route.section : 'why'
  const index = Math.max(0, SECTION_ORDER.indexOf(currentId))
  const section = sections[index]
  const prev = index > 0 ? sections[index - 1] : null
  const next = index < sections.length - 1 ? sections[index + 1] : null
  const isDone = done.has(section.id)

  useEffect(() => {
    document.getElementById('main')?.scrollTo?.({ top: 0 })
  }, [currentId])

  const go = (id: TutorialSectionId) => navigate({ page: 'tutorial', section: id })
  const save = async (patch: Parameters<typeof update>[0]) => {
    try {
      await update(patch)
    } catch (e) {
      const text = errorText(toUiError(e), ui)
      toast({ kind: 'danger', title: ui('tut.saveFailed'), body: text.title })
    }
  }
  const toggleDone = (checked: boolean) => {
    const set = new Set(done)
    if (checked) set.add(section.id)
    else set.delete(section.id)
    void save({ tutorialDone: SECTION_ORDER.filter((id) => set.has(id)) })
  }

  return (
    <div className="page">
      <header className="page__header">
        <div>
          <h1 className="page__title">{ui('tut.title')}</h1>
          <p className="page__subtitle">{ui('tut.subtitle')}</p>
        </div>
        <div className="page__actions">
          <Segmented<Lang>
            role="radiogroup"
            ariaLabel={ui('tut.lang.aria')}
            value={tutLang}
            onChange={(v) => void save({ tutorialLang: v })}
            items={[
              { value: 'zh-CN', label: ui('tut.lang.zh') },
              { value: 'en', label: ui('tut.lang.en') },
            ]}
          />
        </div>
      </header>
      <GlobalBanner />
      <div className="tut" lang={tutLang}>
        <Toc sections={sections} current={currentId} done={done} onPick={go} t={t} />
        <article className="article" aria-labelledby="tut-title">
          <div className="article__kicker">
            <Badge kind="neutral" icon={section.group === 'admin' ? ShieldCheck : Users}>
              {section.group === 'admin' ? t('tut.badge.admin') : t('tut.badge.dev')}
            </Badge>
            {t('tut.time', { n: section.minutes })}
          </div>
          <h2 className="article__title" id="tut-title">
            {section.title}
          </h2>
          <p className="article__lead">
            <Inline text={section.lead} />
          </p>

          {section.steps.length > 0 && (
            <ol className="steps">
              {section.steps.map((st, i) => (
                <li key={i} className={isDone ? 'done' : undefined}>
                  <div>
                    <div className="step__title">{st.title}</div>
                    {st.crumbs && st.crumbs.length > 0 && (
                      <p className="step__text step__crumbs">
                        <span className="crumbs">
                          {st.crumbs.map((c, j) => (
                            <span key={j} style={{ display: 'contents' }}>
                              {j > 0 && <ChevronRight aria-hidden="true" />}
                              <span>{c}</span>
                            </span>
                          ))}
                        </span>
                      </p>
                    )}
                    <Paragraphs text={st.text} className="step__text" />
                    {st.code && <CodeBlock className="step__code" label={st.code.label} code={st.code.text} />}
                  </div>
                </li>
              ))}
            </ol>
          )}

          {section.faq && section.faq.length > 0 && (
            <div className="faq">
              {section.faq.map((f, i) => (
                <details className="faq__item" key={i}>
                  <summary className="faq__q">
                    <ChevronRight aria-hidden="true" />
                    <span>
                      <Inline text={f.q} />
                    </span>
                  </summary>
                  <div className="faq__a">
                    <Paragraphs text={f.a} />
                  </div>
                </details>
              ))}
            </div>
          )}

          {section.callouts && section.callouts.length > 0 && (
            <div className="article__callouts">
              {section.callouts.map((c, i) => (
                <Banner key={i} kind={c.kind === 'warn' ? 'warn' : 'info'} title={c.title}>
                  <div className="callout__body">
                    <Paragraphs text={c.body} />
                  </div>
                </Banner>
              ))}
            </div>
          )}

          <div className="article__done">
            <Checkbox label={t('tut.markDone')} checked={isDone} disabled={!settings} onChange={(e) => toggleDone(e.target.checked)} />
          </div>

          <nav className="article__nav" aria-label={ui('tut.nav.aria')}>
            {prev ? (
              <button type="button" className="navcard" onClick={() => go(prev.id)}>
                <small>{t('tut.prev')}</small>
                <b>{prev.title}</b>
              </button>
            ) : (
              <span />
            )}
            {next && (
              <button type="button" className="navcard navcard--next" onClick={() => go(next.id)}>
                <small>{t('tut.next')}</small>
                <b>{next.title}</b>
              </button>
            )}
          </nav>
        </article>
      </div>
    </div>
  )
}
