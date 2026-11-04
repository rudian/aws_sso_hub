import type { Language, MotionPref, Theme } from '../api/models'
import { Language as L, MotionPref as M, Theme as T } from '../api/models'
import { systemLang } from '../i18n'
import type { Lang } from '../i18n'

/** system removes the attribute so prefers-color-scheme decides. */
export function applyTheme(theme: Theme): void {
  const root = document.documentElement
  if (theme === T.ThemeLight) root.setAttribute('data-theme', 'light')
  else if (theme === T.ThemeDark) root.setAttribute('data-theme', 'dark')
  else root.removeAttribute('data-theme')
}

/** reduce -> data-motion="reduce"; full forces motion; system defers to prefers-reduced-motion in CSS. */
export function applyMotion(motion: MotionPref): void {
  const root = document.documentElement
  if (motion === M.MotionReduce) root.setAttribute('data-motion', 'reduce')
  else root.removeAttribute('data-motion')
}

export function applyPlatform(platform: string): void {
  document.documentElement.setAttribute('data-platform', platform)
}

export function effectiveLang(language: Language | undefined): Lang {
  if (language === L.LanguageZhCN) return 'zh-CN'
  if (language === L.LanguageEn) return 'en'
  return systemLang()
}

export function applyLang(lang: Lang): void {
  document.documentElement.setAttribute('lang', lang)
}
