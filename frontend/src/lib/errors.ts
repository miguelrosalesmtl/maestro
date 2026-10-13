// Sends UI errors to the backend log file (internal/applog). Never throws:
// error reporting must not cause new errors.
import * as App from '../../wailsjs/go/main/App'

type Level = 'error' | 'warn' | 'info'

export function reportError(level: Level, err: unknown, source: string) {
  try {
    const e = err instanceof Error ? err : new Error(typeof err === 'string' ? err : safeString(err))
    void App.LogFrontend(level, e.message || String(e), e.stack ?? '', source).catch(() => {})
  } catch {
    /* the bridge is not ready (e.g. very early in startup) */
  }
}

function safeString(v: unknown) {
  try { return JSON.stringify(v) } catch { return String(v) }
}

let installed = false

/** Uncaught exceptions and unhandled promise rejections anywhere in the UI. */
export function installGlobalErrorHandlers() {
  if (installed) return
  installed = true
  window.addEventListener('error', e => reportError('error', e.error ?? e.message, `window.onerror ${e.filename}:${e.lineno}`))
  window.addEventListener('unhandledrejection', e => reportError('error', e.reason, 'unhandledrejection'))
}
