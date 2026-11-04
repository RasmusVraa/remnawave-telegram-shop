import { useEffect, useLayoutEffect, useRef, useState, type CSSProperties } from 'react'
import { createPortal } from 'react-dom'
import { Check, ChevronDown } from 'lucide-react'

import { cn } from '@/lib/utils'
import { Spinner } from '@/components/ui/spinner'
import { surface } from './Surface'

const LIST_MAX_HEIGHT_PX = 240
const LIST_GAP_PX = 6
const ITEM_HEIGHT_PX = 40

export interface AdminSelectOption<T extends string | number = string> {
  value: T
  label: string
  labelStyle?: CSSProperties
}

interface DropdownPosition {
  left: number
  width: number
  top?: number
  bottom?: number
  openUp: boolean
}

interface AdminSelectProps<T extends string | number = string> {
  value: T | null
  options: AdminSelectOption<T>[]
  onChange: (value: T | null) => void
  placeholder: string
  ariaLabel: string
  className?: string
  allowEmpty?: boolean
  emptyLabel?: string
  disabled?: boolean
  /** Выбор сохраняется на сервере: селект заблокирован, вместо стрелки спиннер. */
  busy?: boolean
  id?: string
}

function estimateListHeight(optionCount: number, allowEmpty: boolean): number {
  const items = optionCount + (allowEmpty ? 1 : 0)
  return Math.min(items * ITEM_HEIGHT_PX + 8, LIST_MAX_HEIGHT_PX)
}

export function AdminSelect<T extends string | number = string>({
  value,
  options,
  onChange,
  placeholder,
  ariaLabel,
  className,
  allowEmpty = false,
  emptyLabel,
  disabled: disabledProp = false,
  busy = false,
  id,
}: AdminSelectProps<T>) {
  const disabled = disabledProp || busy
  const [open, setOpen] = useState(false)
  const [position, setPosition] = useState<DropdownPosition | null>(null)
  const rootRef = useRef<HTMLDivElement>(null)
  const buttonRef = useRef<HTMLButtonElement>(null)
  const listRef = useRef<HTMLUListElement>(null)

  const updatePosition = () => {
    const btn = buttonRef.current
    if (!btn) return

    const rect = btn.getBoundingClientRect()
    const listHeight = estimateListHeight(options.length, allowEmpty)
    const spaceBelow = window.innerHeight - rect.bottom - LIST_GAP_PX
    const spaceAbove = rect.top - LIST_GAP_PX
    const openUp = spaceBelow < listHeight && spaceAbove > spaceBelow

    setPosition({
      left: rect.left,
      width: rect.width,
      openUp,
      ...(openUp
        ? { bottom: window.innerHeight - rect.top + LIST_GAP_PX }
        : { top: rect.bottom + LIST_GAP_PX }),
    })
  }

  useLayoutEffect(() => {
    if (!open) {
      setPosition(null)
      return
    }
    updatePosition()
    window.addEventListener('resize', updatePosition)
    window.addEventListener('scroll', updatePosition, true)
    return () => {
      window.removeEventListener('resize', updatePosition)
      window.removeEventListener('scroll', updatePosition, true)
    }
  }, [open, options.length, allowEmpty])

  useEffect(() => {
    if (!open) return
    const onDocClick = (e: MouseEvent) => {
      const target = e.target as Node
      if (rootRef.current?.contains(target) || listRef.current?.contains(target)) return
      setOpen(false)
    }
    const onEsc = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setOpen(false)
    }
    document.addEventListener('mousedown', onDocClick)
    document.addEventListener('keydown', onEsc)
    return () => {
      document.removeEventListener('mousedown', onDocClick)
      document.removeEventListener('keydown', onEsc)
    }
  }, [open])

  const selectedOption = options.find((o) => o.value === value)

  const selectedLabel =
    value == null || value === ''
      ? emptyLabel ?? placeholder
      : selectedOption?.label ?? placeholder

  const pick = (next: T | null) => {
    onChange(next)
    setOpen(false)
  }

  const listbox = open && position && (
    <ul
      ref={listRef}
      role="listbox"
      aria-label={ariaLabel}
      style={{
        position: 'fixed',
        left: position.left,
        width: position.width,
        zIndex: 9999,
        ...(position.openUp ? { bottom: position.bottom } : { top: position.top }),
      }}
      /*
         * Без cabinet-elevated-card: в тёмной теме он даёт полупрозрачный
         * фон (card / 0.72), и список, висящий над содержимым страницы,
         * просвечивал насквозь. Селектор у него специфичнее утилиты, так
         * что bg-card его не перебивал.
         */
        className="max-h-60 overflow-auto rounded-lg border border-border/60 bg-card py-1 shadow-xl"
    >
      {allowEmpty && (
        <li role="option" aria-selected={value == null || value === ''}>
          <button
            type="button"
            onClick={() => pick(null)}
            className={cn(
              'flex min-h-10 w-full items-center justify-between gap-2 px-3 py-2 text-left text-sm transition-colors hover:bg-accent/50',
              (value == null || value === '') && 'bg-primary/10 text-primary',
            )}
          >
            <span>{emptyLabel ?? placeholder}</span>
            {(value == null || value === '') && <Check className="size-4 shrink-0" />}
          </button>
        </li>
      )}
      {options.map((opt) => {
        const selected = opt.value === value
        return (
          <li key={String(opt.value)} role="option" aria-selected={selected}>
            <button
              type="button"
              onClick={() => pick(opt.value)}
              className={cn(
                'flex min-h-10 w-full items-center justify-between gap-2 px-3 py-2 text-left text-sm transition-colors hover:bg-accent/50',
                selected && 'bg-primary/10 text-primary',
              )}
            >
              <span className="truncate" style={opt.labelStyle}>
                {opt.label}
              </span>
              {selected && <Check className="size-4 shrink-0" />}
            </button>
          </li>
        )
      })}
    </ul>
  )

  return (
    <div ref={rootRef} className={cn('relative', className)}>
      <button
        ref={buttonRef}
        type="button"
        id={id}
        disabled={disabled}
        onClick={() => {
          if (disabled) return
          setOpen((v) => !v)
        }}
        aria-haspopup="listbox"
        aria-expanded={open}
        aria-label={ariaLabel}
        aria-busy={busy || undefined}
        className={cn(
          // Фон на ступень светлее карточки, а не `cabinet-elevated-card`: тот
          // заливает контрол градиентом card → secondary поверх такой же
          // карточки-контейнера, и селект сливался с тем, на чём лежит.
          // Токен, а не цвет: все декор-темы переопределяют --secondary сами.
          surface('raised'),
          'flex min-h-11 w-full items-center justify-between gap-2 rounded-lg px-3 py-2 text-sm font-medium transition-colors hover:bg-accent/40',
          open && 'border-primary/40 ring-1 ring-primary/20',
          disabled && 'cursor-not-allowed opacity-60 hover:bg-transparent',
          busy && 'cursor-wait !opacity-100',
        )}
      >
        <span className="truncate text-left" style={selectedOption?.labelStyle}>
          {selectedLabel}
        </span>
        {busy ? (
          <Spinner className="text-muted-foreground" />
        ) : (
          <ChevronDown
            className={cn(
              'size-4 shrink-0 text-muted-foreground transition-transform',
              open && (position?.openUp ? 'rotate-0' : 'rotate-180'),
            )}
          />
        )}
      </button>

      {listbox && createPortal(listbox, document.body)}
    </div>
  )
}
