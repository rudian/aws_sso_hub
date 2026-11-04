import { TerminalApp } from '../../api/models'
import { Select } from '../../components/Select'
import { useI18n } from '../../i18n'
import type { MessageKey } from '../../i18n'

/** Terminal apps that make sense on a platform; "auto" first. */
export function terminalAppsFor(platform: string): TerminalApp[] {
  return platform === 'windows'
    ? [TerminalApp.TerminalAuto, TerminalApp.TerminalWindowsTerminal, TerminalApp.TerminalPowerShell, TerminalApp.TerminalCmd]
    : [TerminalApp.TerminalAuto, TerminalApp.TerminalMacTerminal, TerminalApp.TerminalITerm2]
}

export function TerminalAppSelect({ platform, value, onChange, id }: { platform: string; value: TerminalApp; onChange: (v: TerminalApp) => void; id?: string }) {
  const { t } = useI18n()
  const options = terminalAppsFor(platform)
  // A stored value from another platform stays visible instead of silently showing the wrong option.
  const all = options.includes(value) ? options : [value, ...options]
  return (
    <Select id={id} value={value} onChange={(e) => onChange(e.target.value as TerminalApp)} aria-label={t('term.app')}>
      {all.map((a) => (
        <option key={a} value={a}>
          {t(`term.app.${a}` as MessageKey)}
        </option>
      ))}
    </Select>
  )
}
