import { useEffect, useMemo, useRef, useState } from 'react'
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  AlertCircle,
  AlertTriangle,
  Check,
  Eye,
  Film,
  History,
  Image as ImageIcon,
  ImagePlus,
  Megaphone,
  PanelBottom,
  Paperclip,
  Send,
  Users,
  UsersRound,
  X,
  type LucideIcon,
} from 'lucide-react'

import { api } from '@/lib/api'
import { cn } from '@/lib/utils'
import type { AdminBroadcastHistoryItemDTO, AdminBroadcastMediaKind } from '@/lib/types/admin'

import { AdminLayout } from '../layout/AdminLayout'
import { AdminPageHeader } from '../components/AdminPageHeader'
import { AdminFeedback } from '../components/AdminFeedback'
import { AdminConfirmModal } from '../components/AdminConfirmModal'
import { AdminBroadcastMessagePreview } from '../components/AdminBroadcastMessagePreview'
import { AdminSelect } from '../components/AdminSelect'
import { TelegramHtmlEditor } from '../components/TelegramHtmlEditor'
import { surface } from '../components/Surface'
import { BroadcastAudienceSelector } from '../components/BroadcastAudienceSelector'
import {
  BroadcastButtonsPicker,
  type BroadcastButtonsState,
} from '../components/BroadcastButtonsPicker'
import { useAdminBootstrap } from '../hooks/useAdminBootstrap'
import { useAdminShell } from '../layout/AdminShellContext'
import { formatAdminApiError } from '../utils/formatAdminApiError'
import { BROADCAST_LINK_KEYS, broadcastLinkLabelKey } from '../utils/broadcastLinks'
import { telegramTextLength } from '../utils/telegramHtml'
import {
  clearBroadcastDraft,
  loadBroadcastDraft,
  normalizeBroadcastButtons,
  saveBroadcastDraft,
} from '../utils/broadcastDraft'
import {
  BroadcastHistoryDetail,
  BroadcastHistoryList,
  BroadcastResultCard,
  broadcastItemHtml,
} from '../components/BroadcastHistory'

interface AudienceItem {
  audience: string
  label: string
  count: number
}

interface UploadedMedia {
  file_id: string
  kind: AdminBroadcastMediaKind
  previewUrl: string
  name: string
}

/**
 * Лимиты Telegram на длину текста. Подпись к вложению короче обычного
 * сообщения вчетверо, и узнавать об этом на отправке — худший момент.
 */
const LIMIT_TEXT = 4096
const LIMIT_CAPTION = 1024

const defaultButtons: BroadcastButtonsState = {
  buy: false,
  connect: false,
  promo: false,
  main_menu: false,
  share_referral: false,
  links: [],
}

function useAudiences() {
  return useQuery<{ audiences: AudienceItem[] }>({
    queryKey: ['admin-broadcast-audiences'],
    queryFn: () => api.adminBroadcastAudiences(),
  })
}

function useBroadcastTariffs(enabled: boolean) {
  return useQuery<{ tariffs: { id: number; name: string; slug: string }[] }>({
    queryKey: ['admin-broadcast-tariffs'],
    queryFn: () => api.adminBroadcastTariffs(),
    enabled,
  })
}

/**
 * Число получателей считает сервер: у платных сегментов оно зависит от фильтра
 * по тарифу, а при нескольких сегментах один человек может быть в двух сразу —
 * сумма цифр из списка аудиторий тогда врёт. Запрос идёт сам при смене выбора:
 * раньше для этого была отдельная кнопка «Предпросмотр», и до подтверждения
 * отправки админ не знал, скольким людям уйдёт сообщение.
 */
function useRecipientCount(audiences: string[], tariffId: number | null) {
  return useQuery({
    queryKey: ['admin-broadcast-count', audiences, tariffId],
    queryFn: () => api.adminBroadcastPreview({ audiences, tariff_id: tariffId }),
    enabled: audiences.length > 0,
    staleTime: 30_000,
  })
}

const PAID_AUDIENCES = ['active_paid', 'inactive_paid']

