import { type ReactNode, useEffect, useRef, useState } from 'react'

import { cn } from '@/lib/utils'

/**
 * Закрытое поле с подсказкой: когда locked, поле недоступно, а при наведении (на телефоне — нажатии)
 * всплывает пояснение, почему, и, если нужно, кнопка, которая снимает причину.
 * align — к какому краю прижимается подсказка: у полей справа она должна расти влево, а не за окно.
 */
export function LockedHint({
  locked,
  message,
  action,
  align = 'left',
  className,
  children,
}: {
  locked: boolean
  message: ReactNode
  action?: ReactNode
  align?: 'left' | 'right'
  className?: string
  children: ReactNode
}) {
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!open) return
    const close = (e: MouseEvent | TouchEvent) => {
      if (!ref.current?.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', close)
    document.addEventListener('touchstart', close)
    return () => {
      document.removeEventListener('mousedown', close)
      document.removeEventListener('touchstart', close)
    }
  }, [open])

  // Причина снята — подсказка не нужна.
  useEffect(() => {
    if (!locked) setOpen(false)
  }, [locked])

  return (
    <div
      ref={ref}
      className={cn('relative', className)}
      onMouseEnter={() => locked && setOpen(true)}
      onMouseLeave={() => setOpen(false)}
    >
      {children}
      {locked && (
        <button
          type="button"
          aria-label={typeof message === 'string' ? message : undefined}
          aria-expanded={open}
          onClick={() => setOpen((o) => !o)}
          onFocus={() => setOpen(true)}
          className="absolute inset-0 z-10 cursor-not-allowed rounded-lg"
        />
      )}
      {locked && open && (
        // Внешний слой с отступом сверху, а не margin: иначе при переходе курсора с поля на подсказку между ними
        // был бы зазор, подсказка закрывалась бы, и до кнопки внутри было бы не дотянуться.
        <div role="status" className={cn('absolute top-full z-30 w-72 max-w-[calc(100vw-3rem)] pt-1.5', align === 'right' ? 'right-0' : 'left-0')}>
          <div className="space-y-2 rounded-lg border border-amber-500/40 bg-card px-3 py-2.5 text-left text-xs font-normal leading-snug text-foreground shadow-lg">
            <p>{message}</p>
            {action}
          </div>
        </div>
      )}
    </div>
  )
}
