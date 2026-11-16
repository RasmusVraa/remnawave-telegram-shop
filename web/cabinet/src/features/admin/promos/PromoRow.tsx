import { useRef, useState, type KeyboardEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { CalendarClock, CircleDot, Copy, CopyPlus, Gift, Hash, MoreVertical, Pencil, Power, Trash2, Users, type LucideIcon } from 'lucide-react'

import { cn } from '@/lib/utils'
import { useCopyToClipboard } from '@/hooks/useCopyToClipboard'
import type { AdminPromoCodeDTO } from '@/lib/types/admin'
import { AdminToggleSwitch } from '../components/AdminToggleSwitch'
import { Spinner } from '@/components/ui/spinner'
import { HeaderFilter } from '../components/HeaderFilter'
import { SortHeaderButton, type SortState } from '../components/SortableTh'
import type { AdminPromoStatusFilter, AdminPromoTypeFilter } from '@/lib/types/admin'
import { useAdminPromoToggle } from '../hooks/useAdminPromos'
import { resolvePromoDisplayStatus } from '../utils/promoStatus'
import {
  CopyCodeButton,
  PromoExpiryText,
  PromoPopover,
  PromoPopoverItem,
  PromoStatusChip,
  PromoTypeTile,
  PromoUsageBar,
  PromoUsageText,
} from './PromoBits'
import type { PromoFormFocus } from './PromoFormModal'
import { describePromo, PROMO_TYPES } from './promoPresentation'

export interface PromoItemActions {
  /** Правка — из меню «⋯» и из листа с подробностями. */
  onEdit: (p: AdminPromoCodeDTO, focus?: PromoFormFocus) => void
  /** Клик по строке или карточке — лист с подробностями (один флоу на ПК и телефоне). */
  onOpen: (p: AdminPromoCodeDTO) => void
  onUsed: (p: AdminPromoCodeDTO) => void
  onDuplicate: (p: AdminPromoCodeDTO) => void
  onDelete: (p: AdminPromoCodeDTO) => void
  onToggled: (updated: AdminPromoCodeDTO) => void
  onError: (err: unknown) => void
}

/** Колонки таблицы из макета: код · что даёт · использовано · срок · статус · вкл. · ⋯ */
export const PROMO_TABLE_COLUMNS = '190px minmax(0,1.6fr) 170px 140px 120px 56px 40px'

/** Переключатель «Активен»: спиннер в ручке только у своей строки. */
export function PromoActiveSwitch({
  promo,
  onToggled,
  onError,
}: {
  promo: AdminPromoCodeDTO
  onToggled: (updated: AdminPromoCodeDTO) => void
  onError: (err: unknown) => void
}) {
  const { t } = useTranslation()
  const toggle = useAdminPromoToggle()
  return (
    <span
      className="inline-flex"
      onClick={(e) => e.stopPropagation()}
      title={promo.type === 'discount' && promo.active ? t('admin.promos.toggle.discountHint') : undefined}
    >
      <AdminToggleSwitch
        checked={promo.active}
        busy={toggle.isPending}
        aria-label={t('admin.promos.toggle.label')}
        onChange={(next) =>
          toggle.mutate({ promo, active: next }, { onSuccess: (updated) => onToggled(updated), onError })
        }
      />
    </span>
  )
}

function PromoRowMenu({ promo, actions }: { promo: AdminPromoCodeDTO; actions: PromoItemActions }) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const btnRef = useRef<HTMLButtonElement>(null)
  const { copy } = useCopyToClipboard()
  const run = (fn: () => void) => () => {
    setOpen(false)
    fn()
  }
  return (
    <>
      <button
        ref={btnRef}
        type="button"
        aria-label={t('admin.promos.menu.more')}
        aria-haspopup="menu"
        aria-expanded={open}
        onClick={(e) => {
          e.stopPropagation()
          setOpen((v) => !v)
        }}
        className={cn(
          'inline-flex size-[34px] items-center justify-center rounded-[10px] border border-border/70 text-foreground transition-colors hover:bg-primary/10',
          open && 'bg-primary/10',
        )}
      >
        <MoreVertical className="size-4" />
      </button>
      <PromoPopover anchorRef={btnRef} open={open} onClose={() => setOpen(false)} align="right" width={210} label={t('admin.promos.menu.more')}>
        <PromoPopoverItem icon={<Pencil className="size-4" />} onClick={run(() => actions.onEdit(promo))}>
          {t('admin.promos.menu.edit')}
        </PromoPopoverItem>
        <PromoPopoverItem icon={<Users className="size-4" />} onClick={run(() => actions.onUsed(promo))}>
          {t('admin.promos.menu.used')}
        </PromoPopoverItem>
        <PromoPopoverItem icon={<CopyPlus className="size-4" />} onClick={run(() => actions.onDuplicate(promo))}>
          {t('admin.promos.menu.duplicate')}
        </PromoPopoverItem>
        <PromoPopoverItem icon={<Copy className="size-4" />} onClick={run(() => void copy(promo.code))}>
          {t('admin.promos.menu.copy')}
        </PromoPopoverItem>
        <div className="mx-1 my-1.5 border-t border-border/60" />
        <PromoPopoverItem danger icon={<Trash2 className="size-4" />} onClick={run(() => actions.onDelete(promo))}>
          {t('admin.promos.menu.delete')}
        </PromoPopoverItem>
      </PromoPopover>
    </>
  )
}