const HISTORY_PAGE_SIZE = 20
const HISTORY_KEY = ['admin-broadcast-history'] as const

function useBroadcastHistory() {
  return useInfiniteQuery({
    queryKey: HISTORY_KEY,
    queryFn: ({ pageParam }) => api.adminBroadcastHistory({ limit: HISTORY_PAGE_SIZE, offset: pageParam }),
    initialPageParam: 0,
    getNextPageParam: (lastPage, pages) => {
      const loaded = pages.reduce((n, p) => n + p.items.length, 0)
      return loaded < lastPage.total ? loaded : undefined
    },
    staleTime: 15_000,
  })
}

/**
 * Рассылка, за которой следит карточка над формой. Пока она идёт, страница
 * спрашивает о ней раз в две секунды: пачки уходят раз в секунду, чаще не
 * изменится ничего.
 */
function useWatchedBroadcast(id: number | null) {
  return useQuery({
    queryKey: ['admin-broadcast-history-item', id],
    queryFn: () => api.adminBroadcastHistoryItem(id as number),
    enabled: id !== null,
    refetchInterval: (query) => (query.state.data?.status === 'running' ? 2000 : false),
  })
}

/** compose и preview — вкладки формы на телефоне; history — список рассылок везде. */
type BroadcastView = 'compose' | 'preview' | 'history'

