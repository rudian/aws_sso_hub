import type { Lang } from '../../../i18n'
import { en } from './en'
import type { TutorialSection } from './types'
import { zh } from './zh'

export { SECTION_ORDER } from './types'
export type { TutorialCallout, TutorialFaq, TutorialSection, TutorialStep } from './types'

/** The tutorial text for a language; it follows the UI language unless the user picked one in the tutorial. */
export function tutorialFor(lang: Lang): TutorialSection[] {
  return lang === 'zh-CN' ? zh : en
}
