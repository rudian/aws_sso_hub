/** Paste of more than this many lines asks once (ui-design 5.7). */
export const PASTE_CONFIRM_LINES = 5

export function countLines(text: string): number {
  if (text === '') return 0
  const t = text.replace(/\r\n?/g, '\n').replace(/\n$/, '')
  return t.split('\n').length
}

export function needsPasteConfirm(text: string): boolean {
  return countLines(text) > PASTE_CONFIRM_LINES
}

/**
 * Remove control characters from clipboard text before it is pasted: every C0 control except tab, CR and LF (this takes
 * out ESC, so a pasted ESC[201~ cannot end bracketed paste early), DEL, and every C1 control.
 */
export function sanitizePaste(text: string): string {
  // eslint-disable-next-line no-control-regex
  return text.replace(/[\x00-\x08\x0b\x0c\x0e-\x1f\x7f-\x9f]/g, '')
}
