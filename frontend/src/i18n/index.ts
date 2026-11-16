import { createContext, createElement, useContext, useMemo } from 'react'
import type { ReactNode } from 'react'
import { formatMessage } from './format'
import type { FormatParams } from './format'
import * as app from './locales/app'
import * as errors from './locales/errors'
import * as accounts from './locales/accounts'
import * as login from './locales/login'
import * as addPortal from './locales/addPortal'
import * as migrate from './locales/migrate'
import * as repos from './locales/repos'
import * as clone from './locales/clone'
import * as git from './locales/git'
import * as onboarding from './locales/onboarding'
import * as health from './locales/health'
import * as term from './locales/term'
import * as settings from './locales/settings'
import * as tut from './locales/tut'
import * as menu from './locales/menu'
import * as projects from './locales/projects'
import * as setup from './locales/setup'
import * as identity from './locales/identity'
import * as update from './locales/update'
import type { UiError } from '../api/errors'
import { errorKeys } from '../api/errors'
import { ErrorCode } from '../api/models'

export type Lang = 'zh-CN' | 'en'

/** Every namespace file, in merge order. keys.test.ts checks that zh and en match inside each one. */
export const namespaces = { app, errors, accounts, login, addPortal, migrate, repos, clone, git, onboarding, health, term, settings, tut, menu, projects, setup, identity, update }

export const zhAll = {
  ...app.zh, ...errors.zh, ...accounts.zh, ...login.zh, ...addPortal.zh, ...migrate.zh, ...repos.zh,
  ...clone.zh, ...git.zh, ...onboarding.zh, ...health.zh, ...term.zh, ...settings.zh, ...tut.zh,
  ...menu.zh, ...projects.zh, ...setup.zh, ...identity.zh, ...update.zh,
} as const
export type MessageKey = keyof typeof zhAll
const enAll: Record<MessageKey, string> = {
  ...app.en, ...errors.en, ...accounts.en, ...login.en, ...addPortal.en, ...migrate.en, ...repos.en,
  ...clone.en, ...git.en, ...onboarding.en, ...health.en, ...term.en, ...settings.en, ...tut.en,
  ...menu.en, ...projects.en, ...setup.en, ...identity.en, ...update.en,
} as Record<MessageKey, string>
const zhMap: Record<MessageKey, string> = zhAll as Record<MessageKey, string>

export type T = (key: MessageKey, params?: Record<string, string | number>) => string

const warned = new Set<string>()

export function translate(lang: Lang, key: MessageKey, params?: FormatParams): string {
  const table = lang === 'zh-CN' ? zhMap : enAll
  const tpl = table[key]
  if (tpl === undefined) {
    if (!warned.has(key)) {
      warned.add(key)
      console.warn(`[i18n] missing key: ${key}`)
    }
    return key
  }
  return formatMessage(tpl, params, lang)
}

export function makeT(lang: Lang): T {
  return (key, params) => translate(lang, key, params)
}

/** navigator.languages.some(l => l.toLowerCase().startsWith('zh')) ? 'zh-CN' : 'en' */
export function systemLang(): Lang {
  const langs = typeof navigator !== 'undefined' ? (navigator.languages?.length ? navigator.languages : [navigator.language ?? '']) : []
  return langs.some((l) => (l ?? '').toLowerCase().startsWith('zh')) ? 'zh-CN' : 'en'
}

interface I18nValue { lang: Lang; t: T }
const defaultValue: I18nValue = { lang: 'en', t: makeT('en') }
const I18nContext = createContext<I18nValue>(defaultValue)

export function I18nProvider(props: { lang: Lang; children?: ReactNode }) {
  const { lang, children } = props
  const value = useMemo<I18nValue>(() => ({ lang, t: makeT(lang) }), [lang])
  return createElement(I18nContext.Provider, { value }, children)
}

export function useI18n(): { lang: Lang; t: T } {
  return useContext(I18nContext)
}

/** Title/body for an error (error table). `detail` is only meant for a "Details" disclosure. */
export function errorText(e: UiError, t: T): { title: string; body?: string; detail?: string } {
  const keys = errorKeys[e.code] ?? errorKeys[ErrorCode.ErrInternal]
  const params: Record<string, string | number> = { ...e.params }
  // E_INVALID_INPUT names the offending field; use a readable label when there is one, the raw name otherwise.
  if (e.code === ErrorCode.ErrInvalidInput && e.params.field) {
    const label = `errors.field.${e.params.field}` as MessageKey
    if (label in zhAll) params.field = t(label)
  }
  if (keys.title === 'error.generic.title' || keys.body === 'error.generic.body') params.reason = e.message
  return {
    title: t(keys.title, params),
    body: keys.body ? t(keys.body, params) : undefined,
    detail: e.detail,
  }
}
