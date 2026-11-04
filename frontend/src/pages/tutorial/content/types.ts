import type { TutorialSectionId } from '../../../lib/route'

/**
 * Tutorial content model. `text`, `lead`, callout bodies and FAQ answers allow `**bold**` and `` `code` `` only;
 * a newline starts a new paragraph. Nothing is ever rendered as HTML.
 */
export interface TutorialStep {
  title: string
  text: string
  /** Console navigation shown as breadcrumb chips. Console labels are AWS's own wording and can change; the step text also names the place in words. */
  crumbs?: string[]
  code?: { label: string; text: string }
}

export interface TutorialCallout {
  kind: 'info' | 'warn'
  title: string
  body: string
}

export interface TutorialFaq {
  q: string
  a: string
}

export interface TutorialSection {
  id: TutorialSectionId
  group: 'admin' | 'dev'
  minutes: number
  title: string
  lead: string
  steps: TutorialStep[]
  callouts?: TutorialCallout[]
  faq?: TutorialFaq[]
}

export const SECTION_ORDER: TutorialSectionId[] = ['why', 'enable', 'assign', 'portal', 'migrate', 'bind', 'daily', 'faq']
