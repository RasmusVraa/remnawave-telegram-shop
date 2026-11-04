import { Check } from 'lucide-react'
import type { ReactNode } from 'react'

import { cn } from '@/lib/utils'
import { Spinner } from '@/components/ui/spinner'

interface AdminCheckboxProps {
  checked: boolean
  onChange: (checked: boolean) => void
  disabled?: boolean
  /** Значение сохраняется на сервере: чекбокс заблокирован, внутри спиннер. */
  busy?: boolean
  id?: string
  className?: string
  'aria-label'?: string
}

export function AdminCheckbox({
  checked,
  onChange,
  disabled,
  busy = false,
  id,
  className,
  'aria-label': ariaLabel,
}: AdminCheckboxProps) {
  return (
    <button
      type="button"
      id={id}
      role="checkbox"
      aria-checked={checked}
      aria-label={ariaLabel}
      aria-busy={busy || undefined}
      disabled={disabled || busy}
      onClick={() => onChange(!checked)}
      className={cn(
        'inline-flex size-5 shrink-0 items-center justify-center rounded-md border-2 transition-colors',
        'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2',
        checked
          ? 'border-primary bg-primary text-primary-foreground'
          : 'border-border bg-background hover:border-primary/60',
        (disabled || busy) && 'pointer-events-none',
        disabled && !busy && 'opacity-50',
        className,
      )}
    >
      {busy ? (
        <Spinner className={cn('h-3 w-3 border-[1.5px]', !checked && 'text-muted-foreground')} />
      ) : (
        checked && <Check className="size-3.5" strokeWidth={3} />
      )}
    </button>
  )
}

interface AdminCheckboxFieldProps extends AdminCheckboxProps {
  label: ReactNode
  description?: ReactNode
}

export function AdminCheckboxField({
  label,
  description,
  className,
  ...props
}: AdminCheckboxFieldProps) {
  return (
    <label
      className={cn(
        'flex cursor-pointer items-start gap-3 rounded-lg transition-colors hover:bg-accent/40',
        props.disabled && !props.busy && 'cursor-not-allowed opacity-60',
        props.busy && 'cursor-wait',
        className,
      )}
    >
      <AdminCheckbox {...props} className="mt-0.5" />
      <span className="min-w-0 flex-1">
        <span className="block text-sm leading-snug">{label}</span>
        {description && (
          <span className="mt-0.5 block text-xs text-muted-foreground">{description}</span>
        )}
      </span>
    </label>
  )
}
