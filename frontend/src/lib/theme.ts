import type { Language, MotionPref, Theme } from '../api/models'
import { Language as L, Theme as T } from '../api/models'
import { systemLang } from '../i18n'
import type { Lang } from '../i18n'
import { applyMotionPref } from './motion'

/** system removes the attribute so prefers-color-scheme decides. */
export function applyTheme(theme: Theme): void {
  const root = document.documentElement
  if (theme === T.ThemeLight) root.setAttribute('data-theme', 'light')
  else if (theme === T.ThemeDark) root.setAttribute('data-theme', 'dark')
  else root.removeAttribute('data-theme')
}

/** Writes data-motion: reduce and full are explicit, system removes it so the OS decides (lib/motion.ts). */
export function applyMotion(motion: MotionPref): void {
  applyMotionPref(motion)
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
