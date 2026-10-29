import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Check,
  ChevronDown,
  Clock,
  CreditCard,
  FlaskConical,
  Gift,
  Moon,
  UserCheck,
  UserX,
  Users,
  type LucideIcon,
} from 'lucide-react'

import { cn } from '@/lib/utils'

interface AudienceItem {
  audience: string
  label: string
  count: number
}

interface BroadcastAudienceSelectorProps {
  audiences: AudienceItem[]
  isLoading: boolean
  selectedAudiences: string[]
  onChange: (audiences: string[]) => void
  audienceLabels: Record<string, string>
  /** Число людей без повторов — его считает сервер, сложить счётчики строк нельзя. */
  recipientCount: number | null
  /** Фильтр по тарифу — показывается под списком только для платных сегментов. */
  children?: React.ReactNode
}

const TEST_AUDIENCE = 'test_broadcast'

/**
 * Порядок и разбивка списка. Ключи те же, что отдаёт бэкенд.
 *
 * Раньше это были три сворачиваемых блока, и чтобы сравнить «активных
 * оплативших» с «неактивными», приходилось открывать один и закрывать другой.
 * Теперь это один список: подзаголовки разделяют, но ничего не прячут.
 */
const GROUPS: { titleKey: string; items: { key: string; icon: LucideIcon }[] }[] = [
  {
    titleKey: 'admin.broadcast.audienceGroup.common',
    items: [
      { key: 'all', icon: Users },
      { key: TEST_AUDIENCE, icon: FlaskConical },
    ],
  },
  {
    titleKey: 'admin.broadcast.audienceGroup.active',
    items: [
      { key: 'active_all', icon: UserCheck },
      { key: 'active_paid', icon: CreditCard },
      { key: 'active_trial', icon: Gift },
    ],
  },
  {
    titleKey: 'admin.broadcast.audienceGroup.inactive',
    items: [
      { key: 'inactive_all', icon: UserX },
      { key: 'inactive_paid', icon: Clock },
      { key: 'inactive_trial', icon: Moon },
    ],
  },
]

const ORDER = GROUPS.flatMap((group) => group.items.map((item) => item.key))

/**
 * Какой сегмент целиком включает данный. Та же таблица, что
 * broadcastAudienceParent на бэкенде: отмеченные «Все активные» уже содержат
 * оплативших и триал, поэтому те показываются отмеченными, но не выбираются.
 */
const PARENT: Record<string, string> = {
  active_all: 'all',
  inactive_all: 'all',
  active_paid: 'active_all',
  active_trial: 'active_all',
  inactive_paid: 'inactive_all',
  inactive_trial: 'inactive_all',
}

/** Отмеченный предок сегмента, если он есть. */
function broadcastAudienceCoveredBy(key: string, selected: readonly string[]): string | null {
  for (let parent = PARENT[key]; parent; parent = PARENT[parent]) {
    if (selected.includes(parent)) return parent
  }
  return null
}

/**
 * Новый выбор после клика по строке. Тест уходит только админу, поэтому он
 * не сочетается с остальными: его выбор снимает их, а их выбор снимает тест.
 * Отмеченный родитель забирает отмеченных потомков — иначе в чипах висели бы
 * «Все активные» и «Активные оплатившие», которые ничего друг к другу не добавляют.
 */
function toggleAudience(selected: readonly string[], key: string): string[] {
  if (selected.includes(key)) return selected.filter((aud) => aud !== key)
  if (key === TEST_AUDIENCE) return [TEST_AUDIENCE]
  const next = selected.filter(
    (aud) => aud !== TEST_AUDIENCE && broadcastAudienceCoveredBy(aud, [key]) === null,
  )
  return ORDER.filter((aud) => aud === key || next.includes(aud))
}

