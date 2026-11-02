import { createContext, type ReactNode, useContext, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useMutation } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Minus, MonitorSmartphone, Plus, Smartphone, Trash2 } from 'lucide-react'

import { AddDeviceIcon } from '@/components/AddDeviceIcon'
import { ConfirmModal } from '@/components/ConfirmModal'
import { ConnectDeviceButton } from '@/components/SubscriptionActions'
import { Skeleton } from '@/components/ui/skeleton'
import { useToast } from '@/components/ui/toast'
import { ConnectInviteModal } from '@/features/subscription/ConnectExtraDevice'
import { DeviceRow } from '@/features/subscription/DeviceRow'
import { DeviceSlotsFullCta, DeviceSlotsModal } from '@/features/subscription/DeviceSlots'
import { SubscriptionLinkResetLink, SubscriptionLinkResetSheet } from '@/features/subscription/SubscriptionLinkReset'
import { api } from '@/lib/api'
import { cn } from '@/lib/utils'
import type { HomeData } from './useHomeData'

/**
 * Управление устройствами на главной: удаление, приглашение на другое
 * устройство, места и сброс ссылки. Раньше это жило на странице «Подписка»;
 * теперь к нему ведут блок «Мои устройства» на ПК, шторка на телефоне и
 * быстрое действие «Добавить доп. устройство» — поэтому состояние модалок
 * общее и лежит в контексте.
 */

type DeviceManagerApi = {
  openConnect: () => void
  openSlots: (target: number) => void
  requestDelete: (hwid: string) => void
  deletePending: boolean
}

const Ctx = createContext<DeviceManagerApi | null>(null)

export function useDeviceManager(): DeviceManagerApi {
  const v = useContext(Ctx)
  if (!v) throw new Error('useDeviceManager outside DeviceManagerProvider')
  return v
}

export function DeviceManagerProvider({ data, children }: { data: HomeData; children: ReactNode }) {
  const { t } = useTranslation()
  const toast = useToast()
  const navigate = useNavigate()
  const { sub, hwid, slotsVisible, connected, stopped, hasLink } = data

  const [deleteConfirmHwid, setDeleteConfirmHwid] = useState<string | null>(null)
  const [connectOpen, setConnectOpen] = useState(false)
  const [resetOpen, setResetOpen] = useState(false)
  const [slotsOpen, setSlotsOpen] = useState(false)
  const [slotsInitial, setSlotsInitial] = useState(0)

  const deleteDevice = useMutation({
    mutationFn: (h: string) => api.deleteDevice(h),
    onSuccess: async () => {
      await data.devicesQuery.refetch()
    },
    // Удаление устройства молча ничего не делало при отказе сервера.
    onError: () => toast.error(t('errors.unknown')),
  })

  const value: DeviceManagerApi = {
    openConnect: () => {
      if (hasLink) setConnectOpen(true)
    },
    openSlots: (target) => {
      if (!hwid) return
      setSlotsInitial(target)
      setSlotsOpen(true)
    },
    requestDelete: (h) => setDeleteConfirmHwid(h),
    deletePending: deleteDevice.isPending,
  }

  return (
    <Ctx.Provider value={value}>
      {children}

      {/* Модалки смонтированы постоянно и сами решают, когда исчезнуть:
          при снятии по флагу анимация ухода не успевала бы проиграться. */}
      {sub?.subscription_link && (
        <ConnectInviteModal
          open={connectOpen}
          subscriptionLink={sub.subscription_link}
          onClose={() => setConnectOpen(false)}
          onResetLink={
            stopped
              ? undefined
              : () => {
                  setConnectOpen(false)
                  setResetOpen(true)
                }
          }
        />
      )}

      {slotsVisible && hwid && (
        <DeviceSlotsModal
          open={slotsOpen}
          hwid={hwid}
          connected={connected}
          expireAt={sub?.expire_at}
          initialTarget={slotsInitial}
          onClose={() => setSlotsOpen(false)}
          onUpdated={() => void Promise.all([data.subQuery.refetch(), data.devicesQuery.refetch()])}
        />
      )}

      <SubscriptionLinkResetSheet
        open={resetOpen}
        availableAt={sub?.link_reset_available_at}
        onClose={() => setResetOpen(false)}
        onConnectThis={() => {
          setResetOpen(false)
          navigate('/connections')
        }}
        onConnectOther={() => {
          setResetOpen(false)
          setConnectOpen(true)
        }}
      />

      <ConfirmModal
        open={Boolean(deleteConfirmHwid)}
        tone="danger"
        icon={Trash2}
        inlineTitle
        title={t('subscriptionPage.deleteDeviceConfirm')}
        description={
          <>
            <p>{t('subscriptionPage.deleteDeviceConfirmHint')}</p>
            {/* Чужое устройство удалением не убрать: пока у человека
                ссылка, оно подключится снова. */}
            {hasLink && !stopped && (
              <div className="mt-3 rounded-xl bg-muted/60 px-3 py-2.5 text-xs leading-relaxed">
                <p>{t('linkReset.deviceHint')}</p>
                <SubscriptionLinkResetLink
                  label={t('linkReset.deviceHintAction')}
                  className="-mb-1.5 mt-0.5 text-foreground"
                  onClick={() => {
                    setDeleteConfirmHwid(null)
                    setResetOpen(true)
                  }}
                />
              </div>
            )}
          </>
        }
        confirmLabel={t('subscriptionPage.deleteDevice')}
        loading={deleteDevice.isPending}
        onClose={() => setDeleteConfirmHwid(null)}
        onConfirm={() => {
          const h = deleteConfirmHwid
          if (!h) return
          setDeleteConfirmHwid(null)
          deleteDevice.mutate(h)
        }}
      />
    </Ctx.Provider>
  )
}

