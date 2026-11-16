import type { ReactNode } from 'react'
import { EmptyState } from './EmptyState'

/** A route that has no screen yet (the owning task replaces the page that uses this). */
export function PlaceholderPage({ title, text }: { title: ReactNode; text: ReactNode }) {
  return (
    <div className="page">
      <EmptyState title={title} text={text} />
    </div>
  )
}
