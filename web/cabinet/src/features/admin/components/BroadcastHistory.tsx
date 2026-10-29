import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import { AlertTriangle, CheckCircle2, Copy, History, X, XCircle } from 'lucide-react'

import { cn } from '@/lib/utils'
import type { AdminBroadcastHistoryItemDTO } from '@/lib/types/admin'

import { AdminModal } from './AdminModal'
import { AdminBroadcastMessagePreview } from './AdminBroadcastMessagePreview'
import { normalizeBroadcastButtons } from '../utils/broadcastDraft'
import { BROADCAST_LINK_KEYS, broadcastLinkLabelKey } from '../utils/broadcastLinks'

type Item = AdminBroadcastHistoryItemDTO

/** Разметка для предпросмотра: текст из Telegram-админки хранится без неё. */
export function broadcastItemHtml(item: Item): string {
  if (item.parse_mode === 'HTML') return item.text
  return item.text.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
}

/** Первая строка текста — по ней рассылку узнают в списке. */
function itemTitle(item: Item, t: TFunction): string {
  const plain =
    item.parse_mode === 'HTML'
      ? (new DOMParser().parseFromString(item.text.replace(/<br\s*\/?>/gi, '\n'), 'text/html').body
          .textContent ?? '')
      : item.text
  const line = plain
    .split('\n')
    .map((s) => s.trim())
    .find(Boolean)
  if (line) return line
  if (item.media_kind) return t(`admin.broadcast.history.mediaKind.${item.media_kind}`)
  return t('admin.broadcast.history.noText')
}

function notDelivered(item: Item) {
  return item.blocked + item.failed
}

function percent(part: number, total: number) {
  return total > 0 ? Math.round((part / total) * 100) : 0
}

function formatWhen(iso: string, locale: string) {
  const d = new Date(iso)
  return {
    date: d.toLocaleDateString(locale, { day: 'numeric', month: 'short' }),
    time: d.toLocaleTimeString(locale, { hour: '2-digit', minute: '2-digit' }),
  }
}

function formatDuration(item: Item, t: TFunction): string | null {
  if (!item.finished_at) return null
  const sec = Math.max(0, Math.round((Date.parse(item.finished_at) - Date.parse(item.started_at)) / 1000))
  if (sec < 60) return t('admin.broadcast.history.durationSec', { s: sec })
  return t('admin.broadcast.history.durationMin', { m: Math.floor(sec / 60), s: sec % 60 })
}

function audienceText(item: Item, audienceLabels: Record<string, string>) {
  return item.audiences.map((a) => audienceLabels[a] ?? a).join(', ')
}

function isTest(item: Item) {
  return item.audiences.length === 1 && item.audiences[0] === 'test_broadcast'
}

/** Полоса доставки: доставлено, заблокировали бота, ошибка Telegram. */
function DeliveryBar({ item, className }: { item: Item; className?: string }) {
  const total = Math.max(item.total, item.sent + notDelivered(item), 1)
  const parts = [
    { value: item.sent, cls: 'bg-emerald-500' },
    { value: item.blocked, cls: 'bg-amber-500' },
    { value: item.failed, cls: 'bg-destructive' },
  ]
  return (
    <div className={cn('flex h-2 gap-0.5 overflow-hidden rounded-full bg-muted', className)} aria-hidden>
      {parts
        .filter((p) => p.value > 0)
        .map((p, i) => (
          <span key={i} className={cn('h-full', p.cls)} style={{ width: `${(p.value / total) * 100}%` }} />
        ))}
    </div>
  )
}

/** Счётчики под полосой. Причины недоставки — только если они были. */
function DeliveryLegend({ item }: { item: Item }) {
  const { t, i18n } = useTranslation()
  const n = (v: number) => v.toLocaleString(i18n.language)
  const rows = [
    { show: true, label: t('admin.broadcast.history.delivered'), value: item.sent, dot: 'bg-emerald-500' },
    { show: item.blocked > 0, label: t('admin.broadcast.history.blocked'), value: item.blocked, dot: 'bg-amber-500' },
    { show: item.failed > 0, label: t('admin.broadcast.history.tgError'), value: item.failed, dot: 'bg-destructive' },
  ]
  return (
    <div className="flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">
      {rows
        .filter((r) => r.show)
        .map((r) => (
          <span key={r.label} className="inline-flex items-center gap-1.5">
            <span className={cn('size-2 rounded-full', r.dot)} aria-hidden />
            {r.label}
            <b className="font-semibold tabular-nums text-foreground">{n(r.value)}</b>
          </span>
        ))}
    </div>
  )
}