/** Кнопка «Добавить доп. устройство» (QR и пересылка приглашения). */
export function AddExtraDeviceButton({ className, size = 'sm' }: { className?: string; size?: 'sm' | 'lg' }) {
  const { t } = useTranslation()
  const { openConnect } = useDeviceManager()
  if (size === 'lg') {
    return (
      <button
        type="button"
        onClick={openConnect}
        className={cn(
          'flex h-12 w-full items-center justify-center gap-2.5 rounded-full border border-border bg-secondary text-[15px] font-semibold text-foreground transition-colors hover:bg-secondary/80',
          className,
        )}
      >
        <AddDeviceIcon size={18} />
        {t('home.qaAddExtra')}
      </button>
    )
  }
  return (
    <button
      type="button"
      onClick={openConnect}
      className={cn(
        'inline-flex h-10 min-w-0 items-center gap-2 rounded-xl border border-border bg-secondary/60 px-3.5 text-sm font-semibold text-foreground transition-colors hover:bg-secondary',
        className,
      )}
    >
      <AddDeviceIcon size={17} className="shrink-0" />
      <span className="truncate">{t('home.qaAddExtra')}</span>
    </button>
  )
}

/** Счётчик мест «− 5 мест +»: нажатие открывает окно мест с ±1. */
function SlotsStepper({ data }: { data: HomeData }) {
  const { t } = useTranslation()
  const { openSlots } = useDeviceManager()
  const hwid = data.hwid
  if (!hwid) return null
  const n = hwid.current_limit
  return (
    <div className="inline-flex h-10 shrink-0 items-center gap-0.5 rounded-xl border border-border bg-secondary/60 px-1 text-sm font-semibold">
      <button
        type="button"
        disabled={!hwid.can_decrease}
        onClick={() => openSlots(n - 1)}
        aria-label={t('subscriptionPage.slotsStepDown')}
        className="inline-flex size-7 items-center justify-center rounded-lg bg-secondary text-foreground transition-colors hover:bg-muted disabled:opacity-40 sm:size-8"
      >
        <Minus size={16} />
      </button>
      {/* Число тоже открывает окно — с текущим значением, а там уже и докупить, и убрать. */}
      <button
        type="button"
        onClick={() => openSlots(n)}
        className="mx-0.5 whitespace-nowrap rounded-lg px-1.5 py-1 tabular-nums transition-colors hover:bg-muted"
      >
        {t('subscriptionPage.slotsCount', { count: n })}
      </button>
      <button
        type="button"
        disabled={!hwid.can_increase}
        onClick={() => openSlots(n + 1)}
        aria-label={t('subscriptionPage.slotsStepUp')}
        className="inline-flex size-7 items-center justify-center rounded-lg bg-secondary text-foreground transition-colors hover:bg-muted disabled:opacity-40 sm:size-8"
      >
        <Plus size={16} />
      </button>
    </div>
  )
}