function CodeWithNote({ promo, size }: { promo: AdminPromoCodeDTO; size: 'row' | 'card' }) {
  return (
    <div className="flex min-w-0 flex-col gap-0.5">
      <div className="flex min-w-0 items-center gap-1">
        <span
          className={cn('truncate font-mono font-bold', size === 'row' ? 'text-[15px]' : 'text-base')}
          title={promo.code}
        >
          {promo.code}
        </span>
        <CopyCodeButton code={promo.code} />
      </div>
    </div>
  )
}

function Note({ note, className }: { note?: string | null; className?: string }) {
  if (!note) return null
  return (
    <p className={cn('truncate text-xs font-medium text-muted-foreground', className)} title={note}>
      {note}
    </p>
  )
}

function Essence({ promo, tariffName, tile }: { promo: AdminPromoCodeDTO; tariffName?: string | null; tile: 30 | 34 }) {
  const { t } = useTranslation()
  const e = describePromo(promo, t, tariffName)
  return (
    <div className="flex min-w-0 items-center gap-3">
      <PromoTypeTile type={promo.type} size={tile} />
      <div className="min-w-0">
        <p className="truncate text-sm font-semibold leading-snug">{e.title}</p>
        <p className="truncate text-[12.5px] leading-snug text-muted-foreground" title={e.sub}>
          {e.sub}
        </p>
      </div>
    </div>
  )
}

export function PromoRow({
  promo,
  tariffName,
  actions,
}: {
  promo: AdminPromoCodeDTO
  tariffName?: string | null
  actions: PromoItemActions
}) {
  const { t } = useTranslation()
  const dim = resolvePromoDisplayStatus(promo) === 'inactive'
  const onKey = (e: KeyboardEvent) => {
    if (e.target !== e.currentTarget) return
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault()
      actions.onOpen(promo)
    }
  }
  return (
    <div
      role="row"
      tabIndex={0}
      onClick={() => actions.onOpen(promo)}
      onKeyDown={onKey}
      className="grid min-h-[68px] cursor-pointer items-center gap-4 border-t border-border/60 px-5 py-2.5 transition-colors hover:bg-primary/5 focus-visible:bg-primary/5 focus-visible:outline-none"
      style={{ gridTemplateColumns: PROMO_TABLE_COLUMNS }}
    >
      <div className={cn('min-w-0', dim && 'opacity-60')}>
        <CodeWithNote promo={promo} size="row" />
        <Note note={promo.note} />
      </div>
      <div className={cn('min-w-0', dim && 'opacity-60')}>
        <Essence promo={promo} tariffName={tariffName} tile={34} />
      </div>
      <button
        type="button"
        title={t('admin.promos.used.open')}
        aria-label={t('admin.promos.used.open')}
        onClick={(e) => {
          e.stopPropagation()
          actions.onUsed(promo)
        }}
        className="-mx-2 flex min-w-0 flex-col gap-1.5 rounded-[10px] px-2 py-1.5 text-left transition-colors hover:bg-primary/10"
      >
        <PromoUsageText promo={promo} />
        <PromoUsageBar promo={promo} className="w-full" />
      </button>
      <div className="min-w-0">
        <PromoExpiryText validUntil={promo.valid_until} />
      </div>
      <div>
        <PromoStatusChip promo={promo} />
      </div>
      <div>
        <PromoActiveSwitch promo={promo} onToggled={actions.onToggled} onError={actions.onError} />
      </div>
      <div onClick={(e) => e.stopPropagation()}>
        <PromoRowMenu promo={promo} actions={actions} />
      </div>
    </div>
  )
}

/** Подпись колонки с иконкой — как в шапке «Пользователей». */
function HeadCell({ icon: Icon, label }: { icon: LucideIcon; label: string }) {
  return (
    <span className="inline-flex items-center gap-1.5">
      <Icon className="size-3.5 shrink-0 text-muted-foreground/70" aria-hidden />
      {label}
    </span>
  )
}

const STATUS_FILTERS: AdminPromoStatusFilter[] = ['all', 'active', 'ended', 'off']

/**
 * Шапка таблицы: сортировка по коду, использованию и сроку; фильтры типа и
 * статуса — те же компоненты, что у «Пользователей» и «Платежей». Без
 * обработчиков (скелетон) — просто подписи.
 */
