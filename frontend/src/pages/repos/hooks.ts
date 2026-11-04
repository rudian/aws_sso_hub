import { useCallback } from 'react'
import { ErrorCode } from '../../api/models'
import { toUiError } from '../../api/errors'
import { errorText, useI18n } from '../../i18n'
import { useActions } from '../../shell/AppActions'
import { System } from '../../api/services'

/** Danger toast for any thrown backend error; a cancelled call stays silent. */
export function useReportError(): (e: unknown) => void {
  const { t } = useI18n()
  const { toast } = useActions()
  return useCallback(
    (e: unknown) => {
      const ue = toUiError(e)
      if (ue.code === ErrorCode.ErrCancelled) return
      const text = errorText(ue, t)
      toast({ kind: 'danger', title: text.title, body: text.body })
    },
    [t, toast],
  )
}

/** Restore the backup that came with a write (the Undo action of a toast). */
export function useUndoBackup(afterwards: () => Promise<void> | void): (backupId: string) => Promise<void> {
  const report = useReportError()
  return useCallback(
    async (backupId: string) => {
      try {
        await System.RestoreBackup(backupId, false)
        await afterwards()
      } catch (e) {
        report(e)
      }
    },
    [afterwards, report],
  )
}