export function BroadcastStatusPill({ item }: { item: Item }) {
  const { t } = useTranslation()
  const tone =
    item.status === 'running'
      ? 'bg-primary/15 text-primary'
      : item.status === 'done'
        ? 'bg-emerald-500/15 text-emerald-600 dark:text-emerald-400'
        : item.status === 'interrupted'
          ? 'bg-amber-500/15 text-amber-600 dark:text-amber-400'
          : 'bg-destructive/15 text-destructive'
  return (
    <span className={cn('inline-block whitespace-nowrap rounded-full px-2 py-0.5 text-[11px] font-semibold', tone)}>
      {isTest(item) && item.status === 'done'
        ? t('admin.broadcast.history.test')
        : t(`admin.broadcast.history.status.${item.status}`)}
    </span>
  )
}

/**
 * Ход и итог последней рассылки — карточкой над формой.
 *
 * Раньше итог приходил только сообщением админу в Telegram, и из кабинета
 * нельзя было понять даже то, дошла ли рассылка до конца.
 */
export function BroadcastResultCard({
  item,
  audienceLabels,
  onDetails,
  onDismiss,
}: {
  item: Item
  audienceLabels: Record<string, string>
  onDetails: () => void
  onDismiss: () => void
}) {
  const { t, i18n } = useTranslation()
  const n = (v: number) => v.toLocaleString(i18n.language)
  const done = item.sent + notDelivered(item)

  if (item.status === 'running') {
    return (
      <div role="status" className="space-y-2.5 rounded-lg border border-primary/40 bg-card p-4">
        <div className="flex items-center gap-3">
          <span className="relative flex size-2.5 shrink-0" aria-hidden>
            <span className="absolute inline-flex size-full animate-ping rounded-full bg-primary opacity-60" />
            <span className="relative inline-flex size-2.5 rounded-full bg-primary" />
          </span>
          <div className="min-w-0">
            <p className="text-sm font-semibold">{t('admin.broadcast.history.runningTitle')}</p>
            <p className="text-xs tabular-nums text-muted-foreground">
              {t('admin.broadcast.history.runningProgress', {
                done: n(done),
                total: n(item.total),
                failed: n(notDelivered(item)),
              })}
            </p>
          </div>
        </div>
        <div className="h-2 overflow-hidden rounded-full bg-muted" aria-hidden>
          <span
            className="block h-full rounded-full bg-primary transition-[width] duration-500"
            style={{ width: `${percent(done, item.total)}%` }}
          />
        </div>
      </div>
    )
  }

  const bad = item.status === 'aborted' || item.status === 'failed'
  const Icon = bad ? XCircle : item.status === 'interrupted' ? AlertTriangle : CheckCircle2
  const when = formatWhen(item.finished_at ?? item.started_at, i18n.language)
  const duration = formatDuration(item, t)

  return (
    <div
      role="status"
      className={cn(
        'space-y-3 rounded-lg border bg-card p-4',
        bad ? 'border-destructive/40' : item.status === 'interrupted' ? 'border-amber-500/40' : 'border-emerald-500/40',
      )}
    >
      <div className="flex items-start gap-3">
        <span
          className={cn(
            'grid size-9 shrink-0 place-items-center rounded-lg',
            bad
              ? 'bg-destructive/15 text-destructive'
              : item.status === 'interrupted'
                ? 'bg-amber-500/15 text-amber-500'
                : 'bg-emerald-500/15 text-emerald-500',
          )}
        >
          <Icon className="size-5" />
        </span>
        <div className="min-w-0 flex-1">
          <p className="text-sm font-semibold">
            {t(`admin.broadcast.history.resultTitle.${item.status}`)} · {when.time}
          </p>
          <p className="truncate text-xs text-muted-foreground">
            {audienceText(item, audienceLabels)}
            {duration ? ` · ${duration}` : ''}
          </p>
        </div>
        <button
          type="button"
          onClick={onDismiss}
          className="shrink-0 rounded-md p-1 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
          aria-label={t('admin.broadcast.history.dismiss')}
        >
          <X className="size-4" />
        </button>
      </div>

      {item.status === 'failed' ? (
        <p className="text-sm text-muted-foreground">{t('admin.broadcast.history.failedHint')}</p>
      ) : (
        <>
          <p className="flex flex-wrap items-baseline gap-x-2">
            <span className="font-heading text-2xl font-extrabold tabular-nums">{n(item.sent)}</span>
            <span className="text-sm text-muted-foreground">
              {t('admin.broadcast.history.deliveredOf', {
                total: n(item.total),
                percent: percent(item.sent, item.total),
              })}
            </span>
          </p>
          <DeliveryBar item={item} />
          <DeliveryLegend item={item} />
          {item.status === 'aborted' && (
            <p className="text-xs text-destructive">{t('admin.broadcast.history.abortedHint')}</p>
          )}
          {item.status === 'interrupted' && (
            <p className="text-xs text-amber-600 dark:text-amber-400">
              {t('admin.broadcast.history.interruptedHint')}
            </p>
          )}
        </>
      )}

      <button
        type="button"
        onClick={onDetails}
        className="inline-flex items-center gap-1.5 rounded-lg border border-border/60 bg-muted/60 px-3 py-1.5 text-sm font-semibold transition-colors hover:bg-accent"
      >
        {t('admin.broadcast.history.details')}
      </button>
    </div>
  )
}