export function PromoTableHeader({
  sort,
  onSort,
  type,
  onType,
  status,
  onStatus,
  busyKey,
}: {
  /** Какой контрол шапки перезапрашивает список: code | uses | expiring | type | status. */
  busyKey?: string | null
  sort?: SortState | null
  onSort?: (key: string) => void
  type?: AdminPromoTypeFilter | null
  onType?: (v: AdminPromoTypeFilter | null) => void
  status?: AdminPromoStatusFilter
  onStatus?: (v: AdminPromoStatusFilter) => void
} = {}) {
  const { t } = useTranslation()
  const sortable = (key: string, icon: LucideIcon, label: string) =>
    onSort ? (
      <SortHeaderButton sortKey={key} sort={sort ?? null} onSort={onSort} busy={busyKey === key}>
        <HeadCell icon={icon} label={label} />
      </SortHeaderButton>
    ) : (
      <HeadCell icon={icon} label={label} />
    )
  return (
    <div
      role="row"
      className="grid min-h-11 items-center gap-4 border-b border-border bg-muted/40 px-5 text-xs font-medium uppercase tracking-wider text-muted-foreground"
      style={{ gridTemplateColumns: PROMO_TABLE_COLUMNS }}
    >
      <div>{sortable('code', Hash, t('admin.promos.columns.code'))}</div>
      <div className="inline-flex items-center gap-1">
        <HeadCell icon={Gift} label={t('admin.promos.columns.gives')} />
        {onType && (
          <HeaderFilter<string>
            value={type ?? 'all'}
            options={[
              { value: 'all', label: t('admin.promos.filters.allTypes') },
              ...PROMO_TYPES.map((ty) => ({ value: ty, label: t(`admin.promos.types.${ty}`) })),
            ]}
            onChange={(v) => onType(v === 'all' ? null : (v as AdminPromoTypeFilter))}
            ariaLabel={t('admin.promos.filters.typeSection')}
          />
        )}
        {busyKey === 'type' && <Spinner className="h-3.5 w-3.5 text-primary" />}
      </div>
      <div>{sortable('uses', Users, t('admin.promos.columns.used'))}</div>
      <div>{sortable('expiring', CalendarClock, t('admin.promos.columns.expiry'))}</div>
      <div className="inline-flex items-center gap-1">
        <HeadCell icon={CircleDot} label={t('admin.promos.columns.status')} />
        {onStatus && (
          <HeaderFilter<AdminPromoStatusFilter>
            value={status ?? 'all'}
            options={STATUS_FILTERS.map((s) => ({ value: s, label: t(`admin.promos.tabs.${s}`) }))}
            onChange={onStatus}
            ariaLabel={t('admin.promos.columns.status')}
          />
        )}
        {busyKey === 'status' && <Spinner className="h-3.5 w-3.5 text-primary" />}
      </div>
      <div>
        <HeadCell icon={Power} label={t('admin.promos.columns.on')} />
      </div>
      <div />
    </div>
  )
}

export function PromoCard({
  promo,
  tariffName,
  actions,
}: {
  promo: AdminPromoCodeDTO
  tariffName?: string | null
  actions: PromoItemActions
}) {
  const { t } = useTranslation()
  const status = resolvePromoDisplayStatus(promo)
  const dim = status === 'inactive'
  return (
    <div
      role="button"
      tabIndex={0}
      onClick={() => actions.onOpen(promo)}
      onKeyDown={(e) => {
        if (e.target !== e.currentTarget) return
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault()
          actions.onOpen(promo)
        }
      }}
      className="flex min-w-0 cursor-pointer flex-col gap-[11px] rounded-[18px] border border-border/60 bg-card p-[14px] transition-colors hover:border-primary/40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
    >
      <div className="flex min-w-0 items-center gap-2">
        <div className={cn('min-w-0', dim && 'opacity-60')}>
          <CodeWithNote promo={promo} size="card" />
        </div>
        {status !== 'active' && <PromoStatusChip promo={promo} />}
        <span className="flex-1" />
        <PromoActiveSwitch promo={promo} onToggled={actions.onToggled} onError={actions.onError} />
        <span className="inline-flex" onClick={(e) => e.stopPropagation()}>
          <PromoRowMenu promo={promo} actions={actions} />
        </span>
      </div>
      {promo.note && <Note note={promo.note} className={cn('-mt-1.5', dim && 'opacity-60')} />}
      <div className={cn(dim && 'opacity-60')}>
        <Essence promo={promo} tariffName={tariffName} tile={30} />
      </div>
      {/* Полоска одной длины на всех карточках: не зависит от ширины даты справа. */}
      {/* Полоска и «N из M» — сразу «Кто использовал» (как ячейка «Использовано» на ПК). */}
      <div className="flex items-center gap-2.5">
        <button
          type="button"
          aria-label={t('admin.promos.used.open')}
          onClick={(e) => {
            e.stopPropagation()
            actions.onUsed(promo)
          }}
          className="-my-2 w-[38%] shrink-0 py-2"
        >
          <PromoUsageBar promo={promo} className="w-full" />
        </button>
        <button
          type="button"
          tabIndex={-1}
          aria-hidden
          onClick={(e) => {
            e.stopPropagation()
            actions.onUsed(promo)
          }}
          className="-my-2 py-2"
        >
          <PromoUsageText promo={promo} className="text-[12.5px]" />
        </button>
        <PromoExpiryText validUntil={promo.valid_until} className="ml-auto text-[12.5px]" />
      </div>
    </div>
  )
}
