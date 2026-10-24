import { useState, useEffect, useRef, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Save,
  Loader2,
  Pencil,
  Plus,
  Settings,
  Gauge,
  Server,
  Coins,
  FileText,
  MapPin,
  Trash2,
  Smartphone,
  HardDrive,
  RotateCcw,
  ListOrdered,
  Sparkles,
  AlertTriangle,
  Eraser,
  type LucideIcon,
} from 'lucide-react'

import { cn } from '@/lib/utils'
import { surface } from './Surface'
import { AdminModal } from './AdminModal'
import { AdminConfirmModal } from './AdminConfirmModal'
import { TariffDescriptionEditor } from './TariffDescriptionEditor'
import { slugifyTariffName } from '../utils/slugifyTariffName'
import { AdminCheckbox, AdminCheckboxField } from './AdminCheckbox'
import {
  adminSectionIconBoxClass,
  adminSectionIconAccentClassNames,
  type AdminSectionIconAccent,
} from '../utils/adminSectionIconAccents'
import { rubPerStarFromSettings, starsFromRub, savingsPercent } from '../utils/tariffStarsPricing'
import { useAdminBotSettings } from '../hooks/useAdminBotSettings'
import {
  useAdminSquads,
  useAdminTariffSquads,
  STRATEGIES,
  type AdminTariff,
  type AdminTariffLocation,
  type CreateTariffInput,
} from '../hooks/useAdminTariffs'
import {
  AdminTariffSquadsApplyPanel,
  AdminTariffSquadsSaveDialog,
} from './AdminTariffSquadsApply'

const GB = 1024 * 1024 * 1024
const PERIOD_MONTHS = [1, 3, 6, 12] as const

const DEVICE_PRESETS = [1, 2, 3, 5] as const
// 0 — безлимит: traffic_limit_bytes = 0 панель трактует как «без ограничения».
const TRAFFIC_PRESETS = [0, 100, 200, 500, 1000] as const

const STRATEGY_I18N_KEYS: Record<string, string> = {
  no_reset: 'admin.tariffs.strategies.noReset',
  DAY: 'admin.tariffs.strategies.day',
  WEEK: 'admin.tariffs.strategies.week',
  MONTH: 'admin.tariffs.strategies.month',
  MONTH_ROLLING: 'admin.tariffs.strategies.monthRolling',
  NO_RESET: 'admin.tariffs.strategies.noResetRw',
}

function parseSquadUUIDs(raw: string): string[] {
  if (!raw.trim()) return []
  return raw.split(',').map((s) => s.trim()).filter(Boolean)
}

function joinSquadUUIDs(uuids: string[]): string {
  return uuids.join(',')
}

function TariffEditorSectionHeader({
  icon: Icon,
  accent,
  children,
}: {
  icon: LucideIcon
  accent: AdminSectionIconAccent
  children: ReactNode
}) {
  const { iconClassName } = adminSectionIconAccentClassNames(accent)
  return (
    <h3 className="mb-3 flex items-center gap-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
      <span className={adminSectionIconBoxClass(accent, 'size-7')}>
        <Icon className={cn('size-3.5', iconClassName)} />
      </span>
      {children}
    </h3>
  )
}

function TariffFieldLabel({ icon: Icon, children }: { icon?: LucideIcon; children: ReactNode }) {
  return (
    <label className="mb-1 flex items-center gap-1.5 text-xs font-medium">
      {Icon && <Icon className="size-3.5 shrink-0 text-muted-foreground" aria-hidden />}
      {children}
    </label>
  )
}

/** Быстрый выбор частых значений — чтобы не набирать числа руками. */
function PresetChips({
  values,
  active,
  onSelect,
  renderLabel,
}: {
  values: readonly number[]
  active: number
  onSelect: (value: number) => void
  renderLabel: (value: number) => string
}) {
  return (
    <div className="mt-1.5 flex flex-wrap gap-1">
      {values.map((value) => (
        <button
          key={value}
          type="button"
          onClick={() => onSelect(value)}
          className={cn(
            'rounded-md border px-2 py-0.5 text-xs transition-colors',
            active === value
              ? 'border-primary/50 bg-primary/10 text-foreground'
              : 'border-border/60 text-muted-foreground hover:bg-accent hover:text-foreground',
          )}
        >
          {renderLabel(value)}
        </button>
      ))}
    </div>
  )
}

type TariffEditorTabId = 'basic' | 'traffic' | 'servers' | 'locations' | 'prices' | 'description'