/** Список рассылок: таблица от md, карточки на телефоне. */
export function BroadcastHistoryList({
  items,
  isLoading,
  hasMore,
  loadingMore,
  onLoadMore,
  onOpen,
  audienceLabels,
}: {
  items: Item[]
  isLoading: boolean
  hasMore: boolean
  loadingMore: boolean
  onLoadMore: () => void
  onOpen: (item: Item) => void
  audienceLabels: Record<string, string>
}) {
  const { t, i18n } = useTranslation()
  const n = (v: number) => v.toLocaleString(i18n.language)

  if (isLoading) {
    return (
      <div className="flex justify-center rounded-lg border border-border/50 bg-card py-10">
        <span className="size-5 animate-spin rounded-full border-2 border-primary border-t-transparent" />
      </div>
    )
  }
  if (!items.length) {
    return (
      <div className="flex flex-col items-center gap-2 rounded-lg border border-border/50 bg-card px-6 py-10 text-center">
        <History className="size-6 text-muted-foreground" />
        <p className="text-sm font-semibold">{t('admin.broadcast.history.emptyTitle')}</p>
        <p className="max-w-sm text-xs text-muted-foreground">{t('admin.broadcast.history.emptyHint')}</p>
      </div>
    )
  }

  const more = hasMore && (
    <div className="border-t border-border/50 px-4 py-3">
      <button
        type="button"
        onClick={onLoadMore}
        disabled={loadingMore}
        className="text-sm font-semibold text-primary disabled:opacity-60"
      >
        {loadingMore ? t('admin.broadcast.previewLoading') : t('admin.broadcast.history.loadMore')}
      </button>
    </div>
  )

  return (
    <div className="overflow-hidden rounded-lg border border-border/50 bg-card">
      {/* Телефон: карточки — в таблице на 390 px не помещается ни одна колонка целиком. */}
      <ul className="divide-y divide-border/50 md:hidden">
        {items.map((item) => {
          const when = formatWhen(item.started_at, i18n.language)
          return (
            <li key={item.id}>
              <button
                type="button"
                onClick={() => onOpen(item)}
                className="grid w-full gap-1.5 px-4 py-3 text-left transition-colors hover:bg-accent/40"
              >
                <span className="flex items-center gap-2 text-xs text-muted-foreground">
                  {when.date}, {when.time}
                  <span className="ml-auto">
                    <BroadcastStatusPill item={item} />
                  </span>
                </span>
                <span className="truncate text-sm font-semibold">{itemTitle(item, t)}</span>
                <span className="truncate text-xs text-muted-foreground">{audienceText(item, audienceLabels)}</span>
                <span className="flex items-center gap-2 text-xs">
                  <b className="font-semibold tabular-nums">
                    {n(item.sent)} / {n(item.total)}
                  </b>
                  <DeliveryBar item={item} className="h-1.5 flex-1" />
                </span>
              </button>
            </li>
          )
        })}
      </ul>

      <table className="hidden w-full text-sm md:table">
        <thead>
          <tr className="border-b border-border/50 text-left text-[11px] uppercase tracking-[0.06em] text-muted-foreground">
            <th className="px-4 py-2.5 font-semibold">{t('admin.broadcast.history.colWhen')}</th>
            <th className="px-4 py-2.5 font-semibold">{t('admin.broadcast.history.colMessage')}</th>
            <th className="px-4 py-2.5 font-semibold">{t('admin.broadcast.history.colAudience')}</th>
            <th className="px-4 py-2.5 font-semibold">{t('admin.broadcast.history.colDelivered')}</th>
            <th className="px-4 py-2.5 font-semibold">{t('admin.broadcast.history.colStatus')}</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-border/50">
          {items.map((item) => {
            const when = formatWhen(item.started_at, i18n.language)
            return (
              <tr
                key={item.id}
                tabIndex={0}
                onClick={() => onOpen(item)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter' || e.key === ' ') {
                    e.preventDefault()
                    onOpen(item)
                  }
                }}
                className="cursor-pointer transition-colors hover:bg-accent/40 focus-visible:bg-accent/40 focus-visible:outline-none"
              >
                <td className="whitespace-nowrap px-4 py-3">
                  <span className="block font-semibold">{when.date}</span>
                  <span className="text-xs text-muted-foreground">{when.time}</span>
                </td>
                <td className="max-w-[18rem] px-4 py-3">
                  <span className="block truncate font-semibold">{itemTitle(item, t)}</span>
                  {item.source === 'telegram' && (
                    <span className="text-xs text-muted-foreground">{t('admin.broadcast.history.fromTelegram')}</span>
                  )}
                </td>
                <td className="max-w-[14rem] px-4 py-3 text-muted-foreground">
                  <span className="line-clamp-2">{audienceText(item, audienceLabels)}</span>
                </td>
                <td className="whitespace-nowrap px-4 py-3 tabular-nums">
                  <b className="font-semibold">{n(item.sent)}</b>
                  <span className="text-muted-foreground"> / {n(item.total)}</span>
                  <DeliveryBar item={item} className="mt-1.5 h-1.5 w-28" />
                </td>
                <td className="px-4 py-3">
                  <BroadcastStatusPill item={item} />
                </td>
              </tr>
            )
          })}
        </tbody>
      </table>
      {more}
    </div>
  )
}

