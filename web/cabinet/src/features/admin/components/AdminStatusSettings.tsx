import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ChevronDown, CircleHelp, Loader2 } from 'lucide-react'

import { cn } from '@/lib/utils'
import { AdminFeedback } from './AdminFeedback'
import { AdminToggleRow } from './AdminToggleSwitch'
import { useAdminSettingsDraft } from '../hooks/useAdminSettingsDraft'
import { ADMIN_STATUS_SETTINGS_GROUP } from '../utils/adminSettingsGroups'

const inputClass =
  'h-10 w-full rounded-lg border border-border/60 bg-background px-3 text-sm outline-none transition-colors placeholder:text-muted-foreground/60 focus-visible:ring-2 focus-visible:ring-ring'

const NUMBER_KEYS = ['STATUS_WORLD_PROBES', 'STATUS_RUSSIA_PROBES', 'STATUS_PROBE_INTERVAL_MIN'] as const
const TEXT_KEYS = ['STATUS_PAGE_TITLE', 'STATUS_PAGE_LEAD'] as const
const ADVANCED_KEYS = [...TEXT_KEYS, ...NUMBER_KEYS]

/** Маленькая подсказка: значок «?», текст появляется при наведении или фокусе и не занимает места. */
function InfoHint({ text }: { text: string }) {
  return (
    <span className="group relative inline-flex">
      <button
        type="button"
        aria-label={text}
        className="rounded-full text-muted-foreground hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
      >
        <CircleHelp className="size-4" />
      </button>
      <span
        role="tooltip"
        className="pointer-events-none absolute left-0 top-full z-30 mt-1.5 w-64 rounded-lg border border-border bg-background px-3 py-2 text-xs font-normal leading-snug text-foreground opacity-0 shadow-lg transition-opacity group-hover:opacity-100 group-focus-within:opacity-100"
      >
        {text}
      </span>
    </span>
  )
}

/** Настройки страницы: два переключателя на виду, остальное под «Дополнительно». */
export function AdminStatusSettings() {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const { groups, isLoading, isError, draft, setDraftValue, togglingKey, isGroupSaving, handleToggle, handleSaveSection, feedback, clearFeedback } =
    useAdminSettingsDraft()

  const group = (groups ?? []).find((g) => g.id === ADMIN_STATUS_SETTINGS_GROUP)
  if (isLoading || isError || !group) return null
  const on = (key: string) => draft[key] !== 'false'
  const saving = isGroupSaving(group.id)

  return (
    <section className="rounded-2xl border border-border/60 bg-card/50 p-4 sm:p-5">
      <h2 className="text-base font-semibold tracking-tight">{t('admin.statusPage.pageTitle')}</h2>
      <div className="mt-1">
        <AdminToggleRow
          label={t('admin.statusPage.probesToggle')}
          hint={t('admin.statusPage.probesToggleHint')}
          checked={on('STATUS_PROBES_ENABLED')}
          busy={togglingKey === 'STATUS_PROBES_ENABLED'}
          onChange={(v) => void handleToggle('STATUS_PROBES_ENABLED', v)}
        />
        <AdminToggleRow
          label={t('admin.statusPage.mapToggle')}
          hint={t('admin.statusPage.mapToggleHint')}
          checked={on('STATUS_SHOW_MAP')}
          busy={togglingKey === 'STATUS_SHOW_MAP'}
          onChange={(v) => void handleToggle('STATUS_SHOW_MAP', v)}
        />
      </div>

      <div className="mt-2 rounded-xl border border-border/60">
        <button
          type="button"
          aria-expanded={open}
          onClick={() => setOpen((v) => !v)}
          className="flex w-full items-center justify-between px-3 py-2.5 text-sm font-medium"
        >
          {t('admin.statusPage.moreSettings')}
          <ChevronDown className={cn('size-4 text-muted-foreground transition-transform', open && 'rotate-180')} />
        </button>
        {open && (
          <div className="space-y-4 border-t border-border/60 p-3">
            {TEXT_KEYS.map((key) => (
              <label key={key} className="block">
                <span className="inline-flex items-center gap-1.5 text-sm font-medium">
                  {t(`admin.statusPage.${key}`)}
                  <InfoHint text={t('admin.statusPage.titleHint')} />
                </span>
                <input
                  className={cn(inputClass, 'mt-1.5')}
                  value={draft[key] ?? ''}
                  placeholder={t('admin.statusPage.defaultText')}
                  onChange={(e) => setDraftValue(key, e.target.value)}
                />
              </label>
            ))}
            <div className="grid items-end gap-3 sm:grid-cols-3">
              {NUMBER_KEYS.map((key) => (
                <label key={key} className="block">
                  <span className="text-sm font-medium">{t(`admin.statusPage.${key}`)}</span>
                  <input
                    className={cn(inputClass, 'mt-1.5')}
                    type="number"
                    value={draft[key] ?? ''}
                    onChange={(e) => setDraftValue(key, e.target.value)}
                  />
                </label>
              ))}
            </div>
            <p className="text-xs text-muted-foreground">{t('admin.statusPage.moreSettingsHint')}</p>
            <button
              type="button"
              disabled={saving}
              onClick={() => void handleSaveSection(group.id, ADVANCED_KEYS)}
              className="inline-flex min-h-10 items-center gap-2 rounded-lg bg-primary px-4 text-sm font-semibold text-primary-foreground transition-colors hover:bg-primary/90 active:bg-primary/75 disabled:opacity-60"
            >
              {saving && <Loader2 className="size-4 animate-spin" />}
              {t('admin.statusPage.save')}
            </button>
          </div>
        )}
      </div>
      <AdminFeedback feedback={feedback} onDismiss={clearFeedback} autoDismissMs={4000} />
    </section>
  )
}