const TARIFF_EDITOR_TABS: {
  id: TariffEditorTabId
  labelKey: string
  icon: LucideIcon
  accent: AdminSectionIconAccent
}[] = [
  { id: 'basic', labelKey: 'admin.tariffs.tabBasic', icon: Settings, accent: 'slate' },
  { id: 'traffic', labelKey: 'admin.tariffs.tabTraffic', icon: Gauge, accent: 'blue' },
  { id: 'servers', labelKey: 'admin.tariffs.tabServers', icon: Server, accent: 'indigo' },
  { id: 'locations', labelKey: 'admin.tariffs.tabLocations', icon: MapPin, accent: 'emerald' },
  { id: 'prices', labelKey: 'admin.tariffs.tabPrices', icon: Coins, accent: 'amber' },
  { id: 'description', labelKey: 'admin.tariffs.tabDescription', icon: FileText, accent: 'violet' },
]

function TariffEditorTabNav({
  tabs,
  activeId,
  onSelect,
  errorTabs,
}: {
  tabs: typeof TARIFF_EDITOR_TABS
  activeId: TariffEditorTabId
  onSelect: (id: TariffEditorTabId) => void
  errorTabs: Set<TariffEditorTabId>
}) {
  const { t } = useTranslation()
  return (
    <div
      role="tablist"
      aria-label={t('admin.tariffs.editTitle')}
      className="sticky top-0 z-10 border-b border-border/70 bg-muted/25 px-3 py-2 backdrop-blur-sm dark:bg-secondary/25 sm:px-5"
    >
      <div className="-mx-1 overflow-x-auto overscroll-x-contain px-1">
        <div className="inline-flex min-w-full gap-1 rounded-lg border border-border/50 bg-card/50 p-1 sm:min-w-0 sm:w-full">
          {tabs.map((tab) => {
            const Icon = tab.icon
            const isActive = activeId === tab.id
            const hasError = errorTabs.has(tab.id)
            const { boxClassName, iconClassName } = adminSectionIconAccentClassNames(tab.accent)
            return (
              <button
                key={tab.id}
                type="button"
                role="tab"
                aria-selected={isActive}
                tabIndex={isActive ? 0 : -1}
                onClick={() => onSelect(tab.id)}
                className={cn(
                  'relative inline-flex min-h-9 shrink-0 items-center justify-center gap-1.5 rounded-md px-3 py-2 text-center text-xs font-medium transition-colors sm:flex-1',
                  isActive ? cn(boxClassName, iconClassName) : 'text-foreground/80 hover:bg-accent hover:text-foreground',
                )}
              >
                <Icon className={cn('size-3.5 shrink-0', isActive ? iconClassName : undefined)} aria-hidden />
                <span className="truncate leading-tight">{t(tab.labelKey)}</span>
                {hasError && (
                  <span
                    className="absolute right-1 top-1 size-1.5 rounded-full bg-rose-500"
                    aria-label={t('admin.tariffs.validation.tabHasErrors')}
                  />
                )}
              </button>
            )
          })}
        </div>
      </div>
    </div>
  )
}

export interface TariffFormData {
  slug: string
  name: string
  sort_order: number
  is_active: boolean
  device_limit: number
  traffic_gb: number
  traffic_limit_reset_strategy: string
  squad_uuids: string[]
  description: string
  description_detail: string
  rub: [number, number, number, number]
  stars: [number | null, number | null, number | null, number | null]
  locations: AdminTariffLocation[]
}

type TariffValidationErrors = Partial<Record<TariffEditorTabId, string[]>>

/**
 * Бэкенд обязательным считает только slug (он выводится из названия), поэтому
 * здесь проверяем лишь бесспорные ошибки данных — не выдумывая бизнес-правил.
 */
function validateTariffForm(form: TariffFormData, hasSquads: boolean): TariffValidationErrors {
  const errors: TariffValidationErrors = {}
  const push = (tab: TariffEditorTabId, key: string) => {
    ;(errors[tab] ??= []).push(key)
  }

  if (!form.name.trim()) push('basic', 'admin.tariffs.validation.nameRequired')
  /*
   * Пустой список сквадов в БД означает «все сквады панели», а не «ни одного»
   * (filterSquadsByUUIDList в remnawave/tariff_profile.go). Кнопка «Очистить»
   * рядом с чекбоксами читалась ровно наоборот, поэтому состав выбирается явно.
   */
  if (hasSquads && form.squad_uuids.length === 0) {
    push('servers', 'admin.tariffs.validation.squadsRequired')
  }
  if (!Number.isFinite(form.device_limit) || form.device_limit < 1) {
    push('traffic', 'admin.tariffs.validation.devicesMin')
  }
  if (!Number.isFinite(form.traffic_gb) || form.traffic_gb < 0) {
    push('traffic', 'admin.tariffs.validation.trafficNegative')
  }
  if (form.rub.some((v) => !Number.isFinite(v) || v < 0)) {
    push('prices', 'admin.tariffs.validation.priceNegative')
  }
  if (form.stars.some((v) => v != null && (!Number.isFinite(v) || v < 0))) {
    push('prices', 'admin.tariffs.validation.starsNegative')
  }
  return errors
}

