import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'
import { AdminCheckbox } from './AdminCheckbox'
import { AdminSelect } from './AdminSelect'
import { surface } from './Surface'
import { useAdminExternalSquads, useAdminSquads } from '../hooks/useAdminTariffs'

function parseList(raw: string): string[] {
  return raw
    .split(',')
    .map((s) => s.trim().toLowerCase())
    .filter(Boolean)
}

/**
 * Какие внутренние сквады получит пробный пользователь (TRIAL_INTERNAL_SQUADS).
 * Выглядит и работает как вкладка «Серверы» в редакторе тарифа. Значение — UUID
 * через запятую; пусто значит «как у остальных»: сквады из SQUAD_UUIDS в .env,
 * а если их там нет — все сквады панели. Сквады тарифов триал не использует.
 */
export function AdminTrialInternalSquadsEditor({
  value,
  disabled,
  onChange,
}: {
  value: string
  disabled?: boolean
  onChange: (next: string) => void
}) {
  const { t } = useTranslation()
  const { data, isLoading, isError } = useAdminSquads()
  const selected = parseList(value)
  const items = data?.items ?? []
  const known = items.map((s) => s.uuid.toLowerCase())
  const unknown = selected.filter((u) => !known.includes(u))

  const toggle = (uuid: string) => {
    const id = uuid.toLowerCase()
    const next = selected.includes(id) ? selected.filter((u) => u !== id) : [...selected, id]
    onChange(next.join(','))
  }

  return (
    <div className="space-y-2">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <span className="block text-sm font-medium leading-snug text-foreground">
          {t('admin.settings.fields.TRIAL_INTERNAL_SQUADS.label')}
        </span>
        {items.length > 0 && (
          <span className="flex items-center gap-2">
            <button
              type="button"
              disabled={disabled}
              onClick={() => onChange(items.map((s) => s.uuid).join(','))}
              className={surface(
                'raised',
                'rounded-md px-2 py-1 text-xs text-muted-foreground transition-colors hover:bg-accent hover:text-foreground',
              )}
            >
              {t('admin.settings.trialSquads.selectAll')}
            </button>
            {selected.length > 0 && (
              <button
                type="button"
                disabled={disabled}
                onClick={() => onChange('')}
                className={surface(
                  'raised',
                  'rounded-md px-2 py-1 text-xs text-muted-foreground transition-colors hover:bg-accent hover:text-foreground',
                )}
              >
                {t('admin.settings.trialSquads.clear')}
              </button>
            )}
          </span>
        )}
      </div>

      {isLoading ? (
        <p className="text-xs text-muted-foreground">{t('admin.settings.trialSquads.loading')}</p>
      ) : isError || items.length === 0 ? (
        <p className="text-xs text-amber-600 dark:text-amber-400">{t('admin.settings.trialSquads.noPanel')}</p>
      ) : (
        <div className="grid gap-2 sm:grid-cols-2">
          {items.map((sq) => {
            const checked = selected.includes(sq.uuid.toLowerCase())
            return (
              <label
                key={sq.uuid}
                className={cn(
                  'flex cursor-pointer items-center gap-2 rounded-lg px-3 py-2 text-sm transition-colors',
                  checked ? 'border border-primary/50 bg-primary/10' : surface('raised', 'hover:bg-accent/40'),
                )}
              >
                <AdminCheckbox checked={checked} disabled={disabled} onChange={() => toggle(sq.uuid)} aria-label={sq.name} />
                <span className="truncate">{sq.name}</span>
              </label>
            )
          })}
        </div>
      )}

      {!isLoading && items.length > 0 && (
        <p className="text-xs text-muted-foreground">
          {selected.length === 0
            ? t('admin.settings.trialSquads.noneSelected')
            : t('admin.settings.trialSquads.selected', { count: selected.length - unknown.length, total: items.length })}
        </p>
      )}
      <p className="text-xs text-muted-foreground">{t('admin.settings.fields.TRIAL_INTERNAL_SQUADS.hint')}</p>

      {unknown.length > 0 && (
        <div className="flex flex-wrap items-center gap-2 text-xs text-amber-600 dark:text-amber-400">
          <span>{t('admin.settings.trialSquads.unknown', { count: unknown.length })}</span>
          <button
            type="button"
            disabled={disabled}
            onClick={() => onChange(selected.filter((u) => known.includes(u)).join(','))}
            className={surface('raised', 'rounded-md px-2 py-1 transition-colors hover:bg-accent hover:text-foreground')}
          >
            {t('admin.settings.trialSquads.unknownDrop')}
          </button>
        </div>
      )}
    </div>
  )
}

/** Внешний сквад пробного периода (TRIAL_EXTERNAL_SQUAD_UUID). Пусто — как у платных. */
export function AdminTrialExternalSquadEditor({
  value,
  disabled,
  onChange,
}: {
  value: string
  disabled?: boolean
  onChange: (next: string) => void
}) {
  const { t } = useTranslation()
  const { data, isLoading, isError } = useAdminExternalSquads()
  const items = data?.items ?? []
  const current = value.trim().toLowerCase()
  // Сохранённого сквада может уже не быть в панели: показываем его, а не молча сбрасываем выбор.
  const missing = current !== '' && !items.some((s) => s.uuid.toLowerCase() === current)
  const options = [
    ...items.map((s) => ({ value: s.uuid, label: s.name })),
    ...(missing ? [{ value: value.trim(), label: t('admin.settings.trialSquads.externalMissing', { uuid: value.trim() }) }] : []),
  ]
  const label = t('admin.settings.fields.TRIAL_EXTERNAL_SQUAD_UUID.label')

  return (
    <div className="space-y-1.5">
      <label htmlFor="setting-TRIAL_EXTERNAL_SQUAD_UUID" className="block text-sm font-medium leading-snug text-foreground">
        {label}
      </label>
      <AdminSelect
        id="setting-TRIAL_EXTERNAL_SQUAD_UUID"
        value={value.trim() === '' ? null : value.trim()}
        options={options}
        onChange={(next) => onChange(next ?? '')}
        allowEmpty
        emptyLabel={t('admin.settings.trialSquads.externalDefault')}
        placeholder={t('admin.settings.trialSquads.externalDefault')}
        ariaLabel={label}
        disabled={disabled || isLoading}
        busy={isLoading}
      />
      <p className="text-xs text-muted-foreground">
        {isError ? t('admin.settings.trialSquads.noPanel') : t('admin.settings.fields.TRIAL_EXTERNAL_SQUAD_UUID.hint')}
      </p>
    </div>
  )
}