/**
 * Низ блока устройств (SPEC §8): кнопка добавления и счётчик мест, плашка
 * «Все места заняты» или пояснение про закреплённый лимит.
 */
function DevicesFooter({ data, withAddButton }: { data: HomeData; withAddButton: boolean }) {
  const { t } = useTranslation()
  const { openSlots } = useDeviceManager()
  const { hwid, slotsVisible, allSlotsTaken, isTrial, stopped, hasLink, sub } = data
  const canBuySlot = Boolean(slotsVisible && hwid?.can_increase)

  if (allSlotsTaken && canBuySlot && hwid) {
    return <DeviceSlotsFullCta hwid={hwid} expireAt={sub?.expire_at} onOpen={() => openSlots(hwid.current_limit + 1)} />
  }

  const showAdd = withAddButton && hasLink && !stopped && !allSlotsTaken
  const pinned = Boolean(slotsVisible && hwid?.pinned)
  const showStepper = Boolean(
    slotsVisible && hwid && !hwid.pinned && !isTrial && (hwid.can_increase || hwid.can_decrease),
  )

  if (!showAdd && !showStepper && !pinned) {
    if (allSlotsTaken && !stopped) {
      return (
        <p className="border-t border-border/[0.55] px-1 pt-3 text-[13px] text-muted-foreground">
          {t('subscriptionPage.devicesAllTaken')}
        </p>
      )
    }
    return null
  }

  return (
    <div className="flex flex-nowrap items-center gap-2 border-t border-border/[0.55] px-0 pb-0.5 pt-3 sm:gap-2.5 sm:px-1">
      {showAdd ? (
        <AddExtraDeviceButton />
      ) : showStepper ? (
        <span className="text-[13px] text-muted-foreground">{t('home.slotsLabel')}</span>
      ) : null}
      <span className="flex-1" />
      {showStepper && <SlotsStepper data={data} />}
      {pinned && <span className="text-[13px] text-muted-foreground">{t('subscriptionPage.slotsPinned')}</span>}
    </div>
  )
}

function DevicesEmpty() {
  const { t } = useTranslation()
  return (
    <div className="flex flex-col items-center gap-1.5 border-t border-border/[0.55] px-3 pb-4 pt-5 text-center">
      <span className="mb-1 inline-flex size-[52px] items-center justify-center rounded-full bg-primary/[0.12] text-primary">
        <MonitorSmartphone size={22} />
      </span>
      <p className="text-base font-bold">{t('home.noDevicesTitle')}</p>
      <p className="text-[13px] text-muted-foreground">{t('home.noDevicesHint')}</p>
    </div>
  )
}

function DevicesRowsSkeleton() {
  return (
    <ul aria-hidden>
      {[0, 1].map((i) => (
        <li key={i} className="flex items-center gap-3 border-t border-border/[0.55] px-1 py-2.5">
          <Skeleton className="size-10 rounded-xl" />
          <div className="space-y-1.5">
            <Skeleton className="h-3.5 w-28" />
            <Skeleton className="h-3 w-20" />
          </div>
        </li>
      ))}
    </ul>
  )
}

/**
 * Последняя строка списка в шторке: «＋ Добавить устройство» — то же окно с
 * QR и приглашением, что у быстрого действия на главной.
 */
