import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'
import type { AdminSettingGroupDTO } from '@/lib/types/admin'
import { AdminToggleRow } from './AdminToggleSwitch'

const ENABLED_KEY = 'REVIEWS_ENABLED'
const WHO_KEY = 'REVIEWS_WRITE_POLICY'
const PUBLISH_KEY = 'REVIEWS_PUBLISH_MODE'
const LANDING_KEY = 'REVIEWS_LANDING_LIMIT'

const WHO = ['verified', 'trial_paid', 'paid'] as const
const PUBLISH = ['instant', 'moderated'] as const

interface Props {
  group: AdminSettingGroupDTO
  draft: Record<string, string>
  saving: boolean
  togglingKey: string | null
  onDraftChange: (key: string, value: string) => void
  onToggle: (key: string, checked: boolean) => void
}

function RadioCards<T extends string>({
  name,
  options,
  value,
  disabled,
  onPick,
  label,
}: {
  name: string
  options: readonly T[]
  value: T
  disabled: boolean
  onPick: (v: T) => void
  label: (v: T) => { title: string; text: string }
}) {
  return (
    <div role="radiogroup" className="space-y-2">
      {options.map((opt) => {
        const on = value === opt
        const { title, text } = label(opt)
        return (
          <button
            key={opt}
            type="button"
            role="radio"
            aria-checked={on}
            name={name}
            disabled={disabled}
            onClick={() => onPick(opt)}
            className={cn(
              'flex w-full items-start gap-3 rounded-xl border px-3.5 py-3 text-left transition-colors disabled:opacity-60',
              on ? 'border-primary/50 bg-primary/[0.07]' : 'border-border/70 hover:bg-accent/40',
            )}
          >
            <span
              className={cn(
                'mt-0.5 flex size-[18px] shrink-0 items-center justify-center rounded-full border-2',
                on ? 'border-primary' : 'border-muted-foreground/50',
              )}
              aria-hidden
            >
              {on && <span className="size-2 rounded-full bg-primary" />}
            </span>
            <span className="min-w-0">
              <span className="block text-sm font-medium">{title}</span>
              <span className="mt-0.5 block text-xs leading-relaxed text-muted-foreground">{text}</span>
            </span>
          </button>
        )
      })}
    </div>
  )
}

/**
 * Группа «Отзывы» в настройках бота: включатель (сохраняется сразу), кто может
 * писать, как публиковать и сколько отзывов показывать на лендинге (эти три —
 * по кнопке «Сохранить секцию»).
 */
export function AdminReviewsSettings({ group, draft, saving, togglingKey, onDraftChange, onToggle }: Props) {
  const { t } = useTranslation()
  const stored = (key: string) => group.fields.find((f) => f.key === key)?.value ?? ''
  const read = (key: string) => (draft[key] ?? stored(key)).trim().toLowerCase()

  const enabled = read(ENABLED_KEY) === 'true'
  const who = (WHO as readonly string[]).includes(read(WHO_KEY)) ? (read(WHO_KEY) as (typeof WHO)[number]) : 'trial_paid'
  const publish = read(PUBLISH_KEY) === 'instant' ? 'instant' : 'moderated'
  const landing = draft[LANDING_KEY] ?? (stored(LANDING_KEY) || '6')

  return (
    <div className="mb-4 space-y-4">
      <div className="rounded-lg border border-border/60 bg-card/80 px-3 shadow-sm">
        <AdminToggleRow
          label={t('admin.settings.reviews.enabledLabel')}
          hint={t('admin.settings.reviews.enabledHint')}
          checked={enabled}
          busy={togglingKey === ENABLED_KEY}
          disabled={saving}
          onChange={(checked) => onToggle(ENABLED_KEY, checked)}
        />
      </div>

      <div className="rounded-lg border border-border/60 bg-card/80 p-3 shadow-sm">
        <p className="text-sm font-medium">{t('admin.settings.reviews.whoTitle')}</p>
        <p className="mb-2.5 mt-0.5 text-xs text-muted-foreground">{t('admin.settings.reviews.whoHint')}</p>
        <RadioCards
          name={WHO_KEY}
          options={WHO}
          value={who}
          disabled={saving}
          onPick={(v) => onDraftChange(WHO_KEY, v)}
          label={(v) => ({ title: t(`admin.settings.reviews.who.${v}.title`), text: t(`admin.settings.reviews.who.${v}.text`) })}
        />
      </div>

      <div className="rounded-lg border border-border/60 bg-card/80 p-3 shadow-sm">
        <p className="mb-2.5 text-sm font-medium">{t('admin.settings.reviews.publishTitle')}</p>
        <RadioCards
          name={PUBLISH_KEY}
          options={PUBLISH}
          value={publish}
          disabled={saving}
          onPick={(v) => onDraftChange(PUBLISH_KEY, v)}
          label={(v) => ({ title: t(`admin.settings.reviews.publish.${v}.title`), text: t(`admin.settings.reviews.publish.${v}.text`) })}
        />
      </div>

      <div className="rounded-lg border border-border/60 bg-card/80 p-3 shadow-sm">
        <label htmlFor={`setting-${LANDING_KEY}`} className="block text-sm font-medium">
          {t('admin.settings.reviews.landingLabel')}
        </label>
        <p className="mb-2 mt-0.5 text-xs text-muted-foreground">{t('admin.settings.reviews.landingHint')}</p>
        <input
          id={`setting-${LANDING_KEY}`}
          type="number"
          inputMode="numeric"
          min={0}
          max={12}
          value={landing}
          disabled={saving}
          onChange={(e) => onDraftChange(LANDING_KEY, e.target.value)}
          className="admin-input w-28 rounded-lg border border-border bg-card px-3 py-2 text-sm"
        />
      </div>
    </div>
  )
}
