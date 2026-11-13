import { cn } from '@/lib/utils'
import { Spinner } from '@/components/ui/spinner'

interface AdminToggleSwitchProps {
  checked: boolean
  onChange: (checked: boolean) => void
  disabled?: boolean
  /** Значение сохраняется на сервере: переключатель заблокирован, на ползунке спиннер. */
  busy?: boolean
  id?: string
  'aria-label'?: string
  /** sm — для тесных мест вроде плитки тарифа. */
  size?: 'md' | 'sm'
}

/** Toggle в стиле Android/iOS — light/dark через design tokens. */
export function AdminToggleSwitch({
  checked,
  onChange,
  disabled,
  busy = false,
  id,
  'aria-label': ariaLabel,
  size = 'md',
}: AdminToggleSwitchProps) {
  const sm = size === 'sm'

  return (
    <button
      type="button"
      id={id}
      role="switch"
      aria-checked={checked}
      aria-label={ariaLabel}
      aria-busy={busy || undefined}
      disabled={disabled || busy}
      onClick={() => onChange(!checked)}
      className={cn(
        'relative inline-flex shrink-0 items-center rounded-full border-2 border-transparent transition-colors',
        'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background',
        sm ? 'h-5 w-9' : 'h-7 w-12',
        checked ? 'bg-primary' : 'bg-muted-foreground/30 dark:bg-muted-foreground/40',
        (disabled || busy) && 'pointer-events-none',
        disabled && !busy && 'opacity-50',
      )}
    >
      <span
        className={cn(
          'pointer-events-none flex items-center justify-center rounded-full bg-background shadow-sm ring-0 transition-transform',
          sm ? 'size-4' : 'size-5',
          checked ? (sm ? 'translate-x-[14px]' : 'translate-x-5') : 'translate-x-0.5',
        )}
      >
        {busy && <Spinner className="h-3 w-3 border-[1.5px] text-muted-foreground" />}
      </span>
    </button>
  )
}

interface AdminToggleRowProps {
  label: string
  hint?: string
  checked: boolean
  onChange: (checked: boolean) => void
  disabled?: boolean
  busy?: boolean
  badge?: string
}

export function AdminToggleRow({ label, hint, checked, onChange, disabled, busy, badge }: AdminToggleRowProps) {
  return (
    <div className="flex items-start justify-between gap-4 border-b border-border/50 py-3 last:border-0">
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-2">
          <span className="text-sm font-medium leading-snug text-foreground">{label}</span>
          {badge && (
            <span className="rounded-md bg-amber-500/15 px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide text-amber-700 dark:text-amber-400">
              {badge}
            </span>
          )}
        </div>
        {hint && <p className="mt-0.5 text-xs text-muted-foreground">{hint}</p>}
      </div>
      <AdminToggleSwitch checked={checked} onChange={onChange} disabled={disabled} busy={busy} aria-label={label} />
    </div>
  )
}
