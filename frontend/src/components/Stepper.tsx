import clsx from 'clsx'
import type { ReactNode } from 'react'

export interface StepperStep {
  id: string
  label: ReactNode
  hint?: ReactNode
  state: 'done' | 'current' | 'todo' | 'error'
}

export function Stepper({ steps, horizontal, className }: { steps: StepperStep[]; horizontal?: boolean; className?: string }) {
  return (
    <ol className={clsx('stepper', horizontal && 'stepper--h', className)}>
      {steps.map((s) => (
        <li key={s.id} className={clsx('stepper__step', s.state !== 'todo' && `is-${s.state}`)} aria-current={s.state === 'current' ? 'step' : undefined}>
          <span className="stepper__marker" aria-hidden="true" />
          <span className="stepper__label">{s.label}</span>
          {s.hint && <span className="stepper__hint">{s.hint}</span>}
        </li>
      ))}
    </ol>
  )
}