function tariffToForm(t?: AdminTariff | null): TariffFormData {
  const rub: [number, number, number, number] = [0, 0, 0, 0]
  const stars: [number | null, number | null, number | null, number | null] = [null, null, null, null]
  if (t?.prices) {
    for (const p of t.prices) {
      const idx = PERIOD_MONTHS.indexOf(p.months as 1 | 3 | 6 | 12)
      if (idx >= 0) {
        rub[idx] = p.amount_rub
        stars[idx] = p.amount_stars ?? null
      }
    }
  }
  return {
    slug: t?.slug ?? '',
    name: t?.name ?? '',
    sort_order: t?.sort_order ?? 0,
    is_active: t?.is_active ?? true,
    device_limit: t?.device_limit ?? 1,
    traffic_gb: t ? t.traffic_limit_bytes / GB : 0,
    traffic_limit_reset_strategy: t?.traffic_limit_reset_strategy ?? 'no_reset',
    squad_uuids: parseSquadUUIDs(t?.active_internal_squad_uuids ?? ''),
    description: t?.description ?? '',
    description_detail: t?.description_detail ?? '',
    rub,
    stars,
    locations: (t?.locations ?? []).map((loc) => ({
      id: loc.id,
      name: loc.name,
      sort_order: loc.sort_order,
      sold_out: loc.sold_out,
      country_code: loc.country_code ?? '',
    })),
  }
}

/**
 * tier_level всегда равен «Порядку».
 *
 * Отдельного поля «Уровень» в форме нет: это два почти всегда одинаковых числа.
 * Раньше при редактировании уровень сохранялся из исходного тарифа и не следовал
 * за изменением порядка — бейдж «Уровень N» и цвет карточки застревали на
 * значении, которое было при создании. TG-админка давно пишет оба поля разом
 * (см. tariff_admin.go, UpdateTariff с tier_level и sort_order), веб просто отстал.
 */
function formToCreateInput(f: TariffFormData): CreateTariffInput {
  const slug = f.slug.trim() || slugifyTariffName(f.name)
  return {
    slug,
    name: f.name.trim() || null,
    sort_order: f.sort_order,
    is_active: f.is_active,
    device_limit: f.device_limit,
    traffic_limit_bytes: Math.round(f.traffic_gb * GB),
    traffic_limit_reset_strategy: f.traffic_limit_reset_strategy,
    active_internal_squad_uuids: joinSquadUUIDs(f.squad_uuids),
    tier_level: f.sort_order,
    description: f.description.trim() || null,
    description_detail: f.description_detail.trim() || null,
    rub: f.rub,
    stars: f.stars,
    locations: f.locations
      .map((loc, index) => ({
        id: loc.id,
        name: loc.name.trim(),
        sort_order: index,
        sold_out: loc.sold_out,
        country_code: (loc.country_code ?? '').trim().toUpperCase() || null,
      }))
      .filter((loc) => loc.name !== ''),
  }
}

function formToUpdateFields(f: TariffFormData): Record<string, unknown> {
  const input = formToCreateInput(f)
  const fields: Record<string, unknown> = {
    name: input.name,
    sort_order: input.sort_order,
    is_active: input.is_active,
    device_limit: input.device_limit,
    traffic_limit_bytes: input.traffic_limit_bytes,
    traffic_limit_reset_strategy: input.traffic_limit_reset_strategy,
    active_internal_squad_uuids: input.active_internal_squad_uuids,
    tier_level: input.tier_level,
    description: input.description,
    description_detail: input.description_detail,
    rub: input.rub,
    stars: input.stars,
    locations: input.locations,
  }
  // slug намеренно не отправляем: он идентификатор тарифа и при редактировании
  // не меняется (раньше это выражалось условием `if (!original)`, которое в
  // пути редактирования никогда не выполнялось).
  return fields
}

interface Props {
  open: boolean
  onClose: () => void
  tariff?: AdminTariff | null
  onSave: (
    data: CreateTariffInput | Record<string, unknown>,
    isEdit: boolean,
    /** Дифф состава сквадов, который надо применить к действующим подписчикам. */
    applySquads?: { add: string[]; remove: string[] } | null,
  ) => void
  saving?: boolean
}

