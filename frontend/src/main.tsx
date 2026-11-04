import React from 'react'
import ReactDOM from 'react-dom/client'
import './styles/tokens.css'
import './styles/components.css'
import { App } from './App'
import { initialUI } from './lib/initialUI'
import { guessPlatform } from './lib/platform'
import { applyLang, applyPlatform } from './lib/theme'
import { systemLang } from './i18n'

// Apply what is known before React renders so the first paint has the right theme, language and platform.
const root = document.documentElement
if (initialUI.theme) root.setAttribute('data-theme', initialUI.theme)
applyPlatform(guessPlatform())
applyLang(initialUI.lang ?? systemLang())

ReactDOM.createRoot(document.getElementById('root') as HTMLElement).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
)