export function BroadcastAudienceSelector({
  audiences,
  isLoading,
  selectedAudiences,
  onChange,
  audienceLabels,
  recipientCount,
  children,
}: BroadcastAudienceSelectorProps) {
  const { t } = useTranslation()
  /*
   * Свёрнут по умолчанию: аудиторию выбирают один раз в начале, а дальше
   * работают с текстом. Развёрнутый список из восьми строк отодвигал бы
   * сообщение за нижний край экрана при каждом заходе.
   */
  const [open, setOpen] = useState(false)

  const byKey = new Map(audiences.map((a) => [a.audience, a]))
  const label = (key: string) => audienceLabels[key] ?? byKey.get(key)?.label ?? key
  const picked = ORDER.filter((key) => selectedAudiences.includes(key))
  const names =
    picked.length === 0
      ? t('admin.broadcast.audienceNone')
      : picked.length <= 2
        ? picked.map(label).join(' + ')
        : t('admin.broadcast.audienceAndMore', { first: label(picked[0]), count: picked.length - 1 })
  const summary =
    picked.length && recipientCount !== null ? `${names} · ${recipientCount}` : names

  return (
    <div className="overflow-hidden rounded-lg border border-border/50 bg-card">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        aria-expanded={open}
        className="flex min-h-12 w-full items-center gap-3 px-4 py-3 text-left transition-colors hover:bg-accent/40"
      >
        <span className="grid size-5 shrink-0 place-items-center rounded-md bg-muted font-mono text-[10px] text-muted-foreground">
          1
        </span>
        <span className="shrink-0 text-sm font-semibold">{t('admin.broadcast.audienceTitle')}</span>
        <span
          className={cn(
            'ml-auto min-w-0 truncate text-xs',
            picked.length ? 'text-muted-foreground' : 'font-semibold text-destructive',
          )}
        >
          {isLoading ? t('admin.broadcast.previewLoading') : summary}
        </span>
        <ChevronDown
          className={cn('size-4 shrink-0 text-muted-foreground transition-transform', open && 'rotate-180')}
        />
      </button>

      {open && (
        <div className="border-t border-border/50">
          {isLoading ? (
            <div className="flex justify-center py-5">
              <span className="size-5 animate-spin rounded-full border-2 border-primary border-t-transparent" />
            </div>
          ) : (
            <>
              <p className="px-4 pt-3 text-xs text-muted-foreground">
                {picked.includes(TEST_AUDIENCE)
                  ? t('admin.broadcast.audienceTestAlone')
                  : t('admin.broadcast.audienceMultiHint')}
              </p>
              <ul aria-label={t('admin.broadcast.audienceTitle')}>
                {GROUPS.map((group) => {
                  const rows = group.items.filter((item) => byKey.has(item.key))
                  if (!rows.length) return null
                  return (
                    <li key={group.titleKey}>
                      <p className="flex items-center gap-2 px-4 pb-1.5 pt-3 text-[11px] font-semibold uppercase tracking-[0.08em] text-muted-foreground">
                        {t(group.titleKey)}
                        <span className="h-px flex-1 bg-border" aria-hidden />
                      </p>
                      <ul>
                        {rows.map((item) => {
                          const data = byKey.get(item.key)!
                          const isSelected = picked.includes(item.key)
                          const coveredBy = isSelected ? null : broadcastAudienceCoveredBy(item.key, picked)
                          const checked = isSelected || coveredBy !== null
                          return (
                            <li key={item.key}>
                              <button
                                type="button"
                                role="checkbox"
                                aria-checked={checked}
                                aria-disabled={coveredBy !== null}
                                title={
                                  coveredBy
                                    ? t('admin.broadcast.audienceIncludedHint', { parent: label(coveredBy) })
                                    : undefined
                                }
                                onClick={() => {
                                  if (coveredBy) return
                                  onChange(toggleAudience(picked, item.key))
                                }}
                                className={cn(
                                  'flex w-full items-center gap-3 border-l-2 px-4 py-2.5 text-left transition-colors',
                                  isSelected
                                    ? 'border-primary bg-primary/10'
                                    : coveredBy
                                      ? 'cursor-default border-transparent text-muted-foreground'
                                      : 'border-transparent hover:bg-accent/40',
                                )}
                              >
                                <span
                                  className={cn(
                                    'grid size-4 shrink-0 place-items-center rounded-[5px] border-[1.5px]',
                                    isSelected
                                      ? 'border-primary bg-primary text-primary-foreground'
                                      : coveredBy
                                        ? 'border-muted bg-muted text-muted-foreground'
                                        : 'border-muted-foreground/50',
                                  )}
                                  aria-hidden
                                >
                                  {checked && <Check className="size-3" strokeWidth={3} />}
                                </span>
                                <item.icon
                                  className={cn(
                                    'size-4 shrink-0',
                                    isSelected ? 'text-primary' : 'text-muted-foreground',
                                  )}
                                />
                                <span
                                  className={cn('min-w-0 flex-1 truncate text-sm', isSelected && 'font-semibold')}
                                >
                                  {label(item.key)}
                                </span>
                                {coveredBy && (
                                  <span className="shrink-0 text-[11px] text-muted-foreground">
                                    {t('admin.broadcast.audienceIncluded')}
                                  </span>
                                )}
                                <span
                                  className={cn(
                                    'shrink-0 rounded-full px-2 py-0.5 text-xs tabular-nums',
                                    isSelected
                                      ? 'bg-primary/15 font-semibold text-primary'
                                      : 'bg-muted text-muted-foreground',
                                  )}
                                >
                                  {data.count}
                                </span>
                              </button>
                            </li>
                          )
                        })}
                      </ul>
                    </li>
                  )
                })}
              </ul>
            </>
          )}

          {children && <div className="border-t border-border/50 px-4 py-3">{children}</div>}
        </div>
      )}
    </div>
  )
}
