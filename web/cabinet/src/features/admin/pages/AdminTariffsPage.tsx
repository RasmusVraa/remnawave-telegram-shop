import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Zap,
  Plus,
  Pencil,
  Trash2,
  Server,
  Smartphone,
  Gauge,
} from 'lucide-react'
import type { LucideIcon } from 'lucide-react'

import { AdminLayout } from '../layout/AdminLayout'
import { AdminPageHeader } from '../components/AdminPageHeader'
import { AdminFeedback } from '../components/AdminFeedback'
import { AdminTariffEditor } from '../components/AdminTariffEditor'
import { AdminConfirmModal } from '../components/AdminConfirmModal'
import { AdminToggleSwitch } from '../components/AdminToggleSwitch'
import { AdminProductSettingsPanel } from '../components/AdminProductSettingsPanel'
import { useAdminMutationFeedback } from '../hooks/useAdminMutationFeedback'
import { tariffTierAccent } from '../utils/tariffTierAccent'
import { TariffDescription } from '@/components/TariffDescription'
import { Card } from '@/components/ui/card'
import { cn } from '@/lib/utils'
import {
  useAdminTariffList,
  useAdminTariffCreate,
  useAdminTariffUpdate,
  useAdminTariffDelete,
  useAdminTariffSquadsApply,
  type AdminTariff,
  type AdminTariffPrice,
  type CreateTariffInput,
} from '../hooks/useAdminTariffs'

const GB = 1024 * 1024 * 1024

/** Периоды прайса — те же, что в редакторе тарифа. */
const PERIOD_MONTHS = [1, 3, 6, 12] as const

/** Безлимит показываем знаком «∞» без единиц: «∞ ГБ» — бессмыслица. */
function formatTrafficLimit(bytes: number, gbUnit: string): string {
  if (!bytes || bytes <= 0) return '∞'
  const gb = bytes / GB
  // Целые значения без «.0»: 200 ГБ, а не 200.0 ГБ.
  return `${Number.isInteger(gb) ? gb : gb.toFixed(1)} ${gbUnit}`
}

function formatRub(amount: number, locale: string): string {
  return `${new Intl.NumberFormat(locale).format(amount)} ₽`
}

function LimitChip({ icon: Icon, value, label }: { icon: LucideIcon; value: string; label: string }) {
  return (
    <span
      title={label}
      className="inline-flex items-center gap-1.5 rounded-lg border border-border/50 bg-muted/25 px-2.5 py-1.5"
    >
      <Icon className="size-3.5 shrink-0 text-muted-foreground" />
      <span className="text-sm font-semibold leading-none tabular-nums">{value}</span>
    </span>
  )
}

/** Прайс тарифа: то, ради чего страницу открывают, — раньше его тут не было вовсе. */
function TariffPrices({ prices, locale }: { prices: AdminTariffPrice[]; locale: string }) {
  const { t } = useTranslation()

  const byMonths = new Map(prices.map((p) => [p.months, p]))
  const shown = PERIOD_MONTHS.map((m) => byMonths.get(m)).filter((p): p is AdminTariffPrice => p != null)

  if (shown.length === 0) {
    return (
      <p className="rounded-xl border border-dashed border-border/60 px-3 py-2.5 text-center text-xs text-muted-foreground">
        {t('admin.tariffs.noPrices')}
      </p>
    )
  }

  return (
    <div className="flex gap-1.5 rounded-xl border border-border/40 bg-muted/15 p-1.5">
      {shown.map((price) => (
        <div key={price.months} className="min-w-0 flex-1 rounded-lg px-1.5 py-1.5 text-center">
          <p className="truncate text-[10px] uppercase tracking-wide text-muted-foreground">
            {t('admin.users.monthsShort', { count: price.months })}
          </p>
          <p className="mt-0.5 truncate text-sm font-semibold tabular-nums">
            {formatRub(price.amount_rub, locale)}
          </p>
          {price.amount_stars != null && price.amount_stars > 0 && (
            <p className="mt-0.5 truncate text-[10px] tabular-nums text-muted-foreground">
              ★ {price.amount_stars}
            </p>
          )}
        </div>
      ))}
    </div>
  )
}