export default function AdminBroadcastPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const fileInputRef = useRef<HTMLInputElement>(null)
  const { data: bootstrap } = useAdminBootstrap()
  const { data, isLoading } = useAudiences()

  // Черновик читается один раз: дальше форма — источник правды, а хранилище догоняет её.
  const [draft] = useState(loadBroadcastDraft)
  const [selectedAudiences, setSelectedAudiences] = useState<string[]>(
    draft?.audiences.length ? draft.audiences : ['active_all'],
  )
  const [selectedTariff, setSelectedTariff] = useState<number | null>(draft?.tariffId ?? null)
  const [html, setHtml] = useState(draft?.html ?? '')
  const [editorSeed, setEditorSeed] = useState(draft?.html ?? '')
  const [buttons, setButtons] = useState<BroadcastButtonsState>(draft?.buttons ?? defaultButtons)
  // previewUrl пустой: картинка предпросмотра жила в памяти прошлой вкладки.
  const [media, setMedia] = useState<UploadedMedia | null>(
    draft?.media ? { ...draft.media, previewUrl: '' } : null,
  )
  const [draftSaved, setDraftSaved] = useState(Boolean(draft))
  const [confirmOpen, setConfirmOpen] = useState(false)
  const [sendSuccess, setSendSuccess] = useState<string | null>(null)
  const [view, setView] = useState<BroadcastView>('compose')
  const [resetKey, setResetKey] = useState(0)
  const [watchedId, setWatchedId] = useState<number | null>(null)
  const [detailItem, setDetailItem] = useState<AdminBroadcastHistoryItemDTO | null>(null)

  const history = useBroadcastHistory()
  const { data: watched } = useWatchedBroadcast(watchedId)
  // Строка идущей рассылки берёт свежие цифры из опроса: список перечитывается
  // реже, и без этого карточка и строка показывали бы разное.
  const historyItems = useMemo(
    () =>
      (history.data?.pages.flatMap((p) => p.items) ?? []).map((item) =>
        watched && item.id === watched.id ? watched : item,
      ),
    [history.data, watched],
  )
  const historyTotal = history.data?.pages[0]?.total ?? 0

  /*
   * Рассылку, запущенную до перезагрузки страницы (или из Telegram-админки),
   * подхватываем из истории: иначе после F5 о ней в кабинете ни слова до
   * самого конца.
   */
  const pickedRunningRef = useRef(false)
  useEffect(() => {
    if (pickedRunningRef.current || !history.data) return
    pickedRunningRef.current = true
    const latest = history.data.pages[0]?.items[0]
    if (latest?.status === 'running') setWatchedId(latest.id)
  }, [history.data])

  // Рассылка закончилась — строка в списке тоже должна показать итог.
  const watchedStatus = watched?.status
  useEffect(() => {
    if (watchedStatus && watchedStatus !== 'running') {
      queryClient.invalidateQueries({ queryKey: HISTORY_KEY })
    }
  }, [watchedStatus, queryClient])

  const isTariffsMode = bootstrap?.sales_mode === 'tariffs'
  const { data: tariffsData } = useBroadcastTariffs(isTariffsMode)
  const hasAudience = selectedAudiences.length > 0
  const hasPaidAudience = selectedAudiences.some((aud) => PAID_AUDIENCES.includes(aud))
  const { data: countData, isFetching: countLoading } = useRecipientCount(selectedAudiences, selectedTariff)

  const recipientCount = hasAudience ? (countData?.recipient_count ?? null) : 0
  const hasContent = Boolean(html.trim() || media)
  const limit = media ? LIMIT_CAPTION : LIMIT_TEXT
  const length = useMemo(() => telegramTextLength(html), [html])
  const overLimit = length > limit

  const uploadMutation = useMutation({
    mutationFn: (file: File) => api.adminBroadcastUploadMedia(file),
    onSuccess: (res, file) => {
      const previewUrl = URL.createObjectURL(file)
      setMedia((prev) => {
        if (prev?.previewUrl) URL.revokeObjectURL(prev.previewUrl)
        return { file_id: res.file_id, kind: res.kind, previewUrl, name: file.name }
      })
      setSendSuccess(null)
    },
  })

  const sendMutation = useMutation({
    mutationFn: () =>
      api.adminBroadcastSend({
        audiences: selectedAudiences,
        tariff_id: selectedTariff,
        text: html,
        buttons,
        media: media ? { file_id: media.file_id, kind: media.kind, name: media.name } : null,
      }),
    onSuccess: (res) => {
      setConfirmOpen(false)
      clearBroadcastDraft()
      setHtml('')
      setEditorSeed('')
      setResetKey((v) => v + 1)
      setButtons(defaultButtons)
      if (media?.previewUrl) URL.revokeObjectURL(media.previewUrl)
      setMedia(null)
      // Ход и итог покажет карточка; строка «запущена» — только если история не записалась.
      if (res.broadcast_id) {
        setWatchedId(res.broadcast_id)
        setSendSuccess(null)
      } else {
        setSendSuccess(t('admin.broadcast.started', { count: res.recipient_count }))
      }
      queryClient.invalidateQueries({ queryKey: ['admin-broadcast-audiences'] })
      queryClient.invalidateQueries({ queryKey: HISTORY_KEY })
    },
    onError: () => setConfirmOpen(false),
  })

  const audienceLabels: Record<string, string> = {
    all: t('admin.broadcast.audience.all'),
    test_broadcast: t('admin.broadcast.audience.testBroadcast', 'Test Broadcast'),
    active_all: t('admin.broadcast.audience.activeAll'),
    active_paid: t('admin.broadcast.audience.activePaid'),
    active_trial: t('admin.broadcast.audience.activeTrial'),
    inactive_all: t('admin.broadcast.audience.inactiveAll'),
    inactive_paid: t('admin.broadcast.audience.inactivePaid'),
    inactive_trial: t('admin.broadcast.audience.inactiveTrial'),
  }
  const selectedAudienceLabels = selectedAudiences.map((aud) => audienceLabels[aud] ?? aud)

  const selectedButtonLabels = [
    buttons.buy ? t('admin.broadcast.buttons.buy') : null,
    buttons.connect ? t('admin.broadcast.buttons.connect') : null,
    ...BROADCAST_LINK_KEYS.filter((key) => buttons.links.includes(key)).map((key) =>
      t(broadcastLinkLabelKey(key)),
    ),
    buttons.promo ? t('admin.broadcast.buttons.promo') : null,
    buttons.share_referral ? t('admin.broadcast.buttons.shareReferral') : null,
    buttons.main_menu ? t('admin.broadcast.buttons.mainMenu') : null,
  ].filter(Boolean) as string[]

  /** «Взять за основу»: форма получает текст, аудитории, кнопки и вложение рассылки. */
  function reuseBroadcast(item: AdminBroadcastHistoryItemDTO) {
    const text = broadcastItemHtml(item)
    setHtml(text)
    setEditorSeed(text)
    setResetKey((v) => v + 1)
    setSelectedAudiences(item.audiences.length ? item.audiences : ['active_all'])
    setSelectedTariff(item.tariff_id)
    setButtons(normalizeBroadcastButtons(item.buttons))
    if (media?.previewUrl) URL.revokeObjectURL(media.previewUrl)
    setMedia(
      item.media_kind && item.media_file_id
        ? { file_id: item.media_file_id, kind: item.media_kind, name: item.media_name, previewUrl: '' }
        : null,
    )
    setDetailItem(null)
    setSendSuccess(null)
    setView('compose')
    window.scrollTo({ top: 0, behavior: 'smooth' })
  }

  function clearMedia() {
    if (media?.previewUrl) URL.revokeObjectURL(media.previewUrl)
    setMedia(null)
    setSendSuccess(null)
  }

  const canSend = hasAudience && hasContent && !overLimit && !sendMutation.isPending

  /*
   * Запись с задержкой: на каждое нажатие клавиши переписывать localStorage
   * незачем, а полсекунды — меньше, чем нужно, чтобы успеть закрыть вкладку.
   */
  useEffect(() => {
    const timer = window.setTimeout(() => {
      setDraftSaved(
        saveBroadcastDraft({
          html,
          audiences: selectedAudiences,
          tariffId: selectedTariff,
          buttons,
          media: media ? { file_id: media.file_id, kind: media.kind, name: media.name } : null,
        }) && hasContent,
      )
    }, 500)
    return () => window.clearTimeout(timer)
  }, [html, selectedAudiences, selectedTariff, buttons, media, hasContent])

  const preview = (
    <div className="rounded-lg border border-border/50 bg-card">
      <h3 className="flex items-center gap-2 border-b border-border/50 px-4 py-3 text-sm font-semibold">
        <Eye className="size-4 text-muted-foreground" />
        {t('admin.broadcast.previewTitle')}
      </h3>
      <div className="space-y-3 p-4">
        <AdminBroadcastMessagePreview
          html={html}
          mediaUrl={media?.previewUrl}
          mediaKind={media?.kind}
          mediaPlaceholder={
            media && !media.previewUrl
              ? t('admin.broadcast.previewMediaFromDraft', { name: media.name || '—' })
              : undefined
          }
          buttons={buttons}
        />

        <dl className="space-y-1 text-sm">
          <SummaryRow label={t('admin.broadcast.audienceTitle')}>
            {/* Переносом, а не многоточием: сегментов теперь несколько, и
                обрезанный хвост списка прятал как раз второй из них. */}
            <span className={cn('block whitespace-normal', !hasAudience && 'text-destructive')}>
              {hasAudience ? selectedAudienceLabels.join(', ') : t('admin.broadcast.audienceNone')}
            </span>
          </SummaryRow>
          <SummaryRow label={t('admin.broadcast.recipients')}>
            {hasAudience && (countLoading || recipientCount === null) ? '…' : recipientCount}
          </SummaryRow>
          <SummaryRow label={t('admin.broadcast.buttonsTitle')}>
            {selectedButtonLabels.length || t('admin.broadcast.buttons.none')}
          </SummaryRow>
        </dl>

        <button
          type="button"
          onClick={() => setConfirmOpen(true)}
          disabled={!canSend}
          className="inline-flex w-full items-center justify-center gap-2 rounded-lg bg-primary px-4 py-2.5 text-sm font-semibold text-primary-foreground transition-colors hover:bg-primary/90 disabled:opacity-50"
        >
          <Send className="size-4" />
          {t('admin.broadcast.send')}
        </button>
        <p className="text-center text-[11px] text-muted-foreground">
          {t('admin.broadcast.previewButtonsHint')}
        </p>
      </div>
    </div>
  )

  return (
    <AdminLayout>
      <div className="space-y-5 pb-20 lg:pb-0">
        <AdminPageHeader
          icon={Megaphone}
          title={t('admin.broadcast.title')}
          subtitle={t('admin.broadcast.subtitle')}
          accent="indigo"
          actions={
            // На ПК история — отдельный режим страницы; на телефоне — третья вкладка.
            <div className={surface('well', 'hidden gap-1 rounded-xl p-1 lg:flex')} role="tablist">
              {(
                [
                  ['compose', t('admin.broadcast.history.newBroadcast')],
                  ['history', t('admin.broadcast.history.tab')],
                ] as const
              ).map(([key, label]) => {
                const active = key === 'history' ? view === 'history' : view !== 'history'
                return (
                  <button
                    key={key}
                    type="button"
                    role="tab"
                    aria-selected={active}
                    onClick={() => setView(key)}
                    className={cn(
                      'inline-flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm font-semibold transition-colors',
                      active ? 'bg-card text-foreground shadow-sm' : 'text-muted-foreground hover:text-foreground',
                    )}
                  >
                    {key === 'history' && <History className="size-4" />}
                    {label}
                    {key === 'history' && historyTotal > 0 && (
                      <span className="text-xs tabular-nums text-muted-foreground">{historyTotal}</span>
                    )}
                  </button>
                )
              })}
            </div>
          }
        />

        {sendSuccess && (
          <AdminFeedback mode="inline" feedback={{ type: 'success', message: sendSuccess }} />
        )}

        {/* На узком экране форма и предпросмотр — две вкладки: иначе за
            результатом приходится листать через весь экран и обратно. */}
        <StickyMobileTabs>
          <div className={surface('well', 'flex gap-1 rounded-xl p-1')}>
            {(['compose', 'preview', 'history'] as const).map((tab) => (
              <button
                key={tab}
                type="button"
                role="tab"
                aria-selected={view === tab}
                onClick={() => setView(tab)}
                className={cn(
                  'flex min-w-0 flex-1 items-center justify-center gap-1.5 rounded-lg px-2 py-2 text-sm font-semibold transition-colors',
                  view === tab ? 'bg-card text-foreground shadow-sm' : 'text-muted-foreground hover:text-foreground',
                )}
              >
                <span className="truncate">
                  {tab === 'compose'
                    ? t('admin.broadcast.compose')
                    : tab === 'preview'
                      ? // Короткое слово: «Предпросмотр» третьей вкладкой на 390 px не помещается.
                        t('admin.broadcast.previewTab')
                      : t('admin.broadcast.history.tab')}
                </span>
              </button>
            ))}
          </div>
        </StickyMobileTabs>

        {watched && watchedId !== null && (
          <BroadcastResultCard
            item={watched}
            audienceLabels={audienceLabels}
            onDetails={() => setDetailItem(watched)}
            onDismiss={() => setWatchedId(null)}
          />
        )}

        {view === 'history' && (
          <BroadcastHistoryList
            items={historyItems}
            isLoading={history.isLoading}
            hasMore={Boolean(history.hasNextPage)}
            loadingMore={history.isFetchingNextPage}
            onLoadMore={() => history.fetchNextPage()}
            onOpen={setDetailItem}
            audienceLabels={audienceLabels}
          />
        )}

        <div
          className={cn(
            'grid gap-5 lg:grid-cols-[minmax(0,1fr)_320px] lg:items-start',
            view === 'history' && 'hidden',
          )}
        >
          <div className={cn('min-w-0 space-y-4', view === 'preview' && 'hidden lg:block')}>
            <BroadcastAudienceSelector
              audiences={data?.audiences ?? []}
              isLoading={isLoading}
              selectedAudiences={selectedAudiences}
              onChange={(next) => {
                setSelectedAudiences(next)
                if (!next.some((aud) => PAID_AUDIENCES.includes(aud))) setSelectedTariff(null)
                setSendSuccess(null)
              }}
              audienceLabels={audienceLabels}
              recipientCount={countLoading ? null : recipientCount}
            >
              {isTariffsMode && hasPaidAudience && (
                <>
                  <label className="mb-1.5 block text-xs text-muted-foreground">
                    {t('admin.broadcast.filterTariff')}
                  </label>
                  <AdminSelect<number>
                    value={selectedTariff}
                    allowEmpty
                    emptyLabel={t('admin.broadcast.allTariffs')}
                    placeholder={t('admin.broadcast.allTariffs')}
                    ariaLabel={t('admin.broadcast.filterTariff')}
                    options={(tariffsData?.tariffs ?? []).map((tariff) => ({
                      value: tariff.id,
                      label: tariff.name,
                    }))}
                    onChange={setSelectedTariff}
                  />
                </>
              )}
            </BroadcastAudienceSelector>

            {/* Сообщение */}
            <div className="overflow-hidden rounded-lg border border-border/50 bg-card">
              <div className="flex items-center gap-3 border-b border-border/50 px-4 py-3">
                <span className="grid size-5 shrink-0 place-items-center rounded-md bg-muted font-mono text-[10px] text-muted-foreground">
                  2
                </span>
                <span className="text-sm font-semibold">{t('admin.broadcast.compose')}</span>
                {draftSaved && (
                  <span className="ml-auto flex min-w-0 items-center gap-1 text-xs text-muted-foreground">
                    <Check className="size-3.5 shrink-0 text-emerald-500" />
                    <span className="truncate">{t('admin.broadcast.draftSaved')}</span>
                  </span>
                )}
                <span
                  className={cn(
                    'shrink-0 text-xs tabular-nums',
                    !draftSaved && 'ml-auto',
                    overLimit ? 'font-semibold text-destructive' : 'text-muted-foreground',
                  )}
                >
                  {length} / {limit}
                </span>
              </div>

              <TelegramHtmlEditor
                resetKey={resetKey}
                initialHtml={editorSeed}
                onChange={(next) => {
                  setHtml(next)
                  setSendSuccess(null)
                }}
                placeholder={t('admin.broadcast.placeholder')}
              />

              <div className="flex flex-wrap items-center gap-2 border-t border-border/50 px-4 py-3">
                <input
                  ref={fileInputRef}
                  type="file"
                  accept="image/jpeg,image/png,image/webp,video/mp4"
                  className="hidden"
                  onChange={(e) => {
                    const file = e.target.files?.[0]
                    if (file) uploadMutation.mutate(file)
                    e.target.value = ''
                  }}
                />
                <button
                  type="button"
                  onClick={() => fileInputRef.current?.click()}
                  disabled={uploadMutation.isPending}
                  className={surface('raised', 'inline-flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm transition-colors hover:bg-accent disabled:opacity-50')}
                >
                  <ImagePlus className="size-4" />
                  {uploadMutation.isPending
                    ? t('admin.broadcast.uploadingMedia')
                    : t('admin.broadcast.attachMedia')}
                </button>
              </div>

              {/* Про MP4 сказано до отправки, а не после: WebM и MOV Telegram
                  видео не считает и молча кладёт их файлом. */}
              <p className="flex items-start gap-2 px-4 pb-3 text-[11px] leading-relaxed text-muted-foreground">
                <AlertTriangle className="mt-0.5 size-3.5 shrink-0 text-amber-500" />
                <span>{t('admin.broadcast.mp4Hint')}</span>
              </p>

              {uploadMutation.isError && (
                <div className="mx-4 mb-4 flex items-start gap-2 rounded-md border border-destructive/40 bg-destructive/10 p-3 text-sm text-destructive">
                  <AlertCircle className="mt-0.5 size-4 shrink-0" />
                  <span>{formatAdminApiError(uploadMutation.error, t)}</span>
                </div>
              )}

              {media && (
                /*
                 * Строкой, а не карточкой с крупным превью: видео без постера
                 * рисовалось чёрным прямоугольником, и блок выглядел пустой
                 * дырой. Сам файл всё равно виден в предпросмотре справа.
                 */
                <div className={surface('well', 'mx-4 mb-4 flex items-center gap-3 rounded-lg px-3 py-2')}>
                  <span
                    className={cn(
                      'grid size-9 shrink-0 place-items-center rounded-lg',
                      media.kind === 'video'
                        ? 'bg-violet-500/15 text-violet-500'
                        : 'bg-sky-500/15 text-sky-500',
                    )}
                  >
                    {media.kind === 'video' ? <Film className="size-4" /> : <ImageIcon className="size-4" />}
                  </span>
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-sm font-medium">{media.name}</span>
                    <span className="block text-xs text-muted-foreground">
                      {media.kind === 'video'
                        ? t('admin.broadcast.videoAttached')
                        : t('admin.broadcast.photoAttached')}
                    </span>
                  </span>
                  <button
                    type="button"
                    onClick={clearMedia}
                    className="shrink-0 rounded-md p-1 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
                    aria-label={t('admin.broadcast.removePhoto')}
                  >
                    <X className="size-4" />
                  </button>
                </div>
              )}
            </div>

            {/* Кнопки */}
            <div className="overflow-hidden rounded-lg border border-border/50 bg-card">
              <div className="flex items-center gap-3 border-b border-border/50 px-4 py-3">
                <span className="grid size-5 shrink-0 place-items-center rounded-md bg-muted font-mono text-[10px] text-muted-foreground">
                  3
                </span>
                <PanelBottom className="size-4 shrink-0 text-muted-foreground" />
                <span className="text-sm font-semibold">{t('admin.broadcast.buttonsTitle')}</span>
                <span className="ml-auto text-xs text-muted-foreground">
                  {selectedButtonLabels.length
                    ? t('admin.broadcast.linksSelected', { count: selectedButtonLabels.length })
                    : t('admin.broadcast.buttons.none')}
                </span>
              </div>
              <div className="p-4">
                <BroadcastButtonsPicker buttons={buttons} onChange={setButtons} />
              </div>
            </div>

            {sendMutation.isError && (
              <div className="flex items-start gap-2 rounded-lg border border-destructive/40 bg-destructive/10 p-3 text-sm text-destructive">
                <AlertCircle className="mt-0.5 size-4 shrink-0" />
                <span>{formatAdminApiError(sendMutation.error, t)}</span>
              </div>
            )}
          </div>

          <div
            className={cn(
              'min-w-0 lg:sticky lg:top-4',
              view === 'compose' && 'hidden lg:block',
            )}
          >
            {preview}
          </div>
        </div>
      </div>

      {/* Панель отправки на телефоне: кнопка всегда под рукой, а не в конце формы. */}
      <div
        className={cn(
          'fixed inset-x-0 bottom-0 z-30 flex items-center gap-3 border-t border-border/60 bg-card/95 px-4 py-2.5 backdrop-blur lg:hidden',
          view === 'history' && 'hidden',
        )}
      >
        <span className="text-xs leading-tight text-muted-foreground">
          {t('admin.broadcast.recipients')}
          <b className="block text-base text-foreground tabular-nums">
            {countLoading || recipientCount === null ? '…' : recipientCount}
          </b>
        </span>
        <button
          type="button"
          onClick={() => setConfirmOpen(true)}
          disabled={!canSend}
          className="ml-auto inline-flex items-center gap-2 rounded-lg bg-primary px-4 py-2.5 text-sm font-semibold text-primary-foreground transition-colors hover:bg-primary/90 disabled:opacity-50"
        >
          <Send className="size-4" />
          {t('admin.broadcast.send')}
        </button>
      </div>

      <BroadcastHistoryDetail
        item={detailItem}
        onClose={() => setDetailItem(null)}
        onReuse={reuseBroadcast}
        replacesDraft={hasContent}
        audienceLabels={audienceLabels}
      />

      <AdminConfirmModal
        open={confirmOpen}
        onClose={() => setConfirmOpen(false)}
        onConfirm={() => sendMutation.mutate()}
        title={t('admin.broadcast.confirmSend')}
        confirmLabel={t('admin.broadcast.send')}
        loading={sendMutation.isPending}
        icon={Megaphone}
        message={
          /*
           * Строками, а не одним предложением: раньше здесь шло слитно
           * «Аудитория: … Получателей: … Кнопки: …», и проверить перед
           * отправкой на пять тысяч человек было нечего — текст не читался.
           */
          <div className="space-y-3">
            <p className="text-sm text-muted-foreground">{t('admin.broadcast.confirmLead')}</p>
            <dl className={surface('raised', 'divide-y divide-border/60 rounded-lg')}>
              <ConfirmRow icon={Users} label={t('admin.broadcast.audienceTitle')}>
                {selectedAudienceLabels.map((name) => (
                  <span key={name} className="block">
                    {name}
                  </span>
                ))}
              </ConfirmRow>
              <ConfirmRow icon={UsersRound} label={t('admin.broadcast.recipients')}>
                {recipientCount === null ? '…' : recipientCount}
              </ConfirmRow>
              <ConfirmRow icon={PanelBottom} label={t('admin.broadcast.buttonsTitle')}>
                {selectedButtonLabels.length
                  ? selectedButtonLabels.join(', ')
                  : t('admin.broadcast.buttons.none')}
              </ConfirmRow>
              <ConfirmRow icon={Paperclip} label={t('admin.broadcast.attachment')}>
                {media ? (
                  <>
                    {media.name}
                    <span className="block text-xs font-normal text-muted-foreground">
                      {media.kind === 'video'
                        ? t('admin.broadcast.videoAttached')
                        : t('admin.broadcast.photoAttached')}
                    </span>
                  </>
                ) : (
                  t('admin.broadcast.attachmentNone')
                )}
              </ConfirmRow>
            </dl>
            <p className="flex items-start gap-2 rounded-lg border border-amber-500/35 bg-amber-500/10 p-2.5 text-xs leading-relaxed">
              <AlertTriangle className="mt-0.5 size-3.5 shrink-0 text-amber-500" />
              <span>{t('admin.broadcast.confirmWebOnly')}</span>
            </p>
          </div>
        }
      />
    </AdminLayout>
  )
}