export function AdminTariffEditor({ open, onClose, tariff, onSave, saving }: Props) {
  const { t } = useTranslation()
  const { data: squadsData } = useAdminSquads()
  const { data: botSettings } = useAdminBotSettings()
  const [form, setForm] = useState<TariffFormData>(() => tariffToForm(tariff))
  const [confirmCloseOpen, setConfirmCloseOpen] = useState(false)
  const isEdit = tariff != null
  const hasSquads = Boolean(squadsData && squadsData.items.length > 0)
  const tabs = hasSquads ? TARIFF_EDITOR_TABS : TARIFF_EDITOR_TABS.filter((tab) => tab.id !== 'servers')
  const [activeTab, setActiveTab] = useState<TariffEditorTabId>('basic')

  const rubPerStar = rubPerStarFromSettings(botSettings)
  const errors = validateTariffForm(form, hasSquads)
  const errorTabs = new Set(Object.keys(errors) as TariffEditorTabId[])
  const activeTabErrors = errors[activeTab] ?? []
  const hasErrors = errorTabs.size > 0

  // Снимок исходной формы — по нему определяем несохранённые правки.
  const initialSnapshot = useRef('')
  /*
   * Редакторы описания неуправляемые (contenteditable с управляемым значением
   * роняет каретку на каждом вводе), поэтому перечитывают текст по смене
   * ключа — в тот же момент, когда сбрасывается форма.
   */
  const [descriptionResetKey, setDescriptionResetKey] = useState(0)
  useEffect(() => {
    if (!open) return
    const initial = tariffToForm(tariff)
    setForm(initial)
    setActiveTab('basic')
    setConfirmCloseOpen(false)
    setDescriptionResetKey((v) => v + 1)
    initialSnapshot.current = JSON.stringify(initial)
  }, [open, tariff?.id])

  const isDirty = JSON.stringify(form) !== initialSnapshot.current

  const set = <K extends keyof TariffFormData>(k: K, v: TariffFormData[K]) =>
    setForm((p) => ({ ...p, [k]: v }))

  const toggleSquad = (uuid: string) => {
    setForm((p) => ({
      ...p,
      squad_uuids: p.squad_uuids.includes(uuid)
        ? p.squad_uuids.filter((u) => u !== uuid)
        : [...p.squad_uuids, uuid],
    }))
  }

  /** Замораживает расчётные звёзды в полях — если нужна «красивая» цена вместо расчёта. */
  const fillStarsByRate = () => {
    setForm((p) => ({
      ...p,
      stars: p.rub.map((rub) => starsFromRub(rub, rubPerStar) ?? null) as TariffFormData['stars'],
    }))
  }

  const clearStars = () => set('stars', [null, null, null, null])

  /*
   * Дифф состава относительно сохранённого тарифа. Исходный пустой список
   * разворачивается во все сквады панели — иначе «пусто -> выбрал два»
   * выглядело бы как добавление, хотя на деле это снятие остальных.
   */
  const originalSquads =
    tariff == null
      ? []
      : (() => {
          const parsed = parseSquadUUIDs(tariff.active_internal_squad_uuids ?? '')
          return parsed.length > 0 ? parsed : (squadsData?.items ?? []).map((s) => s.uuid)
        })()
  /*
   * UUID, которых нет в панели. Тариф мог быть заведён из SQUAD_UUIDS мимо
   * панели (tariff_admin.go) или сквад пересоздали с новым UUID. Галочкой такой
   * сквад не показать — он не в списке панели, — поэтому он молча ехал обратно
   * при каждом сохранении. Показываем его явно и даём убрать.
   */
  const knownSquadUuids = squadsData?.items.map((sq) => sq.uuid) ?? []
  const unknownSquads = hasSquads ? form.squad_uuids.filter((u) => !knownSquadUuids.includes(u)) : []
  const addedSquads = form.squad_uuids.filter((u) => !originalSquads.includes(u))
  const removedSquads = originalSquads.filter((u) => !form.squad_uuids.includes(u))
  const squadsChanged = addedSquads.length > 0 || removedSquads.length > 0

  const { data: squadsApplyData } = useAdminTariffSquads(open && isEdit ? (tariff?.id ?? null) : null)
  const activeOnTariff = squadsApplyData?.active_count ?? 0
  const [applyDialogOpen, setApplyDialogOpen] = useState(false)

  const submit = (applySquads: { add: string[]; remove: string[] } | null) => {
    if (isEdit) {
      onSave(formToUpdateFields(form), true, applySquads)
    } else {
      onSave(formToCreateInput(form), false)
    }
  }

  const handleSave = () => {
    if (hasErrors) return
    // Спрашиваем только когда выбор реально что-то меняет: состав изменился
    // и на тарифе есть кому его применять.
    if (isEdit && squadsChanged && activeOnTariff > 0) {
      setApplyDialogOpen(true)
      return
    }
    submit(null)
  }

  const handleRequestClose = () => {
    if (isDirty) {
      setConfirmCloseOpen(true)
      return
    }
    onClose()
  }

  const previewSlug = form.slug.trim() || slugifyTariffName(form.name)
  const hasAnyStars = form.stars.some((v) => v != null)

  return (
    <>
      <AdminModal
        open={open}
        onClose={handleRequestClose}
        title={isEdit ? t('admin.tariffs.editTitle') : t('admin.tariffs.createTitle')}
        panelClassName="sm:max-w-2xl"
        icon={isEdit ? Pencil : Plus}
        iconAccent="emerald"
        footer={
          <div className="flex items-center justify-end gap-2">
            {isDirty && (
              <span className="mr-auto hidden text-xs text-muted-foreground sm:inline">
                {t('admin.tariffs.unsavedIndicator')}
              </span>
            )}
            {/* Отмена нейтральная: акцентная подсветка читалась как «вот
                основное действие», хотя рядом стоит «Сохранить». */}
            <button
              type="button"
              onClick={handleRequestClose}
              className="rounded-lg border border-border px-4 py-2 text-sm text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
            >
              {t('admin.cancel')}
            </button>
            <button
              type="button"
              onClick={handleSave}
              disabled={saving || hasErrors}
              className="inline-flex items-center gap-2 rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground disabled:opacity-50"
            >
              {saving ? <Loader2 className="size-4 animate-spin" /> : <Save className="size-4" />}
              {t('admin.save')}
            </button>
          </div>
        }
        bodyClassName="p-0"
      >
        <TariffEditorTabNav
          tabs={tabs}
          activeId={activeTab}
          onSelect={setActiveTab}
          errorTabs={errorTabs}
        />
        <div className="space-y-5 p-5">
          {activeTabErrors.length > 0 && (
            <div className="flex items-start gap-2 rounded-lg border border-rose-500/30 bg-rose-500/5 px-3 py-2 text-xs text-rose-600 dark:text-rose-400">
              <AlertTriangle className="mt-0.5 size-3.5 shrink-0" aria-hidden />
              <ul className="space-y-0.5">
                {activeTabErrors.map((key) => (
                  <li key={key}>{t(key)}</li>
                ))}
              </ul>
            </div>
          )}

          {activeTab === 'basic' && (
            <section className="space-y-3">
              <div>
                <TariffFieldLabel>{t('admin.tariffs.name')}</TariffFieldLabel>
                <input
                  className="admin-input w-full px-3 py-2"
                  value={form.name}
                  onChange={(e) => set('name', e.target.value)}
                />
                {previewSlug && (
                  <p className="mt-1 text-xs text-muted-foreground">
                    {t('admin.tariffs.slugPreview')}{' '}
                    <code className="rounded bg-muted px-1 py-0.5 font-mono text-[0.7rem]">{previewSlug}</code>
                    {isEdit && <span className="ml-1">{t('admin.tariffs.slugImmutable')}</span>}
                  </p>
                )}
              </div>
              <div>
                <TariffFieldLabel icon={ListOrdered}>{t('admin.tariffs.sortOrder')}</TariffFieldLabel>
                <input
                  type="number"
                  className="admin-input w-full max-w-xs px-3 py-2"
                  value={form.sort_order}
                  onChange={(e) => set('sort_order', Number(e.target.value))}
                />
                <p className="mt-1 text-xs text-muted-foreground">{t('admin.tariffs.sortOrderHint')}</p>
              </div>
              <AdminCheckboxField
                checked={form.is_active}
                onChange={(v) => set('is_active', v)}
                label={t('admin.tariffs.active')}
                description={t('admin.tariffs.activeHint')}
                className="border border-border/60 p-3"
              />
            </section>
          )}

          {activeTab === 'traffic' && (
            <section className="grid gap-4 sm:grid-cols-2">
              <div>
                <TariffFieldLabel icon={Smartphone}>{t('admin.tariffs.devices')}</TariffFieldLabel>
                <input
                  type="number"
                  min={1}
                  className="admin-input w-full px-3 py-2"
                  value={form.device_limit}
                  onChange={(e) => set('device_limit', Number(e.target.value))}
                />
                <PresetChips
                  values={DEVICE_PRESETS}
                  active={form.device_limit}
                  onSelect={(v) => set('device_limit', v)}
                  renderLabel={(v) => String(v)}
                />
              </div>
              <div>
                <TariffFieldLabel icon={HardDrive}>{t('admin.tariffs.traffic')}</TariffFieldLabel>
                <input
                  type="number"
                  min={0}
                  step={0.1}
                  className="admin-input w-full px-3 py-2"
                  value={form.traffic_gb}
                  onChange={(e) => set('traffic_gb', Number(e.target.value))}
                />
                <PresetChips
                  values={TRAFFIC_PRESETS}
                  active={form.traffic_gb}
                  onSelect={(v) => set('traffic_gb', v)}
                  renderLabel={(v) => (v === 0 ? t('admin.tariffs.unlimited') : String(v))}
                />
                {form.traffic_gb === 0 && (
                  <p className="mt-1.5 text-xs text-emerald-600 dark:text-emerald-400">
                    {t('admin.tariffs.trafficUnlimitedHint')}
                  </p>
                )}
              </div>
              <div className="sm:col-span-2">
                <TariffFieldLabel icon={RotateCcw}>{t('admin.tariffs.strategy')}</TariffFieldLabel>
                <select
                  className="admin-input w-full max-w-xs px-3 py-2"
                  value={form.traffic_limit_reset_strategy}
                  onChange={(e) => set('traffic_limit_reset_strategy', e.target.value)}
                >
                  {STRATEGIES.map((s) => (
                    <option key={s} value={s}>
                      {t(STRATEGY_I18N_KEYS[s] ?? s, { defaultValue: s })}
                    </option>
                  ))}
                </select>
              </div>
            </section>
          )}

          {activeTab === 'servers' && hasSquads && squadsData && (
            <section>
              <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
                <span className="text-xs text-muted-foreground">
                  {form.squad_uuids.length === 0
                    ? t('admin.tariffs.squadsAllSelected')
                    : t('admin.tariffs.squadsSelected', {
                        count: form.squad_uuids.length - unknownSquads.length,
                        total: squadsData.items.length,
                      })}
                </span>
                {squadsData.items.some((sq) => !form.squad_uuids.includes(sq.uuid)) && (
                  <button
                    type="button"
                    onClick={() => set('squad_uuids', squadsData.items.map((sq) => sq.uuid))}
                    className={surface('raised', 'rounded-md px-2 py-1 text-xs text-muted-foreground transition-colors hover:bg-accent hover:text-foreground')}
                  >
                    {t('admin.tariffs.squadsSelectAll')}
                  </button>
                )}
              </div>
              <div className="grid gap-2 sm:grid-cols-2">
                {squadsData.items.map((sq) => (
                  <label
                    key={sq.uuid}
                    className={cn(
                      'flex cursor-pointer items-center gap-2 rounded-lg px-3 py-2 text-sm transition-colors',
                      form.squad_uuids.includes(sq.uuid)
                        ? 'border border-primary/50 bg-primary/10'
                        : surface('raised', 'hover:bg-accent/40'),
                    )}
                  >
                    <AdminCheckbox
                      checked={form.squad_uuids.includes(sq.uuid)}
                      onChange={() => toggleSquad(sq.uuid)}
                      aria-label={sq.name}
                    />
                    <span className="truncate">{sq.name}</span>
                  </label>
                ))}
              </div>
              <p className="mt-2 text-xs text-muted-foreground">{t('admin.tariffs.squadsHint')}</p>
              {unknownSquads.length > 0 && (
                <div className="mt-2 flex flex-wrap items-center gap-2 text-xs text-amber-600 dark:text-amber-400">
                  <span>{t('admin.tariffs.squadsUnknown', { count: unknownSquads.length })}</span>
                  <button
                    type="button"
                    onClick={() =>
                      set('squad_uuids', form.squad_uuids.filter((u) => knownSquadUuids.includes(u)))
                    }
                    className={surface('raised', 'rounded-md px-2 py-1 transition-colors hover:bg-accent hover:text-foreground')}
                  >
                    {t('admin.tariffs.squadsUnknownDrop')}
                  </button>
                </div>
              )}
              {isEdit && tariff && (
                <AdminTariffSquadsApplyPanel
                  tariffId={tariff.id}
                  squads={squadsData.items}
                  dirty={isDirty}
                />
              )}
            </section>
          )}

          {activeTab === 'locations' && (
            <section className="space-y-3">
              <p className="text-xs text-muted-foreground">{t('admin.tariffs.locationHint')}</p>
              {form.locations.map((loc, index) => (
                <div key={`${loc.id}-${index}`} className="flex items-center gap-2">
                  <input
                    className="admin-input w-14 shrink-0 px-2 py-2 text-center uppercase"
                    maxLength={2}
                    value={loc.country_code ?? ''}
                    placeholder="DE"
                    aria-label="DE"
                    onChange={(e) => {
                      const country_code = e.target.value.toUpperCase()
                      setForm((prev) => ({
                        ...prev,
                        locations: prev.locations.map((row, i) => (i === index ? { ...row, country_code } : row)),
                      }))
                    }}
                  />
                  <input
                    className="admin-input min-w-0 flex-1 px-3 py-2"
                    maxLength={80}
                    value={loc.name}
                    placeholder={t('admin.tariffs.locationName')}
                    aria-label={t('admin.tariffs.locationName')}
                    onChange={(e) => {
                      const name = e.target.value
                      setForm((prev) => ({
                        ...prev,
                        locations: prev.locations.map((row, i) => (i === index ? { ...row, name } : row)),
                      }))
                    }}
                  />
                  <label className="flex shrink-0 items-center gap-1.5 text-xs text-muted-foreground">
                    <AdminCheckbox
                      checked={loc.sold_out}
                      aria-label={t('admin.tariffs.locationSoldOut')}
                      onChange={(sold_out) => {
                        setForm((prev) => ({
                          ...prev,
                          locations: prev.locations.map((row, i) => (i === index ? { ...row, sold_out } : row)),
                        }))
                      }}
                    />
                    {t('admin.tariffs.locationSoldOut')}
                  </label>
                  <button
                    type="button"
                    className="inline-flex size-8 shrink-0 items-center justify-center rounded-md text-muted-foreground hover:bg-accent hover:text-foreground"
                    aria-label={t('admin.tariffs.removeLocation')}
                    onClick={() => {
                      setForm((prev) => ({
                        ...prev,
                        locations: prev.locations.filter((_, i) => i !== index),
                      }))
                    }}
                  >
                    <Trash2 className="size-3.5" aria-hidden />
                  </button>
                </div>
              ))}
              <button
                type="button"
                className="inline-flex items-center gap-1.5 rounded-md border border-border/60 px-3 py-1.5 text-xs font-medium hover:bg-accent"
                onClick={() =>
                  setForm((prev) => ({
                    ...prev,
                    locations: [...prev.locations, { id: 0, name: '', sort_order: prev.locations.length, sold_out: false, country_code: '' }],
                  }))
                }
              >
                <Plus className="size-3.5" aria-hidden />
                {t('admin.tariffs.addLocation')}
              </button>
            </section>
          )}

          {activeTab === 'prices' && (
            <section>
              <p className="mb-3 text-xs text-muted-foreground">{t('admin.tariffs.pricesHint')}</p>
              <div className="overflow-hidden rounded-xl border border-border/60">
                <div className="hidden grid-cols-[minmax(6rem,1fr)_1fr_1fr] gap-3 border-b border-border/50 bg-muted/25 px-4 py-2 text-xs font-medium text-muted-foreground sm:grid">
                  <span>{t('admin.tariffs.pricePeriod')}</span>
                  <span>{t('admin.tariffs.priceRub')}</span>
                  <span>{t('admin.tariffs.priceStars')}</span>
                </div>
                <div className="divide-y divide-border/50">
                  {PERIOD_MONTHS.map((m, i) => {
                    const autoStars = starsFromRub(form.rub[i], rubPerStar)
                    const saving = savingsPercent(form.rub[0], form.rub[i], m)
                    const perMonth = m > 1 && form.rub[i] > 0 ? Math.round(form.rub[i] / m) : null
                    return (
                      <div
                        key={m}
                        className="grid gap-3 px-4 py-3 sm:grid-cols-[minmax(6rem,1fr)_1fr_1fr] sm:items-start"
                      >
                        <div className="flex items-center gap-2 sm:block">
                          <span className="text-sm font-medium">
                            {t('admin.users.monthsShort', { count: m })}
                          </span>
                          {saving != null && (
                            <span className="inline-block rounded-md bg-emerald-500/15 px-1.5 py-0.5 text-xs font-medium text-emerald-600 dark:text-emerald-400 sm:mt-1">
                              −{saving}%
                            </span>
                          )}
                        </div>
                        <div>
                          <label className="mb-1 block text-xs text-muted-foreground sm:sr-only">
                            {t('admin.tariffs.priceRub')}
                          </label>
                          <div className="relative">
                            <span className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-sm text-muted-foreground">
                              ₽
                            </span>
                            <input
                              type="number"
                              min={0}
                              className="admin-input w-full py-2 pl-7 pr-3 tabular-nums"
                              value={form.rub[i] || ''}
                              onChange={(e) => {
                                const n = [...form.rub] as [number, number, number, number]
                                n[i] = Number(e.target.value) || 0
                                set('rub', n)
                              }}
                            />
                          </div>
                          {perMonth != null && (
                            <p className="mt-1 text-xs text-muted-foreground">
                              {t('admin.tariffs.perMonth', { amount: perMonth })}
                            </p>
                          )}
                        </div>
                        <div>
                          <label className="mb-1 block text-xs text-muted-foreground sm:sr-only">
                            {t('admin.tariffs.priceStars')}
                          </label>
                          <div className="relative">
                            <span className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-sm text-muted-foreground">
                              ⭐
                            </span>
                            <input
                              type="number"
                              min={0}
                              placeholder={autoStars != null ? `≈ ${autoStars}` : '—'}
                              className="admin-input w-full py-2 pl-8 pr-3 tabular-nums"
                              value={form.stars[i] ?? ''}
                              onChange={(e) => {
                                const n = [...form.stars] as TariffFormData['stars']
                                n[i] = e.target.value ? Number(e.target.value) : null
                                set('stars', n)
                              }}
                            />
                          </div>
                          {form.stars[i] != null && autoStars != null && form.stars[i] !== autoStars && (
                            <p className="mt-1 text-xs text-muted-foreground">
                              {t('admin.tariffs.starsByRateNote', { count: autoStars })}
                            </p>
                          )}
                        </div>
                      </div>
                    )
                  })}
                </div>
              </div>

              {rubPerStar > 0 ? (
                <div className="mt-3 flex flex-wrap items-center gap-2">
                  <p className="mr-auto text-xs text-muted-foreground">
                    {t('admin.tariffs.starsAutoHint', { rate: rubPerStar })}
                  </p>
                  <button
                    type="button"
                    onClick={fillStarsByRate}
                    className="inline-flex items-center gap-1.5 rounded-md border border-border/60 px-2.5 py-1.5 text-xs transition-colors hover:bg-accent"
                  >
                    <Sparkles className="size-3.5" aria-hidden />
                    {t('admin.tariffs.starsFreeze')}
                  </button>
                  {hasAnyStars && (
                    <button
                      type="button"
                      onClick={clearStars}
                      className="inline-flex items-center gap-1.5 rounded-md border border-border/60 px-2.5 py-1.5 text-xs text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
                    >
                      <Eraser className="size-3.5" aria-hidden />
                      {t('admin.tariffs.starsReset')}
                    </button>
                  )}
                </div>
              ) : (
                <p className="mt-3 flex items-start gap-1.5 text-xs text-amber-600 dark:text-amber-400">
                  <AlertTriangle className="mt-0.5 size-3.5 shrink-0" aria-hidden />
                  <span>{t('admin.tariffs.starsRateMissing')}</span>
                </p>
              )}
            </section>
          )}

          {activeTab === 'description' && (
            <>
              <section>
                <TariffEditorSectionHeader icon={FileText} accent="slate">
                  {t('admin.tariffs.description')}
                </TariffEditorSectionHeader>
                <p className="mb-3 text-xs text-muted-foreground">{t('admin.tariffs.descriptionHint')}</p>
                <TariffDescriptionEditor
                  value={form.description}
                  onChange={(v) => set('description', v)}
                  resetKey={descriptionResetKey}
                  placeholder={t('admin.tariffs.descriptionSource')}
                />
              </section>

              <section>
                <TariffEditorSectionHeader icon={FileText} accent="blue">
                  {t('admin.tariffs.descriptionDetail')}
                </TariffEditorSectionHeader>
                <p className="mb-3 text-xs text-muted-foreground">
                  {t('admin.tariffs.descriptionDetailHint')}
                </p>
                <TariffDescriptionEditor
                  value={form.description_detail}
                  onChange={(v) => set('description_detail', v)}
                  resetKey={descriptionResetKey}
                  placeholder={t('admin.tariffs.descriptionSource')}
                />
              </section>
            </>
          )}
        </div>
      </AdminModal>

      <AdminConfirmModal
        open={confirmCloseOpen}
        onClose={() => setConfirmCloseOpen(false)}
        onConfirm={() => {
          setConfirmCloseOpen(false)
          onClose()
        }}
        title={t('admin.tariffs.unsavedTitle')}
        message={t('admin.tariffs.unsavedMessage')}
        confirmLabel={t('admin.tariffs.unsavedConfirm')}
        variant="destructive"
      />

      <AdminTariffSquadsSaveDialog
        open={applyDialogOpen}
        onClose={() => setApplyDialogOpen(false)}
        onSubmit={(applyNow) => {
          setApplyDialogOpen(false)
          submit(applyNow ? { add: addedSquads, remove: removedSquads } : null)
        }}
        added={addedSquads}
        removed={removedSquads}
        squads={squadsData?.items ?? []}
        activeCount={activeOnTariff}
        loading={Boolean(saving)}
      />
    </>
  )
}
