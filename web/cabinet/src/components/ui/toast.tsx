import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from 'react'
import { createPortal } from 'react-dom'
import { AlertTriangle, CheckCircle2, Info, X } from 'lucide-react'

import { cn } from '@/lib/utils'

/**
 * Минимальные тосты без внешних зависимостей.
 *
 * До этого в кабинете не было вообще никакого способа сообщить о результате
 * действия: мутации вроде активации триала имели только onSuccess, и при ошибке
 * кнопка просто разблокировалась — пользователь не понимал, сработало или нет.
 *
 * Держим сознательно простым: очередь сообщений, автоскрытие, портал в body.
 * Ошибки живут дольше успеха — их нужно успеть прочитать.
 */

type ToastVariant = 'success' | 'error' | 'info'

/** Необязательная кнопка в тосте — например, «Отменить». Нажатие закрывает тост. */
interface ToastAction {
  label: string
  onClick: () => void
}

interface ToastOptions {
  action?: ToastAction
  /** Своя длительность показа вместо стандартной для варианта. */
  durationMs?: number
}

interface Toast {
  id: number
  message: string
  variant: ToastVariant
  action?: ToastAction
  /** Тост уезжает за край экрана — ещё в списке, но уже без кликов. */
  leaving?: boolean
}

interface ToastApi {
  success: (message: string, options?: ToastOptions) => void
  error: (message: string, options?: ToastOptions) => void
  info: (message: string, options?: ToastOptions) => void
}

const ToastContext = createContext<ToastApi | null>(null)

const AUTO_HIDE_MS: Record<ToastVariant, number> = {
  success: 3000,
  info: 4000,
  error: 6000,
}

/** Длительность анимации ухода — совпадает с toast-out в tailwind.config. */
const LEAVE_MS = 300

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([])
  const nextId = useRef(1)
  const timers = useRef(new Map<number, number>())

  // Сначала тост уезжает вбок, и только потом пропадает из списка.
  const dismiss = useCallback((id: number) => {
    const timer = timers.current.get(id)
    if (timer !== undefined) window.clearTimeout(timer)
    setToasts((prev) => prev.map((toast) => (toast.id === id ? { ...toast, leaving: true } : toast)))
    timers.current.set(
      id,
      window.setTimeout(() => {
        setToasts((prev) => prev.filter((toast) => toast.id !== id))
        timers.current.delete(id)
      }, LEAVE_MS),
    )
  }, [])

  const push = useCallback(
    (message: string, variant: ToastVariant, options?: ToastOptions) => {
      const text = message.trim()
      if (!text) return
      const id = nextId.current++
      setToasts((prev) => [...prev.slice(-2), { id, message: text, variant, action: options?.action }])
      timers.current.set(
        id,
        window.setTimeout(() => dismiss(id), options?.durationMs ?? AUTO_HIDE_MS[variant]),
      )
    },
    [dismiss],
  )

  // Снимаем таймеры при размонтировании: иначе setState по уже мёртвому дереву.
  useEffect(() => {
    const pending = timers.current
    return () => {
      pending.forEach((timer) => window.clearTimeout(timer))
      pending.clear()
    }
  }, [])

  const api = useMemo<ToastApi>(
    () => ({
      success: (message: string, options?: ToastOptions) => push(message, 'success', options),
      error: (message: string, options?: ToastOptions) => push(message, 'error', options),
      info: (message: string, options?: ToastOptions) => push(message, 'info', options),
    }),
    [push],
  )

  return (
    <ToastContext.Provider value={api}>
      {children}
      {typeof document !== 'undefined' &&
        createPortal(<ToastViewport toasts={toasts} onDismiss={dismiss} />, document.body)}
    </ToastContext.Provider>
  )
}

/** Вне провайдера возвращает no-op: тост не должен ронять экран, на котором вызван. */
export function useToast(): ToastApi {
  const ctx = useContext(ToastContext)
  return ctx ?? noopToast
}

const noopToast: ToastApi = {
  success: () => {},
  error: () => {},
  info: () => {},
}

const variantIcon = {
  success: CheckCircle2,
  error: AlertTriangle,
  info: Info,
} as const

const variantClass: Record<ToastVariant, string> = {
  success: 'border-emerald-500/30 text-emerald-700 dark:text-emerald-300',
  error: 'border-destructive/40 text-destructive',
  info: 'border-border text-foreground',
}

function ToastViewport({
  toasts,
  onDismiss,
}: {
  toasts: Toast[]
  onDismiss: (id: number) => void
}) {
  if (!toasts.length) return null

  return (
    <div
      // Над нижней навигацией на мобильном; на десктопе она скрыта, отступ безвреден.
      className="pointer-events-none fixed inset-x-0 z-[3000] flex flex-col items-center gap-2 px-3 bottom-[calc(5.5rem+max(env(safe-area-inset-bottom,0px),var(--cabinet-tg-safe-bottom)))] sm:bottom-4"
      role="status"
      aria-live="polite"
    >
      {toasts.map((toast) => {
        const Icon = variantIcon[toast.variant]
        return (
          <div
            key={toast.id}
            className={cn(
              'pointer-events-auto flex w-full max-w-sm items-start gap-2.5 rounded-xl border bg-card/95 px-3.5 py-3 text-sm shadow-lg backdrop-blur-md',
              toast.leaving ? 'pointer-events-none animate-toast-out' : 'animate-fade-in',
              variantClass[toast.variant],
            )}
          >
            <Icon className="mt-0.5 size-4 shrink-0" aria-hidden />
            <p className="min-w-0 flex-1 text-card-foreground">{toast.message}</p>
            {toast.action ? (
              <button
                type="button"
                onClick={() => {
                  toast.action?.onClick()
                  onDismiss(toast.id)
                }}
                className="-my-0.5 shrink-0 rounded-md px-1.5 py-0.5 font-semibold text-primary transition-colors hover:bg-primary/10"
              >
                {toast.action.label}
              </button>
            ) : null}
            <button
              type="button"
              onClick={() => onDismiss(toast.id)}
              className="-mr-1 -mt-1 shrink-0 rounded-md p-1 text-muted-foreground transition-colors hover:text-foreground"
              aria-label="✕"
            >
              <X className="size-3.5" aria-hidden />
            </button>
          </div>
        )
      })}
    </div>
  )
}