function AddDeviceRow() {
  const { t } = useTranslation()
  const { openConnect } = useDeviceManager()
  return (
    <button
      type="button"
      onClick={openConnect}
      className="flex w-full items-center gap-3 border-t border-border/[0.55] px-1 py-2.5 text-left text-[15px] font-semibold text-primary"
    >
      <span className="inline-flex size-10 shrink-0 items-center justify-center rounded-xl border-[1.5px] border-dashed border-primary/55">
        <Plus size={18} />
      </span>
      {t('home.addDevice')}
    </button>
  )
}

/** Подзаголовок шторки: «Подключено 3 из 4» или без лимита — «Подключено 3». */
export function devicesSheetSubtitle(data: HomeData, t: (k: string, o?: Record<string, unknown>) => string): string | undefined {
  const d = data.devices
  if (!d?.enabled) return undefined
  const used = d.connected ?? 0
  return d.device_limit > 0 ? t('home.connectedOf', { used, limit: d.device_limit }) : t('home.connectedN', { used })
}

/**
 * Список устройств. variant="desktop" — карточка на главной ПК с шапкой
 * «Мои устройства 4 / 5» и кнопкой добавления; variant="sheet" — строки
 * прямо на шторке телефона (счётчик — в её подзаголовке), в конце списка
 * «＋ Добавить устройство», ниже места.
 */
export function DevicesPanel({ data, variant }: { data: HomeData; variant: 'desktop' | 'sheet' }) {
  const { t } = useTranslation()
  const { requestDelete, deletePending } = useDeviceManager()
  const { devices, devicesQuery, hasLink, stopped, allSlotsTaken } = data
  const list = devices?.devices ?? []
  const sheet = variant === 'sheet'
  const counter = devices?.enabled ? (
    <span
      className={cn(
        'shrink-0 tabular-nums',
        'text-[13px] text-muted-foreground',
      )}
    >
      {devices.connected ?? 0} / {devices.device_limit > 0 ? devices.device_limit : t('subscriptionPage.unlimited')}
    </span>
  ) : null

  return (
    <section
      id={sheet ? undefined : 'devices'}
      className={cn(
        sheet
          ? // первая строка лежит сразу под заголовком шторки — разделитель над ней лишний
            '-mt-1 [&>*:first-child]:border-t-0 [&>ul>li:first-child]:border-t-0'
          : 'cabinet-elevated-card scroll-mt-24 p-4 pb-3',
      )}
    >
      {!sheet && (
        <div className="flex items-center gap-2.5 px-1 pb-2">
          <p className="flex items-center gap-2 text-base font-bold">
            <Smartphone size={18} className="text-primary" />
            {t('home.myDevices')}
          </p>
          <span className="flex-1" />
          {counter}
        </div>
      )}

      {!devices?.enabled && !devicesQuery.isLoading ? (
        <p className="border-t border-border/[0.55] px-1 py-3 text-sm text-muted-foreground">
          {t('subscriptionPage.devicesUnavailable')}
        </p>
      ) : devicesQuery.isLoading ? (
        <DevicesRowsSkeleton />
      ) : list.length === 0 ? (
        <DevicesEmpty />
      ) : (
        <ul>
          {list.map((d) => (
            <DeviceRow key={d.hwid} device={d} deleteDisabled={deletePending} onDelete={() => requestDelete(d.hwid)} />
          ))}
        </ul>
      )}

      {/* Пустой список — первое устройство подключают на этом же: главная кнопка, а не «＋» с QR для другого. */}
      {sheet && devices?.enabled && !devicesQuery.isLoading && list.length === 0 ? (
        <div className="pt-1">
          <ConnectDeviceButton
            days={data.effectiveDays}
            devicesUsed={data.connected}
            devicesLimit={data.deviceLimit}
            isTrial={data.isTrial}
          />
        </div>
      ) : (
        sheet && devices?.enabled && hasLink && !stopped && !allSlotsTaken && <AddDeviceRow />
      )}

      <DevicesFooter data={data} withAddButton={!sheet} />
    </section>
  )
}
