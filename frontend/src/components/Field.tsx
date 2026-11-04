import { createContext, useContext, useId } from 'react'
import type { ReactNode } from 'react'
import { TriangleAlert } from 'lucide-react'

interface FieldCtx {
  id: string
  describedBy?: string
  invalid: boolean
}
const FieldContext = createContext<FieldCtx | null>(null)

/** Inputs placed inside a <Field> pick up id, aria-describedby and aria-invalid from here. */
export function useFieldProps(): { id?: string; 'aria-describedby'?: string; 'aria-invalid'?: boolean } {
  const c = useContext(FieldContext)
  if (!c) return {}
  return { id: c.id, 'aria-describedby': c.describedBy, 'aria-invalid': c.invalid || undefined }
}

export interface FieldProps {
  label: ReactNode
  hint?: ReactNode
  error?: ReactNode
  className?: string
  children: ReactNode
}

export function Field({ label, hint, error, className, children }: FieldProps) {
  const id = useId()
  const hintId = hint ? `${id}-hint` : undefined
  const errId = error ? `${id}-err` : undefined
  const describedBy = [errId, hintId].filter(Boolean).join(' ') || undefined
  return (
    <FieldContext.Provider value={{ id, describedBy, invalid: !!error }}>
      <div className={['field', className].filter(Boolean).join(' ')}>
        <label className="field__label" htmlFor={id}>
          {label}
        </label>
        {children}
        {hint && (
          <div className="field__hint" id={hintId}>
            {hint}
          </div>
        )}
        {error && (
          <div className="field__error" id={errId} role="alert">
            <TriangleAlert size={13} aria-hidden="true" />
            {error}
          </div>
        )}
      </div>
    </FieldContext.Provider>
  )
}
