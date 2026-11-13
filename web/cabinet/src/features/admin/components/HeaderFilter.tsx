import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { Check, ChevronDown } from 'lucide-react'

import { cn } from '@/lib/utils'

export interface HeaderFilterOption<T extends string> {
  value: T
  label: string
}

const LIST_GAP_PX = 6
const LIST_MIN_WIDTH_PX = 176

/**
 * Фильтр в шапке колонки: стрелка рядом с названием раскрывает список значений.
 *
 * Список рисуется в портале с `position: fixed`, как у AdminSelect: таблица
 * лежит в контейнере с `overflow`, и обычный выпадающий блок внутри неё
 * обрезался бы по краю карточки.
 *
 * Первая опция считается «без фильтра»: при другом выборе стрелка получает
 * акцент и рядом показывается выбранное значение, чтобы фильтр не терялся.
 */
export function HeaderFilter<T extends string>({
  value,
  options,
  onChange,
  ariaLabel,
}: {
  value: T
  options: HeaderFilterOption<T>[]
  onChange: (value: T) => void
  ariaLabel: string
}) {
  const [open, setOpen] = useState(false)
  const [pos, setPos] = useState<{ left: number; top: number } | null>(null)
  const btnRef = useRef<HTMLButtonElement>(null)
  const listRef = useRef<HTMLUListElement>(null)

  const active = options.length > 0 && value !== options[0].value
  const selected = options.find((o) => o.value === value)

  useLayoutEffect(() => {
    if (!open) {
      setPos(null)
      return
    }
    const place = () => {
      const rect = btnRef.current?.getBoundingClientRect()
      if (!rect) return
      const left = Math.max(8, Math.min(rect.left, window.innerWidth - LIST_MIN_WIDTH_PX - 8))
      setPos({ left, top: rect.bottom + LIST_GAP_PX })
    }
    place()
    window.addEventListener('resize', place)
    window.addEventListener('scroll', place, true)
    return () => {
      window.removeEventListener('resize', place)
      window.removeEventListener('scroll', place, true)
    }
  }, [open])

  useEffect(() => {
    if (!open) return
    const onDown = (e: MouseEvent) => {
      const target = e.target as Node
      if (btnRef.current?.contains(target) || listRef.current?.contains(target)) return
      setOpen(false)
    }
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setOpen(false)
    }
    document.addEventListener('mousedown', onDown)
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('mousedown', onDown)
      document.removeEventListener('keydown', onKey)
    }
  }, [open])

  return (
    <>
      <button
        ref={btnRef}
        type="button"
        aria-label={ariaLabel}
        aria-haspopup="listbox"
        aria-expanded={open}
        onClick={() => setOpen((v) => !v)}
        className={cn(
          'inline-flex items-center gap-1 rounded px-1 py-0.5 text-xs font-medium normal-case tracking-normal transition-colors',
          active
            ? 'bg-primary/15 text-primary hover:bg-primary/25'
            : 'text-muted-foreground/70 hover:bg-accent/15 hover:text-foreground',
          open && 'bg-accent/15 text-foreground',
        )}
      >
        {active && <span className="max-w-[7rem] truncate">{selected?.label}</span>}
        <ChevronDown className={cn('size-3.5 shrink-0 transition-transform', open && 'rotate-180')} aria-hidden />
      </button>

      {open &&
        pos &&
        createPortal(
          <ul
            ref={listRef}
            role="listbox"
            aria-label={ariaLabel}
            style={{ position: 'fixed', left: pos.left, top: pos.top, minWidth: LIST_MIN_WIDTH_PX, zIndex: 9999 }}
            className="overflow-auto rounded-lg border border-border/60 bg-card py-1 text-sm font-medium normal-case tracking-normal text-foreground shadow-xl"
          >
            {options.map((opt) => {
              const isSel = opt.value === value
              return (
                <li key={opt.value} role="option" aria-selected={isSel}>
                  <button
                    type="button"
                    onClick={() => {
                      onChange(opt.value)
                      setOpen(false)
                    }}
                    className={cn(
                      'flex min-h-10 w-full items-center justify-between gap-3 px-3 py-2 text-left transition-colors hover:bg-accent/50',
                      isSel && 'bg-primary/10 text-primary',
                    )}
                  >
                    <span className="truncate">{opt.label}</span>
                    {isSel && <Check className="size-4 shrink-0" />}
                  </button>
                </li>
              )
            })}
          </ul>,
          document.body,
        )}
    </>
  )
}
