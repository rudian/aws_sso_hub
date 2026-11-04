import { DefaultAction } from '../../api/models'
import type { GitGlobalState, HelperEntry } from '../../api/models'

const sameEntry = (a: HelperEntry, b: HelperEntry) => a.scope === b.scope && a.key === b.key && a.value === b.value

/** The credential lines that concern CodeCommit: every line except the unrelated helpers shown in their own section. */
export function codeCommitLines(state: GitGlobalState): HelperEntry[] {
  const other = state.otherHelpers ?? []
  return (state.credentialLines ?? []).filter((l) => !other.some((o) => sameEntry(o, l)))
}

/** One line as git config prints it: `credential.helper = value`. */
export function formatEntry(e: HelperEntry): string {
  return `${e.key} = ${e.value}`
}

/** What the default card starts on: removal only when it can actually be applied, otherwise "keep as is". */
export function initialDefaultAction(opts: { wantRemove: boolean; hasDefault: boolean; urlRulesBlock: boolean }): DefaultAction {
  return opts.wantRemove && opts.hasDefault && !opts.urlRulesBlock ? DefaultAction.DefaultRemove : DefaultAction.DefaultKeep
}

/** A selected option that is no longer available (the radio is disabled) falls back to "keep as is". */
export function effectiveDefaultAction(action: DefaultAction, opts: { hasDefault: boolean; urlRulesBlock: boolean }): DefaultAction {
  if (action === DefaultAction.DefaultRemove && (!opts.hasDefault || opts.urlRulesBlock)) return DefaultAction.DefaultKeep
  return action
}
