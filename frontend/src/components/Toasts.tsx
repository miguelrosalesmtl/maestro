import { AlertTriangle, ArrowRightLeft, GitBranch, X, Zap } from 'lucide-react'
import { useStore } from '../store'
import { cx } from '../lib/util'

export function Toasts() {
  const { toasts, dismissToast, openJira } = useStore()
  return (
    <div className="toasts">
      {toasts.map(t => {
        const Icon = { automation: Zap, dev: GitBranch, jira: ArrowRightLeft, error: AlertTriangle }[t.tone]
        return (
          <div key={t.id} className={cx('toast', `toast-${t.tone}`)} onClick={() => t.onClick ? t.onClick() : t.issueKey && openJira(t.issueKey)}>
            <span className="toast-icon"><Icon size={15} /></span>
            <div className="toast-body">
              <div className="toast-title">{t.title}</div>
              {t.detail && <div className="toast-detail">{t.detail}</div>}
            </div>
            <button className="icon-btn" onClick={e => { e.stopPropagation(); dismissToast(t.id) }}><X size={13} /></button>
            <i className="toast-timer" />
          </div>
        )
      })}
    </div>
  )
}
