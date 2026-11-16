import { ChevronRight } from 'lucide-react'
import { useEffect, useMemo, useRef, useState } from 'react'
import { Identity } from '../../api'
import { ErrorCode } from '../../api/models'
import type { Author, GlobalIdentity, GlobalIdentityPreview } from '../../api/models'
import { toUiError } from '../../api/errors'
import { Button, DiffView, Field, Input, Modal } from '../../components'
import { useI18n } from '../../i18n'
import { fieldError, sameAuthor, validateAuthor } from './identityModel'
import { useActions } from '../../shell/AppActions'
import { useIdentityToast } from './identityHooks'

/** The ladder: repository, project, global. The most specific one wins. */
export function Ladder() {
  const { t } = useI18n()
  return (
    <div className="stack" style={{ '--gap': '8px', alignItems: 'flex-end' } as React.CSSProperties}>
      <ol className="ladder" aria-label={t('identity.ladder.label')}>
        <li>{t('identity.ladder.repo')}</li>
        <ChevronRight aria-hidden="true" />
        <li>{t('identity.ladder.project')}</li>
        <ChevronRight aria-hidden="true" />
        <li>{t('identity.ladder.global')}</li>
      </ol>
      <span className="field__hint">{t('identity.ladder.rule')}</span>
    </div>
  )
}

/** The global author identity card: name, email, Save (enabled on change), the ladder (design 5.12). */
export function GlobalIdentityCard({ global }: { global: GlobalIdentity }) {
  const { t } = useI18n()
  const { done, report } = useIdentityToast()
  const { toast } = useActions()
  const saved = useMemo<Author>(() => ({ name: global.name, email: global.email }), [global.name, global.email])
  const [name, setName] = useState(saved.name)
  const [email, setEmail] = useState(saved.email)
  const [touched, setTouched] = useState({ name: false, email: false })
  const [busy, setBusy] = useState(false)
  const [review, setReview] = useState<GlobalIdentityPreview | null>(null)
  const titleRef = useRef<HTMLHeadingElement>(null)

  // A saved, undone or outside change replaces what is in the fields.
  useEffect(() => {
    setName(saved.name)
    setEmail(saved.email)
    setTouched({ name: false, email: false })
  }, [saved])

  const draft = { name, email }
  const dirty = !sameAuthor(draft, saved)
  const valid = validateAuthor(draft)
  const showName = fieldError(name, valid.name, touched.name)
  const showEmail = fieldError(email, valid.email, touched.email)

  const startSave = async () => {
    if (!valid.name || !valid.email) {
      setTouched({ name: true, email: true })
      return
    }
    setBusy(true)
    try {
      const pv = await Identity.PreviewGlobal({ name: name.trim(), email: email.trim() })
      if (pv.unchanged) {
        toast({ kind: 'info', title: t('identity.global.unchanged') })
        return
      }
      setReview(pv)
    } catch (e) {
      report(e)
    } finally {
      setBusy(false)
    }
  }

  const apply = async () => {
    if (!review) return
    setBusy(true)
    try {
      const res = await Identity.ApplyGlobal(review.after, review.preview.baseHash)
      setReview(null)
      done(t('identity.global.done'), res)
      // the modal returns focus to Save, which the reload disables: keep the person's place on the card
      window.setTimeout(() => titleRef.current?.focus(), 0)
    } catch (e) {
      report(e)
      if (toUiError(e).code === ErrorCode.ErrConfigChanged) {
        // ~/.gitconfig changed after the preview: show the diff of the file as it is now
        try {
          const pv = await Identity.PreviewGlobal(review.after)
          setReview(pv.unchanged ? null : pv)
        } catch {
          setReview(null)
        }
      }
    } finally {
      setBusy(false)
    }
  }

  return (
    <section className={global.set ? 'card' : 'card card--warn'} aria-labelledby="ident-global-title">
      <div className="card__body ident-hero">
        <form
          className="stack"
          style={{ '--gap': 'var(--sp-4)' } as React.CSSProperties}
          onSubmit={(e) => {
            e.preventDefault()
            if (dirty && !busy) void startSave()
          }}
        >
          <div>
            <h2 className="card__title" id="ident-global-title" ref={titleRef} tabIndex={-1}>
              {t('identity.global.title')}
            </h2>
            <div className="field__hint" style={{ marginTop: 2 }}>
              {global.set ? t(global.usedBy === 1 ? 'identity.global.hint.one' : 'identity.global.hint', { n: global.usedBy }) : t('identity.global.unset')}
            </div>
          </div>
          <div className="ident-fields">
            <Field label={t('identity.field.name')} error={showName ? t('identity.field.name.invalid') : undefined}>
              <Input value={name} autoComplete="off" spellCheck={false} onChange={(e) => setName(e.target.value)} onBlur={() => setTouched((x) => ({ ...x, name: true }))} />
            </Field>
            <Field label={t('identity.field.email')} error={showEmail ? t(email.trim() === '' ? 'identity.field.email.empty' : 'identity.field.email.invalid') : undefined}>
              <Input value={email} type="email" autoComplete="off" spellCheck={false} onChange={(e) => setEmail(e.target.value)} onBlur={() => setTouched((x) => ({ ...x, email: true }))} />
            </Field>
            <Button type="submit" variant={global.set ? 'secondary' : 'primary'} disabled={!dirty} loading={busy && !review}>
              {t('identity.global.save')}
            </Button>
          </div>
        </form>
        <Ladder />
      </div>

      {review && (
        <Modal
          open
          size="lg"
          title={t('identity.global.review.title')}
          subtitle={t('identity.global.review.body', { file: review.preview.file })}
          dismissible={!busy}
          onClose={() => setReview(null)}
          footer={
            <>
              <Button variant="secondary" disabled={busy} onClick={() => setReview(null)}>
                {t('common.cancel')}
              </Button>
              <Button variant="primary" loading={busy} onClick={() => void apply()}>
                {t('identity.global.review.action')}
              </Button>
            </>
          }
        >
          <DiffView preview={review.preview} />
        </Modal>
      )}
    </section>
  )
}
