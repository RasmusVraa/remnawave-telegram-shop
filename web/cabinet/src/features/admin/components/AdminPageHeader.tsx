import type { LucideIcon } from 'lucide-react'
import type { ReactNode } from 'react'

import { cn } from '@/lib/utils'

interface AdminPageHeaderProps {
  icon: LucideIcon
  title: string
  subtitle?: string
  actions?: ReactNode
  /** Между заголовком и действиями (например, вкладки страницы). */
  center?: ReactNode
  accent?: 'primary' | 'emerald' | 'amber' | 'violet' | 'rose' | 'blue' | 'indigo' | 'slate' | 'cyan'
}

const accentStyles = {
  primary: 'bg-primary/10 text-primary dark:bg-primary/20',
  emerald: 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400',
  amber: 'bg-amber-500/10 text-amber-600 dark:text-amber-400',
  violet: 'bg-violet-500/10 text-violet-600 dark:text-violet-400',
  rose: 'bg-rose-500/10 text-rose-600 dark:text-rose-400',
  blue: 'bg-blue-500/10 text-blue-600 dark:text-blue-400',
  indigo: 'bg-indigo-500/10 text-indigo-600 dark:text-indigo-400',
  slate: 'bg-slate-500/10 text-slate-600 dark:text-slate-400',
  cyan: 'bg-cyan-500/10 text-cyan-600 dark:text-cyan-400',
}

export function AdminPageHeader({
  icon: Icon,
  title,
  subtitle,
  actions,
  center,
  accent = 'primary',
}: AdminPageHeaderProps) {
  return (
    <div className="flex flex-wrap items-start justify-between gap-4">
      <div className="flex items-center gap-3">
        <div className={cn('flex size-11 items-center justify-center rounded-xl', accentStyles[accent])}>
          <Icon className="size-5" />
        </div>
        <div>
          <h1 className="text-xl font-semibold tracking-tight">{title}</h1>
          {subtitle && <p className="text-sm text-muted-foreground">{subtitle}</p>}
        </div>
      </div>
      {center && <div className="flex min-w-0 flex-1 items-center justify-center self-center">{center}</div>}
      {actions && <div className="flex flex-wrap items-center gap-2">{actions}</div>}
    </div>
  )
}