/** Подробности рассылки: цифры, сообщение как его увидели, «взять за основу». */
export function BroadcastHistoryDetail({
  item,
  onClose,
  onReuse,
  replacesDraft,
  audienceLabels,
}: {
  item: Item | null
  onClose: () => void
  onReuse: (item: Item) => void
  /** В форме уже что-то набрано — «взять за основу» это заменит, о чём стоит сказать заранее. */
  replacesDraft: boolean
  audienceLabels: Record<string, string>
}) {
  const { t, i18n } = useTranslation()
  if (!item) return null
  const n = (v: number) => v.toLocaleString(i18n.language)
  const when = formatWhen(item.started_at, i18n.language)
  const duration = formatDuration(item, t)
  const buttons = normalizeBroadcastButtons(item.buttons)
  const buttonLabels = [
    buttons.buy ? t('admin.broadcast.buttons.buy') : null,
    buttons.connect ? t('admin.broadcast.buttons.connect') : null,
    ...BROADCAST_LINK_KEYS.filter((key) => buttons.links.includes(key)).map((key) => t(broadcastLinkLabelKey(key))),
    buttons.promo ? t('admin.broadcast.buttons.promo') : null,
    buttons.share_referral ? t('admin.broadcast.buttons.shareReferral') : null,
    buttons.main_menu ? t('admin.broadcast.buttons.mainMenu') : null,
  ].filter(Boolean) as string[]
  const fail = notDelivered(item)

  const stats = [
    { label: t('admin.broadcast.history.recipients'), value: n(item.total), cls: '' },
    { label: t('admin.broadcast.history.delivered'), value: n(item.sent), cls: 'text-emerald-500' },
    { label: t('admin.broadcast.history.notDelivered'), value: n(fail), cls: fail ? 'text-destructive' : '' },
    { label: t('admin.broadcast.history.duration'), value: duration ?? '—', cls: '' },
  ]

  return (
    <AdminModal
      open
      onClose={onClose}
      size="md"
      icon={History}
      title={t('admin.broadcast.history.detailTitle', { date: `${when.date}, ${when.time}` })}
      description={itemTitle(item, t)}
      footer={
        <div className="flex flex-col-reverse gap-2 sm:flex-row sm:items-center sm:justify-end">
          {replacesDraft && (
            <p className="text-center text-xs text-muted-foreground sm:mr-auto sm:text-left">
              {t('admin.broadcast.history.reuseReplaces')}
            </p>
          )}
          <button
            type="button"
            onClick={onClose}
            className="rounded-lg border border-border/60 px-4 py-2 text-sm font-semibold transition-colors hover:bg-accent"
          >
            {t('common.close')}
          </button>
          <button
            type="button"
            onClick={() => onReuse(item)}
            className="inline-flex items-center justify-center gap-2 rounded-lg bg-primary px-4 py-2 text-sm font-semibold text-primary-foreground transition-colors hover:bg-primary/90"
          >
            <Copy className="size-4" />
            {t('admin.broadcast.history.reuse')}
          </button>
        </div>
      }
    >
      <div className="space-y-4">
        <div className="flex items-center gap-2">
          <BroadcastStatusPill item={item} />
          {item.source === 'telegram' && (
            <span className="text-xs text-muted-foreground">{t('admin.broadcast.history.fromTelegram')}</span>
          )}
        </div>

        <dl className="grid grid-cols-2 gap-2 sm:grid-cols-4">
          {stats.map((s) => (
            <div key={s.label} className="rounded-lg bg-muted/60 px-3 py-2">
              <dt className="text-[11px] text-muted-foreground">{s.label}</dt>
              <dd className={cn('font-heading text-lg font-extrabold tabular-nums', s.cls)}>{s.value}</dd>
            </div>
          ))}
        </dl>

        {item.status !== 'failed' && (
          <div className="space-y-2">
            <DeliveryBar item={item} />
            <DeliveryLegend item={item} />
          </div>
        )}
        {item.status === 'aborted' && <p className="text-xs text-destructive">{t('admin.broadcast.history.abortedHint')}</p>}
        {item.status === 'interrupted' && (
          <p className="text-xs text-amber-600 dark:text-amber-400">{t('admin.broadcast.history.interruptedHint')}</p>
        )}
        {item.status === 'failed' && <p className="text-xs text-destructive">{t('admin.broadcast.history.failedHint')}</p>}

        <div className="space-y-2">
          <p className="text-[11px] font-semibold uppercase tracking-[0.06em] text-muted-foreground">
            {t('admin.broadcast.history.message')}
          </p>
          <AdminBroadcastMessagePreview
            html={broadcastItemHtml(item)}
            mediaKind={item.media_kind || undefined}
            mediaPlaceholder={
              item.media_kind
                ? t(
                    item.media_name
                      ? 'admin.broadcast.history.mediaPlaceholder'
                      : 'admin.broadcast.history.mediaPlaceholderNoName',
                    {
                      kind: t(`admin.broadcast.history.mediaKind.${item.media_kind}`),
                      name: item.media_name,
                    },
                  )
                : undefined
            }
            buttons={buttons}
          />
          {item.source === 'telegram' && (
            <p className="text-xs text-muted-foreground">{t('admin.broadcast.history.plainTextNote')}</p>
          )}
        </div>

        <dl className="grid grid-cols-[minmax(0,7.5rem)_minmax(0,1fr)] gap-x-3 gap-y-1.5 text-sm">
          <dt className="text-muted-foreground">{t('admin.broadcast.audienceTitle')}</dt>
          <dd className="font-semibold">{audienceText(item, audienceLabels)}</dd>
          <dt className="text-muted-foreground">{t('admin.broadcast.buttonsTitle')}</dt>
          <dd className="font-semibold">{buttonLabels.length ? buttonLabels.join(', ') : t('admin.broadcast.buttons.none')}</dd>
          <dt className="text-muted-foreground">{t('admin.broadcast.attachment')}</dt>
          <dd className="font-semibold">
            {item.media_kind
              ? `${t(`admin.broadcast.history.mediaKind.${item.media_kind}`)}${item.media_name ? ` · ${item.media_name}` : ''}`
              : t('admin.broadcast.attachmentNone')}
          </dd>
        </dl>
      </div>
    </AdminModal>
  )
}
