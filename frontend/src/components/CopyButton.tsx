import clsx from 'clsx'
import { Check, Copy } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { copyText } from '../api/platform'
import { useI18n } from '../i18n'
import { Button } from './Button'

/** Copies `text` and shows "Copied" with a check for 1.5 s (`.code__copy.is-copied`). */
export function CopyButton({ text, label, className }: { text: string; label?: string; className?: string }) {
  const { t } = useI18n()
  const [copied, setCopied] = useState(false)
  const timer = useRef<number | undefined>(undefined)
  useEffect(() => () => window.clearTimeout(timer.current), [])
  return (
    <Button
      variant="ghost"
      size="sm"
      icon={copied ? Check : Copy}
      className={clsx('code__copy', copied && 'is-copied', className)}
      onClick={async () => {
        try {
          await copyText(text)
          setCopied(true)
          window.clearTimeout(timer.current)
          timer.current = window.setTimeout(() => setCopied(false), 1500)
        } catch {
          /* clipboard unavailable: nothing to confirm */
        }
      }}
    >
      {copied ? t('common.copied') : (label ?? t('common.copy'))}
    </Button>
  )
}
