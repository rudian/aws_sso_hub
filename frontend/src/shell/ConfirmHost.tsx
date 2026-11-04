import { useRef } from 'react'
import { Button } from '../components/Button'
import { Modal } from '../components/Modal'
import { useI18n } from '../i18n'
import type { ConfirmInput } from './AppActions'

export interface PendingConfirm {
  input: ConfirmInput
  resolve: (ok: boolean) => void
}

/** `.modal--sm`; focus starts on Cancel; the primary button repeats the verb. */
export function ConfirmHost({ pending }: { pending: PendingConfirm | null }) {
  const { t } = useI18n()
  const cancelRef = useRef<HTMLButtonElement>(null)
  if (!pending) return null
  const { input, resolve } = pending
  return (
    <Modal
      open
      size="sm"
      title={input.title}
      onClose={() => resolve(false)}
      initialFocus={cancelRef}
      footer={
        <>
          <Button ref={cancelRef} variant="secondary" onClick={() => resolve(false)}>
            {input.cancelLabel ?? t('common.cancel')}
          </Button>
          <Button variant={input.danger ? 'danger' : 'primary'} onClick={() => resolve(true)}>
            {input.confirmLabel}
          </Button>
        </>
      }
    >
      {input.body}
    </Modal>
  )
}
