import { useEffect } from 'react'
import { X, CheckCircle2, AlertCircle, Info } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription } from '@/components/ui/alert'
import { hapticError, hapticSuccess } from '@/lib/haptic'
import { cn } from '@/lib/utils'

export interface AdminFeedbackState {
  type: 'success' | 'error' | 'info'
  message: string
}

interface AdminFeedbackProps {
  feedback: AdminFeedbackState | null
  onDismiss?: () => void
  className?: string
  /** inline — в потоке страницы; toast — всплывающее уведомление */
  mode?: 'inline' | 'toast'
  autoDismissMs?: number
}

const toastVariantClasses: Record<AdminFeedbackState['type'], string> = {
  success:
    'border-emerald-500/45 bg-card text-emerald-800 shadow-[0_8px_32px_rgba(0,0,0,0.18)] backdrop-blur-md dark:border-emerald-400/35 dark:bg-[hsl(var(--card))] dark:text-emerald-300',
  error:
    'border-destructive/55 bg-card text-destructive shadow-[0_8px_32px_rgba(0,0,0,0.18)] backdrop-blur-md dark:border-destructive/45 dark:bg-[hsl(var(--card))]',
  info:
    'border-border bg-card text-foreground shadow-[0_8px_32px_rgba(0,0,0,0.18)] backdrop-blur-md dark:bg-[hsl(var(--card))]',
}

export function AdminFeedback({
  feedback,
  onDismiss,
  className,
  mode = 'toast',
  autoDismissMs = 4500,
}: AdminFeedbackProps) {
  const { t } = useTranslation()

  // Результат действия — вибрацией: на этом уведомлении держится почти вся обратная связь админки.
  useEffect(() => {
    if (feedback?.type === 'error') hapticError()
    else if (feedback?.type === 'success') hapticSuccess()
  }, [feedback])

  useEffect(() => {
    if (!feedback || !onDismiss || mode !== 'toast') return
    const ms = feedback.type === 'error' ? 6500 : autoDismissMs
    const timer = window.setTimeout(onDismiss, ms)
    return () => window.clearTimeout(timer)
  }, [feedback, onDismiss, mode, autoDismissMs])

  if (!feedback) return null

  const Icon =
    feedback.type === 'error' ? AlertCircle : feedback.type === 'info' ? Info : CheckCircle2

  const alert = (
    <Alert
      className={cn(
        mode === 'toast' ? toastVariantClasses[feedback.type] : undefined,
        mode === 'toast' && 'mb-0',
        !mode || mode === 'inline'
          ? feedback.type === 'error'
            ? 'border-destructive/50 bg-destructive/10 text-destructive'
            : feedback.type === 'success'
              ? 'border-emerald-500/40 bg-emerald-500/10 text-emerald-800 dark:border-emerald-400/35 dark:bg-emerald-500/15 dark:text-emerald-300'
              : 'bg-card text-foreground'
          : undefined,
        className,
      )}
    >
      <AlertDescription className="flex items-start gap-2">
        <Icon className="size-4 shrink-0 mt-0.5" />
        <span className="flex-1">{feedback.message}</span>
        {onDismiss && (
          <button
            type="button"
            onClick={onDismiss}
            className="rounded p-0.5 opacity-70 hover:opacity-100"
            aria-label={t('common.close')}
          >
            <X className="size-4" />
          </button>
        )}
      </AlertDescription>
    </Alert>
  )

  if (mode === 'inline') {
    return <div className="mb-4">{alert}</div>
  }

  return (
    <div
      className="pointer-events-none fixed bottom-4 left-4 right-4 z-[250] flex justify-center sm:left-auto sm:right-6 sm:justify-end"
      role="status"
      aria-live="polite"
    >
      <div className="pointer-events-auto w-full max-w-sm animate-fade-in">{alert}</div>
    </div>
  )
}
