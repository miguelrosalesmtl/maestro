import { Component, type ErrorInfo, type ReactNode } from 'react'
import { FolderOpen, RotateCcw, TriangleAlert } from 'lucide-react'
import * as App from '../../wailsjs/go/main/App'
import { reportError } from '../lib/errors'

interface State { error: Error | null }

/** Catches render errors so one broken view never leaves a blank window. */
export class ErrorBoundary extends Component<{ children: ReactNode }, State> {
  state: State = { error: null }

  static getDerivedStateFromError(error: Error): State {
    return { error }
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    reportError('error', Object.assign(error, { stack: `${error.stack ?? ''}\nComponent stack:${info.componentStack ?? ''}` }), 'react render')
  }

  render() {
    if (!this.state.error) return this.props.children
    return (
      <div className="crash-screen">
        <div className="crash-card">
          <TriangleAlert size={28} />
          <h1>Something went wrong</h1>
          <p>This part of Maestro hit an error. It has been written to the log file.</p>
          <pre>{this.state.error.message}</pre>
          <div className="crash-actions">
            <button className="btn primary" onClick={() => location.reload()}><RotateCcw size={14} /> Reload</button>
            <button className="btn ghost" onClick={() => { void App.OpenLogFolder().catch(() => {}) }}><FolderOpen size={14} /> Open log folder</button>
          </div>
        </div>
      </div>
    )
  }
}
