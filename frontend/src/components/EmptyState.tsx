import type { ReactNode } from 'react'

/** Default art: a keycard outline in line art (`--text-3`). */
export function KeycardArt() {
  return (
    <svg className="empty__art" viewBox="0 0 72 72" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <rect x="10" y="20" width="48" height="34" rx="6" opacity=".45" transform="rotate(-8 34 37)" />
      <rect x="14" y="18" width="48" height="34" rx="6" />
      <path d="M14 30h8" strokeDasharray="2 4" />
      <circle cx="42" cy="32" r="4.5" />
      <path d="M39.5 37.5h5l1.6 8h-8.2z" />
    </svg>
  )
}

export function EmptyState({ art, title, text, actions }: { art?: ReactNode; title: ReactNode; text?: ReactNode; actions?: ReactNode }) {
  return (
    <div className="empty">
      {art ?? <KeycardArt />}
      <div className="empty__title">{title}</div>
      {text && <div className="empty__text">{text}</div>}
      {actions && <div className="empty__actions">{actions}</div>}
    </div>
  )
}
