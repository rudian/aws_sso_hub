import clsx from 'clsx'
import type { ReactNode } from 'react'

export type ToolKind = 'claude' | 'codex' | 'opencode'

// Product names, not UI copy.
const NAMES: Record<ToolKind, string> = { claude: 'Claude Code', codex: 'Codex', opencode: 'OpenCode' }

// Neutral glyphs in an ink tile: no vendor colours, no vendor logos.
const GLYPHS: Record<ToolKind, ReactNode> = {
  claude: (
    <>
      <path d="M12 3v18" />
      <path d="M4.2 7.5l15.6 9" />
      <path d="M19.8 7.5l-15.6 9" />
    </>
  ),
  codex: (
    <>
      <path d="m7 8 4 4-4 4" />
      <path d="M13 16h4" />
    </>
  ),
  opencode: (
    <>
      <path d="M9 5H6a1 1 0 0 0-1 1v12a1 1 0 0 0 1 1h3" />
      <path d="M15 5h3a1 1 0 0 1 1 1v12a1 1 0 0 1-1 1h-3" />
      <circle cx="12" cy="12" r="1.5" />
    </>
  ),
}

export function toolName(kind: ToolKind): string {
  return NAMES[kind]
}

export interface ToolMarkProps {
  tool: ToolKind
  /** 36px tile instead of 22px. */
  large?: boolean
  className?: string
}

export function ToolMark({ tool, large, className }: ToolMarkProps) {
  const name = NAMES[tool]
  return (
    <span className={clsx('tool', large && 'tool--lg', className)} title={name} role="img" aria-label={name}>
      <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
        {GLYPHS[tool]}
      </svg>
    </span>
  )
}