function TariffCard({
  tariff,
  onEdit,
  locale,
  position,
}: {
  tariff: AdminTariff
  onEdit: () => void
  locale: string
  /** Позиция в отсортированном списке — она задаёт цвет карточки. */
  position: number
}) {
  const { t } = useTranslation()
  const update = useAdminTariffUpdate()
  const del = useAdminTariffDelete()
  const [deleteOpen, setDeleteOpen] = useState(false)

  const squadCount = tariff.active_internal_squad_uuids
    ? tariff.active_internal_squad_uuids.split(',').filter(Boolean).length
    : 0

  const title = tariff.name?.trim() || tariff.slug
  const accent = tariffTierAccent(position)
  const inactive = !tariff.is_active

  return (
    <Card
      className={cn(
        'cabinet-elevated-card relative flex h-full flex-col overflow-hidden transition-shadow hover:shadow-md',
        inactive && 'bg-muted/30',
      )}
    >
      {/* Полоса уровня: идентичность тарифа, не статус (статус — бейдж справа). */}
      <span
        aria-hidden
        className={cn('absolute inset-y-0 left-0 w-1', accent.bar, inactive && 'opacity-30')}
      />

      <div className="flex flex-1 flex-col p-5 pl-6">
        {/* Header */}
        <div className="flex items-start justify-between gap-3">
          <div className="flex min-w-0 flex-1 items-start gap-2.5">
            <div
              className={cn(
                'flex size-9 shrink-0 items-center justify-center rounded-lg',
                accent.iconBox,
                inactive && 'opacity-60',
              )}
            >
              <Zap className={cn('size-4', accent.iconColor)} />
            </div>
            <div className="min-w-0 flex-1">
              <h3
                className={cn(
                  'break-words text-base font-semibold leading-snug',
                  inactive && 'text-muted-foreground',
                )}
              >
                {title}
              </h3>
              {/* slug и порядок витрины — рабочие идентификаторы, раньше их было не видно. */}
              <p className="mt-0.5 truncate font-mono text-[11px] text-muted-foreground/70">
                {tariff.slug} · #{tariff.sort_order}
              </p>
              <div className="mt-1.5 flex flex-wrap items-center gap-1.5">
                <span
                  className={cn(
                    'rounded-full px-2 py-0.5 text-[11px] font-medium leading-none',
                    tariff.is_active
                      ? 'bg-emerald-500/15 text-emerald-600 dark:text-emerald-400'
                      : 'bg-muted text-muted-foreground',
                  )}
                >
                  {tariff.is_active ? t('admin.tariffs.active') : t('admin.promos.inactive')}
                </span>
                {tariff.tier_level != null && (
                  <span
                    className={cn(
                      'rounded-full px-2 py-0.5 text-[11px] font-medium leading-none',
                      accent.badge,
                    )}
                  >
                    {t('admin.tariffs.tierLevel')} {tariff.tier_level}
                  </span>
                )}
              </div>
            </div>
          </div>

          <div className="flex shrink-0 items-center gap-1 self-start">
            <AdminToggleSwitch
              checked={tariff.is_active}
              onChange={(next) => update.mutate({ id: tariff.id, fields: { is_active: next } })}
              busy={update.isPending}
              aria-label={t('admin.tariffs.toggleActive')}
            />
            <button
              type="button"
              onClick={onEdit}
              className="rounded-lg p-2 text-muted-foreground transition-colors hover:bg-muted/60 hover:text-foreground"
              title={t('admin.edit')}
            >
              <Pencil className="size-4" />
            </button>
            {/* Удаление приглушено до наведения: необратимое действие не должно весить столько же, сколько «изменить». */}
            <button
              type="button"
              onClick={() => setDeleteOpen(true)}
              className="rounded-lg p-2 text-muted-foreground/50 transition-colors hover:bg-destructive/10 hover:text-destructive"
              title={t('admin.delete')}
            >
              <Trash2 className="size-4" />
            </button>
          </div>
        </div>

        <AdminConfirmModal
          open={deleteOpen}
          onClose={() => setDeleteOpen(false)}
          onConfirm={() => {
            del.mutate(tariff.id, { onSuccess: () => setDeleteOpen(false) })
          }}
          title={t('admin.tariffs.deleteTariff')}
          message={t('admin.tariffs.confirmDelete', { name: title })}
          confirmLabel={t('admin.delete')}
          variant="destructive"
          loading={del.isPending}
          icon={Trash2}
          iconAccent="rose"
        />

        {tariff.description?.trim() && (
          <div className="mt-4 rounded-xl border border-border/40 bg-muted/10 px-3 py-2.5">
            <TariffDescription text={tariff.description} className="text-sm text-muted-foreground" />
          </div>
        )}

        {/*
         * mt-auto прижимает лимиты и цены к низу карточки: описания у тарифов
         * разной длины, и без этого данные вставали на разной высоте в соседних
         * колонках — именно отсюда бралась асимметрия сетки.
         */}
        <div className="mt-auto space-y-2.5 pt-4">
          <div className="flex flex-wrap gap-1.5">
            <LimitChip
              icon={Smartphone}
              value={String(tariff.device_limit)}
              label={t('admin.tariffs.devices')}
            />
            <LimitChip
              icon={Gauge}
              value={formatTrafficLimit(tariff.traffic_limit_bytes, t('admin.users.subscription.gbUnit'))}
              label={t('admin.tariffs.trafficShort')}
            />
            <LimitChip
              icon={Server}
              value={squadCount > 0 ? String(squadCount) : t('admin.tariffs.unlimited')}
              label={t('admin.tariffs.squadsShort')}
            />
          </div>

          <TariffPrices prices={tariff.prices} locale={locale} />
        </div>
      </div>
    </Card>
  )
}