/**
 * Полоса вкладок липнет под шапку: раньше она уезжала вверх вместе с формой, и
 * чтобы из кнопок под сообщением глянуть предпросмотр, приходилось сначала
 * долистать до начала страницы. Шапка на телефоне прячется при прокрутке вниз
 * (до md), и полоса тогда поднимается к самому краю — так же, как панель
 * периода на странице статистики.
 *
 * Отдельный компонент, потому что useAdminShell работает только внутри
 * AdminLayout, а сама страница его и рисует.
 */
function StickyMobileTabs({ children }: { children: React.ReactNode }) {
  const { mobileHeaderVisible } = useAdminShell()
  return (
    <div
      className={cn(
        'sticky z-40 -mx-3 border-b border-border/80 bg-card/92 px-3 py-2 backdrop-blur-xl transition-[top] duration-200 ease-out sm:-mx-4 sm:px-4 dark:border-primary/12 lg:hidden',
        mobileHeaderVisible
          ? 'top-[calc(3.5rem+var(--cabinet-tg-safe-top))]'
          : 'top-[var(--cabinet-tg-safe-top)] md:top-[calc(3.5rem+var(--cabinet-tg-safe-top))]',
      )}
    >
      {children}
    </div>
  )
}

function SummaryRow({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex items-baseline gap-2">
      <dt className="min-w-[92px] shrink-0 text-xs text-muted-foreground">{label}</dt>
      <dd className="min-w-0 truncate font-semibold">{children}</dd>
    </div>
  )
}

function ConfirmRow({
  icon: Icon,
  label,
  children,
}: {
  icon: LucideIcon
  label: string
  children: React.ReactNode
}) {
  return (
    <div className="grid grid-cols-[minmax(0,7rem)_minmax(0,1fr)] gap-3 px-3 py-2 text-sm">
      <dt className="flex items-center gap-2 text-muted-foreground">
        <Icon className="size-3.5 shrink-0" />
        <span className="min-w-0 truncate">{label}</span>
      </dt>
      <dd className="min-w-0 font-semibold">{children}</dd>
    </div>
  )
}
