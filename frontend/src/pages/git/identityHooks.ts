import { useCallback } from 'react'
import { Identity } from '../../api'
import { ErrorCode } from '../../api/models'
import type { IdentityChangeResult } from '../../api/models'
import { toUiError } from '../../api/errors'
import { useI18n } from '../../i18n'
import { useActions } from '../../shell/AppActions'
import { useReportError } from '../repos/hooks'

/**
 * Toast helpers for identity writes. Every apply returns an undo ID that lives in memory only (the 32 most recent
 * applies): E_INVALID_INPUT with field undoId means "cannot undo any more".
 */
export function useIdentityToast(): {
  /** A success toast with Undo when the apply wrote something. */
  done: (title: string, res: IdentityChangeResult, kind?: 'ok' | 'warn') => void
  report: (e: unknown) => void
} {
  const { t } = useI18n()
  const { toast } = useActions()
  const report = useReportError()

  const undo = useCallback(
    async (undoId: string) => {
      try {
        await Identity.Undo(undoId)
        toast({ kind: 'ok', title: t('identity.undo.done') })
      } catch (e) {
        const ue = toUiError(e)
        if (ue.code === ErrorCode.ErrInvalidInput && ue.params.field === 'undoId') toast({ kind: 'warn', title: t('identity.undo.gone') })
        else report(e)
      }
    },
    [report, t, toast],
  )

  const done = useCallback(
    (title: string, res: IdentityChangeResult, kind: 'ok' | 'warn' = 'ok') => {
      toast({ kind, title, action: res.undoId ? { label: t('common.undo'), onClick: () => void undo(res.undoId) } : undefined })
    },
    [t, toast, undo],
  )
  return { done, report }
}
