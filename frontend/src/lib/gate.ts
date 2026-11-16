/** The startup tools gate (design 5.1). While it is blocked the setup screen replaces the whole window. */
export const GATE_RECHECK_EVENT = 'codepass:gate-recheck'

/** The setup screen calls this after an install or a manual re-check; the shell asks Tools.GetSetupState again. */
export function requestGateRecheck(): void {
  window.dispatchEvent(new Event(GATE_RECHECK_EVENT))
}
