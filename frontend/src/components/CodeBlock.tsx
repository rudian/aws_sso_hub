import clsx from 'clsx'
import { CopyButton } from './CopyButton'

export function CodeBlock({ label, code, copyable = true, className }: { label?: string; code: string; copyable?: boolean; className?: string }) {
  return (
    <div className={clsx('code', className)}>
      {(label || copyable) && (
        <div className="code__head">
          <span className="code__lang">{label}</span>
          {copyable && <CopyButton text={code} />}
        </div>
      )}
      <pre className="code__body">{code}</pre>
    </div>
  )
}
