import { useEffect, useRef, useState, type ReactNode } from 'react'
import { ChevronDown, Search } from 'lucide-react'
import { cx } from '../lib/util'
import { useOutside } from './bits'

export interface ComboItem {
  value: string
  label: ReactNode
  text: string // what the search matches against
}

/** Searchable dropdown used for parent and link pickers. */
export function Combobox({ items, value, onChange, placeholder = 'Select…', className }: {
  items: ComboItem[]
  value: string
  onChange: (v: string) => void
  placeholder?: string
  className?: string
}) {
  const [open, setOpen] = useState(false)
  const [q, setQ] = useState('')
  const [idx, setIdx] = useState(0)
  const ref = useRef<HTMLDivElement>(null)
  const input = useRef<HTMLInputElement>(null)
  useOutside(ref, () => setOpen(false))

  const terms = q.toLowerCase().split(/\s+/).filter(Boolean)
  const shown = items.filter(i => terms.every(t => i.text.toLowerCase().includes(t)))
  const current = items.find(i => i.value === value)

  useEffect(() => { if (open) { setQ(''); setIdx(0); setTimeout(() => input.current?.focus()) } }, [open])
  useEffect(() => setIdx(0), [q])

  const pick = (v: string) => { onChange(v); setOpen(false) }

  return (
    <div className={cx('combo', className)} ref={ref}>
      <button type="button" className={cx('combo-btn', open && 'open')} onClick={() => setOpen(o => !o)}>
        <span className="combo-value">{current ? current.label : <span className="muted">{placeholder}</span>}</span>
        <ChevronDown size={14} />
      </button>
      {open && (
        <div className="combo-pop pop-in">
          <div className="combo-search">
            <Search size={13} />
            <input ref={input} value={q} onChange={e => setQ(e.target.value)} placeholder="Search…"
              onKeyDown={e => {
                if (e.key === 'ArrowDown') { e.preventDefault(); setIdx(i => Math.min(i + 1, shown.length - 1)) }
                if (e.key === 'ArrowUp') { e.preventDefault(); setIdx(i => Math.max(i - 1, 0)) }
                if (e.key === 'Enter') { e.preventDefault(); if (shown[idx]) pick(shown[idx].value) }
                if (e.key === 'Escape') { e.stopPropagation(); setOpen(false) }
              }} />
          </div>
          <div className="combo-list scroll">
            {shown.map((it, i) => (
              <button type="button" key={it.value} className={cx('combo-item', i === idx && 'on', it.value === value && 'selected')}
                onMouseEnter={() => setIdx(i)} onClick={() => pick(it.value)}>
                {it.label}
              </button>
            ))}
            {!shown.length && <div className="combo-empty">No matches</div>}
          </div>
        </div>
      )}
    </div>
  )
}
