import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ChevronRight } from 'lucide-react'

import { Spinner } from '@/components/ui/spinner'
import { cn } from '@/lib/utils'
import { DecorThemePickerSheet } from '@/features/decor/DecorThemePickerSheet'
import { DECOR_PICKER_ACCENT } from '@/features/decor/decorThemePicker'
import { normalizeDecorTheme } from '@/features/decor/decorThemes'

import { surface } from './Surface'

/**
 * «Тема оформления» в админке: кнопка с текущей темой вместо длинного списка.
 * Выбор — в том же окне с плитками, что и у пользователя (без пункта «Как у сервиса»).
 * Тема применяется сразу (настройка instant), поэтому отдельной кнопки «Сохранить» нет.
 */
export function AdminDecorThemeField({
  id,
  label,
  hint,
  value,
  disabled,
  busy,
  onChange,
}: {
  id: string
  label: string
  hint?: string
  value: string
  disabled: boolean
  busy: boolean
  onChange: (next: string) => void
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const theme = normalizeDecorTheme(value)

  return (
    <div className="space-y-1.5 border-b border-border/50 py-3 last:border-0">
      <label htmlFor={id} className="block text-sm font-medium leading-snug text-foreground">
        {label}
      </label>
      <button
        id={id}
        type="button"
        disabled={disabled || busy}
        onClick={() => setOpen(true)}
        className={cn(
          // Тот же фон «на ступень светлее карточки», что у AdminSelect: токен, декор-темы его переопределяют.
          surface('raised'),
          'flex min-h-11 w-full items-center gap-3 rounded-[14px] px-4 py-3 text-left transition-colors hover:bg-accent/40 disabled:opacity-60',
        )}
      >
        <span
          className="size-[22px] shrink-0 rounded-full border border-white/20"
          style={{ background: DECOR_PICKER_ACCENT[theme] }}
          aria-hidden
        />
        <span className="min-w-0 flex-1 truncate text-base">
          {t(`admin.settings.enum.${theme}`, { defaultValue: theme })}
        </span>
        {busy ? (
          <Spinner />
        ) : (
          <span className="flex shrink-0 items-center gap-0.5 text-sm text-muted-foreground">
            {t('home.decorPicker.change')}
            <ChevronRight className="size-4" />
          </span>
        )}
      </button>
      {hint && <p className="text-xs text-muted-foreground">{hint}</p>}
      <DecorThemePickerSheet
        open={open}
        onClose={() => setOpen(false)}
        value={theme}
        includeAuto={false}
        onApply={(next) => {
          if (next !== 'auto') onChange(next)
        }}
      />
    </div>
  )
}