export default function AdminTariffsPage() {
  const { t, i18n } = useTranslation()
  const locale = i18n.language?.startsWith('en') ? 'en-US' : 'ru-RU'
  const { data: tariffs, isLoading } = useAdminTariffList()
  const create = useAdminTariffCreate()
  const update = useAdminTariffUpdate()
  const applySquadsMutation = useAdminTariffSquadsApply()
  const { feedback, clear, showSuccess, showError } = useAdminMutationFeedback()

  const [editorOpen, setEditorOpen] = useState(false)
  const [editingTariff, setEditingTariff] = useState<AdminTariff | null>(null)

  const breadcrumbTail = editorOpen
    ? (editingTariff
      ? t('admin.breadcrumb.tariffEdit', { name: editingTariff.name ?? editingTariff.slug })
      : t('admin.breadcrumb.tariffCreate'))
    : undefined

  const openCreate = () => { setEditingTariff(null); setEditorOpen(true) }
  const openEdit = (tariff: AdminTariff) => { setEditingTariff(tariff); setEditorOpen(true) }
  const closeEditor = () => { setEditorOpen(false); setEditingTariff(null) }

  const handleSave = (
    data: CreateTariffInput | Record<string, unknown>,
    isEdit: boolean,
    applySquads?: { add: string[]; remove: string[] } | null,
  ) => {
    const savedMsg = t('admin.feedback.saved')
    if (isEdit && editingTariff) {
      const tariffId = editingTariff.id
      update.mutate(
        { id: tariffId, fields: data as Record<string, unknown> },
        {
          onSuccess: () => {
            showSuccess(savedMsg)
            closeEditor()
            // Применяем только после успешного сохранения: иначе в панель уехал
            // бы состав, которого в тарифе нет.
            if (applySquads && (applySquads.add.length > 0 || applySquads.remove.length > 0)) {
              applySquadsMutation.mutate(
                { id: tariffId, add: applySquads.add, remove: applySquads.remove },
                {
                  onSuccess: (run) =>
                    showSuccess(t('admin.tariffs.squadsApply.started', { count: run.total })),
                  onError: showError,
                },
              )
            }
          },
          onError: showError,
        },
      )
    } else {
      create.mutate(data as CreateTariffInput, {
        onSuccess: () => {
          showSuccess(savedMsg)
          closeEditor()
        },
        onError: showError,
      })
    }
  }

  return (
    <AdminLayout meta={{ breadcrumbTail }}>
      <AdminFeedback feedback={feedback} onDismiss={clear} />
      <div className="space-y-6">
        <AdminPageHeader
          icon={Zap}
          title={t('admin.tariffs.title')}
          subtitle={t('admin.tariffs.subtitle')}
          accent="emerald"
          actions={
            <button
              type="button"
              onClick={openCreate}
              className="inline-flex items-center gap-1.5 rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90"
            >
              <Plus className="size-4" />
              {t('admin.tariffs.create')}
            </button>
          }
        />

        {isLoading ? (
          <div className="py-12 text-center text-muted-foreground">{t('admin.loading')}</div>
        ) : !tariffs || tariffs.length === 0 ? (
          <Card className="border-dashed p-12 text-center">
            <Zap className="mx-auto mb-3 size-10 text-muted-foreground/50" />
            <p className="text-muted-foreground">{t('admin.tariffs.empty')}</p>
            <button type="button" onClick={openCreate} className="mt-4 text-sm text-primary hover:underline">
              {t('admin.tariffs.createFirst')}
            </button>
          </Card>
        ) : (
          <div className="grid items-stretch gap-4 lg:grid-cols-2">
            {tariffs.map((tariff, index) => (
              <TariffCard
                key={tariff.id}
                tariff={tariff}
                locale={locale}
                position={index}
                onEdit={() => openEdit(tariff)}
              />
            ))}
          </div>
        )}

        {/* Настройки продукта — прайс доп. устройств, триал, курс звёзд — рядом с прайсом тарифов. */}
        <AdminProductSettingsPanel />
      </div>

      <AdminTariffEditor
        open={editorOpen}
        onClose={closeEditor}
        tariff={editingTariff}
        onSave={handleSave}
        saving={create.isPending || update.isPending}
      />
    </AdminLayout>
  )
}
