import { useCallback, useEffect, useRef, useState, type MouseEvent, type ReactNode } from 'react'
import { useNavigate } from 'react-router-dom'
import { ArrowRight, Check, Download, ExternalLink, MonitorSmartphone, Plus } from 'lucide-react'

import { AppLayout } from '@/components/AppLayout'
import { Logo } from '@/components/Logo'
import { PageReveal, RevealItem } from '@/components/PageReveal'
import { PageTitleWithBack } from '@/components/PageTitleWithBack'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { SupportChatModal } from '@/features/support/SupportChatModal'
import { ApiError } from '@/lib/api'
import { cn } from '@/lib/utils'
import { useToast } from '@/components/ui/toast'

import { DevicePicker } from './guide/DevicePicker'
import { GuideHelp } from './guide/GuideHelp'
import { GuideIllustration, type SceneFreeze, type SceneKind } from './guide/GuideIllustration'
import { DeviceChip, GuideMarkdown, GuideStepLine } from './guide/GuideParts'
import { isPhonePlatform } from './guide/glyphs'
import { guideText, type GuideText } from './guide/guideText'
import { pickText } from './guide/types'
import { emptyProgress, inviteScope, useGuideProgress, type GuideProgress } from './guide/useGuideProgress'
import { useAppReturn, type AppTrip } from './guide/useAppReturn'
import { useConnectionGuide } from './useConnectionGuide'
import './guide/connections-guide.css'

const SCENES: SceneKind[] = ['install', 'add', 'connect']

/** Короткое состояние внутри шага; сбрасывается при любой смене шага. */
type Flow = { downloaded: boolean; noOpen: boolean }
const FRESH_FLOW: Flow = { downloaded: false, noOpen: false }

/** Сколько ждём, что после «Добавить подписку» страница уйдёт в приложение. */
const APP_OPEN_WAIT_MS = 3500
/** Сколько висит тост «Приложение установлено / Подписка добавлена». */
const NOTICE_MS = 4500

/** ПК — три карточки в ряд, телефон — мастер по одному шагу. Граница как у md: в Tailwind. */
function useIsDesktop(): boolean {
  const query = '(min-width: 768px)'
  const [desktop, setDesktop] = useState(() => typeof window !== 'undefined' && window.matchMedia(query).matches)
  useEffect(() => {
    const mq = window.matchMedia(query)
    const onChange = () => setDesktop(mq.matches)
    onChange()
    mq.addEventListener('change', onChange)
    return () => mq.removeEventListener('change', onChange)
  }, [])
  return desktop
}

async function copyToClipboard(value: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(value)
    return true
  } catch {
    // Старые WebView без Clipboard API — через временное поле.
    try {
      const el = document.createElement('textarea')
      el.value = value
      el.setAttribute('readonly', '')
      el.style.position = 'fixed'
      el.style.opacity = '0'
      document.body.appendChild(el)
      el.select()
      const ok = document.execCommand('copy')
      el.remove()
      return ok
    } catch {
      return false
    }
  }
}

export default function ConnectionsPage() {
  const g = useConnectionGuide()
  const text = guideText[g.lang]
  const isDesktop = useIsDesktop()
  const navigate = useNavigate()
  const app = g.selectedApp
  // Шаг «Включение» есть не у всех приложений: нет его описания в app-config —
  // гид из двух шагов, и «Да, подписка появилась» сразу ведёт к финалу.
  const hasConnectStep = Boolean(pickText(app?.connectAndUseStep?.description, g.lang).trim())
  const total = hasConnectStep ? 3 : 2
  const scenes = SCENES.slice(0, total)

  const ready0 = () => !g.loading && !g.inviteError && !g.configError && Boolean(g.selectedApp)

  const progressKey =
    app && g.selectedPlatform ? `${g.inviteMode ? inviteScope(g.inviteToken) : 'me'}:${g.selectedPlatform}:${app.id}` : ''
  const { progress, update } = useGuideProgress(progressKey)
  const { done, finished } = progress
  // Сохранённый шаг мог остаться от варианта с тремя шагами.
  const current = Math.min(progress.current, total - 1)

  const [flow, setFlow] = useState<Flow>(FRESH_FLOW)
  const [helpOpen, setHelpOpen] = useState(false)
  const [pickerOpen, setPickerOpen] = useState(false)
  const [chatOpen, setChatOpen] = useState(false)
  const [copied, setCopied] = useState(false)
  const helpRef = useRef<HTMLDivElement>(null)
  // Каждое раскрытие помощи прокручивает к ней — и по ссылке, и по самому заголовку блока.
  const [helpScroll, setHelpScroll] = useState(0)
  const noOpenTimer = useRef(0)

  // Другая пара «платформа + приложение» — шаги начинаются со своего состояния.
  useEffect(() => {
    setFlow(FRESH_FLOW)
  }, [progressKey])

  useEffect(() => () => window.clearTimeout(noOpenTimer.current), [])

  const changeProgress = useCallback(
    (fn: (p: GuideProgress) => GuideProgress) => {
      setFlow(FRESH_FLOW)
      window.clearTimeout(noOpenTimer.current)
      update(fn)
    },
    [update],
  )

  const goTo = (step: number) => changeProgress((p) => ({ ...p, current: step, finished: false }))
  const complete = (step: number) =>
    changeProgress((p) => {
      const nextDone = [...p.done] as GuideProgress['done']
      nextDone[step] = true
      return step >= total - 1 ? { ...p, done: nextDone, finished: true } : { ...p, done: nextDone, current: step + 1 }
    })
  const undo = (step: number) =>
    changeProgress((p) => {
      const nextDone = [...p.done] as GuideProgress['done']
      nextDone[step] = false
      return { current: step, done: nextDone, finished: false }
    })

  /*
   * Телефон: вместо «Установил — дальше» и «Подписка появилась?» гид сам
   * замечает, что человек сходил в магазин или в приложение и вернулся, и
   * переходит дальше. Ошибся — плашка «Ещё нет» / «Не появилась?» вернёт назад.
   */
  const toast = useToast()
  const trip = useAppReturn(progressKey, ready0() && !isDesktop, (kind) => {
    complete(kind === 'install' ? 0 : 1)
    // Временный тост, а не плашка в шаге: сообщили и ушли, кнопка — на случай ошибки.
    toast.success(kind === 'install' ? text.noticeInstalled : text.noticeAdded, {
      action: { label: kind === 'install' ? text.notInstalled : text.notAddedShort, onClick: () => revertTrip(kind) },
      durationMs: NOTICE_MS,
    })
  })

  function addSubscription(compact: boolean) {
    if (!g.openAddSubscription()) return
    // ПК: все шаги перед глазами, а помощь — прямо под ними, поэтому без
    // вопроса «Подписка появилась?» сразу переходим к следующему шагу.
    if (compact) {
      complete(1)
      return
    }
    trip.start('add')
    setFlow((f) => ({ ...f, noOpen: false }))
    // Страница так и не ушла в приложение — значит, оно не открылось (часто — ещё
    // не установлено). Показываем помощь прямо под кнопкой, без лишних вопросов.
    window.clearTimeout(noOpenTimer.current)
    noOpenTimer.current = window.setTimeout(() => {
      if (!trip.hasLeft()) setFlow((f) => ({ ...f, noOpen: true }))
    }, APP_OPEN_WAIT_MS)
  }

  function openHelp() {
    setHelpOpen(true)
    setHelpScroll((n) => n + 1)
  }

  // После отрисовки раскрытого блока: весь он должен оказаться на экране, вместе с кнопками.
  useEffect(() => {
    if (!helpScroll) return
    const id = requestAnimationFrame(() => helpRef.current?.scrollIntoView({ behavior: 'smooth', block: 'nearest' }))
    return () => cancelAnimationFrame(id)
  }, [helpScroll])

  /** Гид поторопился: шаг на самом деле не сделан — возвращаемся к нему. */
  function revertTrip(kind: AppTrip) {
    if (kind === 'install') {
      undo(0)
    } else {
      undo(1)
      openHelp()
    }
  }

  function connectAnother() {
    changeProgress(() => emptyProgress)
    setPickerOpen(true)
  }

  async function copyLink() {
    if (!(await copyToClipboard(g.subscriptionLink))) return
    setCopied(true)
    window.setTimeout(() => setCopied(false), 2000)
  }

  function openSupport() {
    if (g.supportChat) {
      setChatOpen(true)
      return
    }
    window.open(g.supportUrl, '_blank', 'noopener,noreferrer')
  }

  const Shell = g.inviteMode ? InviteShell : AppLayout
  const ready = ready0()
  const shape = isPhonePlatform(g.selectedPlatform) ? 'phone' : 'laptop'

  /* ── Куски шага ─────────────────────────────────────────────────── */

  const stepTitle = (i: number) => (i === 0 ? text.installTitle(app?.name || '') : i === 1 ? text.addTitle : text.usageTitle)
  const stepDescription = (i: number) =>
    app ? pickText([app.installationStep, app.addSubscriptionStep, app.connectAndUseStep][i]?.description, g.lang) : ''

  function scene(kind: SceneKind, freeze: SceneFreeze, className: string) {
    if (!app) return null
    return (
      <GuideIllustration
        kind={kind}
        shape={shape}
        app={app}
        brandName={g.brandName}
        brandLogoUrl={g.brandLogoUrl}
        labels={{ add: text.illAdd, added: text.illAdded, off: text.illOff, on: text.illOn }}
        freeze={freeze}
        className={className}
      />
    )
  }

  /** Действия шага. compact — карточка на ПК; isCurrent — только у текущего шага главная кнопка яркая. */
  function stepActions(i: number, compact: boolean, isCurrent: boolean): ReactNode {
    if (!app) return null
    const size = compact ? 'h-10 w-full' : 'h-12 w-full text-base [&_svg]:size-[18px]'
    const primary = isCurrent ? 'default' : 'outline'

    if (i === 0) {
      const buttons = (app.installationStep.buttons || []).filter((b) => b.buttonLink?.trim())
      const installButtons = buttons.map((btn, idx) => (
        <Button
          key={`${btn.buttonLink}-${idx}`}
          asChild
          variant={idx === 0 && !flow.downloaded ? primary : 'outline'}
          className={size}
        >
          <a
            href={btn.buttonLink}
            target="_blank"
            rel="noopener noreferrer"
            onClick={() => {
              if (compact) {
                complete(0)
                return
              }
              trip.start('install')
              setFlow((f) => ({ ...f, downloaded: true }))
            }}
          >
            {idx === 0 ? <Download /> : <ExternalLink />}
            <span className="truncate">{pickText(btn.buttonText, g.lang)}</span>
          </a>
        </Button>
      ))
      if (flow.downloaded) {
        return (
          <>
            {installButtons}
            <Button variant={primary} className={size} onClick={() => complete(0)}>
              <Check />
              {text.installedNext}
            </Button>
          </>
        )
      }
      const skip = (
        <button
          type="button"
          onClick={() => complete(0)}
          className="mx-auto inline-flex items-center gap-1.5 px-1 py-1 text-[12.5px] font-medium text-muted-foreground transition-colors hover:text-primary"
        >
          {text.alreadyInstalled}
          <ArrowRight size={13} />
        </button>
      )
      // На ПК ссылка над кнопками: кнопки всех трёх карточек лежат по одному низу.
      return compact ? (
        <>
          {skip}
          {installButtons}
        </>
      ) : (
        <>
          {installButtons}
          {skip}
        </>
      )
    }

    if (i === 1) {
      const hint = g.addHint ? (
        <p className="text-xs text-amber-700 dark:text-amber-300/90">{text[g.addHint]}</p>
      ) : null
      return (
        <>
          <Button variant={primary} className={size} onClick={() => addSubscription(compact)} disabled={g.addDisabled}>
            <Plus />
            {flow.noOpen ? text.tryAgain : text.addSubscription}
          </Button>
          {flow.noOpen ? (
            <div className="rounded-xl border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-[13px] text-muted-foreground">
              <b className="font-semibold text-foreground">{text.noOpenLead(app.name)}</b> {text.noOpenTip}{' '}
              <button type="button" onClick={openHelp} className="font-semibold text-primary hover:underline">
                {text.helpTitle}
              </button>
            </div>
          ) : null}
          {hint}
        </>
      )
    }

    return (
      <Button variant={primary} className={size} onClick={() => complete(2)}>
        <Check />
        {text.connected}
      </Button>
    )
  }

  /* ── Разметка ───────────────────────────────────────────────────── */

  // На /connect «назад» некуда: у гостя нет кабинета, куда возвращаться.
  const title = (
    <PageTitleWithBack
      title={text.pageTitle}
      showBack={!g.inviteMode}
      titleClassName="text-2xl font-semibold tracking-tight text-foreground dark:text-slate-100"
    />
  )

  const stepLine = (withCounter: boolean) => (
    <GuideStepLine
      text={text}
      current={current}
      total={total}
      done={done}
      finished={finished}
      onPick={goTo}
      withCounter={withCounter}
    />
  )

  let body: ReactNode
  if (g.loading) {
    body = <GuideSkeleton desktop={isDesktop} />
  } else if (g.inviteError) {
    body = <p className="text-sm text-destructive">{inviteErrorText(g.inviteError, text)}</p>
  } else if (g.configError || !app) {
    body = <p className="text-sm text-destructive">{text.configError}</p>
  } else if (finished) {
    body = (
      <Finale
        desktop={isDesktop}
        text={text}
        description={text.finaleText(g.brandName, app.name)}
        illustration={scene('done', 'none', isDesktop ? 'h-[240px]' : 'h-[220px]')}
        onAnother={connectAnother}
        onShowSteps={() => goTo(0)}
        onHome={g.inviteMode ? null : () => navigate('/')}
      />
    )
  } else if (isDesktop) {
    body = (
      <div className={cn('grid gap-x-3', total === 2 ? 'grid-cols-2' : 'grid-cols-3')}>
        {scenes.map((kind, i) => {
          const isCurrent = i === current
          const state = done[i] ? 'done' : isCurrent ? 'current' : 'rest'
          // Анимация — у выбранного шага, даже уже пройденного. Остальные стоят:
          // пройденные — на итоговом кадре, будущие «Подписка» и «Включение» — на
          // начальном, чтобы не выглядеть уже сделанными.
          const freeze: SceneFreeze = isCurrent ? 'none' : state === 'done' || i === 0 ? 'end' : 'start'
          return (
            <div
              key={kind}
              onClick={(e: MouseEvent<HTMLDivElement>) => {
                if ((e.target as HTMLElement).closest('button, a')) return
                if (i !== current || finished) goTo(i)
              }}
              className={cn(
                // Подсетка: картинка, заголовок, описание и кнопки на одних линиях во всех карточках.
                'row-span-4 grid grid-rows-subgrid gap-y-2.5 rounded-2xl border p-3 transition-[border-color,box-shadow,opacity] duration-300',
                'border-border bg-foreground/[0.03] dark:border-white/10',
                state === 'current' &&
                  'border-primary/60 shadow-[0_0_0_3px_hsl(var(--primary)/0.15),0_14px_30px_-18px_hsl(var(--primary)/0.9)] dark:border-primary/60',
                // Вернулись к пройденному шагу — подсветка зелёная, как сегмент шаг-линии.
                state === 'done' &&
                  isCurrent &&
                  'border-emerald-500/60 shadow-[0_0_0_3px_rgb(16_185_129_/_0.15),0_14px_30px_-18px_rgb(16_185_129_/_0.9)] dark:border-emerald-500/60',
                state === 'rest' && 'cursor-pointer opacity-80 hover:opacity-100',
                state === 'done' && !isCurrent && 'cursor-pointer',
              )}
            >
              {scene(kind, freeze, cn('cg-ill--compact h-[170px]', state === 'done' && !isCurrent && 'opacity-65 saturate-50'))}
              <div className="flex items-center gap-2.5">
                <StepNumber index={i} state={state} />
                <b className="text-[15.5px] font-semibold leading-tight text-foreground">{stepTitle(i)}</b>
              </div>
              <GuideMarkdown text={stepDescription(i)} clamp labels={text} className="min-w-0 [&_div]:text-[13.5px]" />
              <div className="flex flex-col gap-1.5 self-end pt-1">
                {state === 'done' ? (
                  <>
                    {/* Вопроса «появилась?» на ПК нет — вместо него тихая ссылка к помощи.
                        Над кнопкой, чтобы кнопки карточек остались на одной линии. */}
                    {i === 1 ? (
                      <button
                        type="button"
                        onClick={openHelp}
                        className="mx-auto px-1 py-1 text-[12.5px] font-medium text-muted-foreground transition-colors hover:text-primary"
                      >
                        {text.notAdded}
                      </button>
                    ) : null}
                    <Button variant="outline" className={cn('h-10 w-full', OK_BUTTON)} onClick={() => undo(i)}>
                      <Check />
                      {text.markedDone}
                    </Button>
                  </>
                ) : (
                  stepActions(i, true, state === 'current')
                )}
              </div>
            </div>
          )
        })}
      </div>
    )
  } else {
    body = (
      <div key={current} className="cg-step-enter">
        {scene(SCENES[current], 'none', 'h-[210px]')}
        <div className="mt-4 flex flex-col gap-1.5">
          <div className="text-xs font-bold uppercase tracking-wider text-primary">
            {text.stepLabel(current + 1)}
            {done[current] ? ` · ${text.stepDone} ✓` : ''}
          </div>
          <h2 className="text-[23px] font-bold leading-tight tracking-tight text-foreground">{stepTitle(current)}</h2>
          <GuideMarkdown text={stepDescription(current)} labels={text} className="[&_div]:text-[14.5px]" />
        </div>
        <div className="mt-3.5 flex flex-col gap-2">{stepActions(current, false, true)}</div>
      </div>
    )
  }

  const help =
    ready && app ? (
      <GuideHelp
        ref={helpRef}
        text={text}
        lang={g.lang}
        open={helpOpen}
        onToggle={() => (helpOpen ? setHelpOpen(false) : openHelp())}
        extra={app.additionalAfterAddSubscriptionStep}
        onCopy={g.subscriptionLink ? copyLink : null}
        copied={copied}
        onSupport={g.supportChat || g.supportUrl ? openSupport : null}
        className={isDesktop ? 'mt-4' : 'bg-card dark:bg-[linear-gradient(180deg,#0e1b34d6,#0a1428d1)]'}
      />
    ) : null

  return (
    <Shell>
      <PageReveal className="mx-auto w-full max-w-5xl space-y-4">
        <RevealItem>
          <Card className="overflow-visible border-border/80 bg-card dark:bg-[linear-gradient(180deg,#0e1b34d6,#0a1428d1)]">
            <CardContent className="p-4 sm:p-6">
              {isDesktop ? (
                <>
                  <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
                    <div className="min-w-0">{title}</div>
                    {ready && app ? (
                      <DeviceChip text={text} platform={g.selectedPlatform} app={app} onOpen={() => setPickerOpen(true)} />
                    ) : null}
                  </div>
                  {ready ? <div className="mb-3.5">{stepLine(true)}</div> : null}
                </>
              ) : (
                <>
                  {/* Номер шага на телефоне не пишем — его видно по шаг-линии, а место
                      справа от заголовка отдано кнопке выбора устройства. */}
                  <div className="mb-3.5 flex items-center justify-between gap-3">
                    <div className="min-w-0">{title}</div>
                    {ready && app ? (
                      <DeviceChip text={text} platform={g.selectedPlatform} app={app} onOpen={() => setPickerOpen(true)} compact />
                    ) : null}
                  </div>
                  {ready ? <div className="mb-3.5">{stepLine(false)}</div> : null}
                </>
              )}
              {body}
              {isDesktop ? help : null}
            </CardContent>
          </Card>
        </RevealItem>
        {!isDesktop && !finished && help ? <RevealItem>{help}</RevealItem> : null}
      </PageReveal>

      <DevicePicker
        open={pickerOpen}
        onClose={() => setPickerOpen(false)}
        text={text}
        platforms={g.availablePlatforms}
        platform={g.selectedPlatform}
        onPlatform={g.pickPlatform}
        apps={g.appsFor(g.selectedPlatform)}
        appId={app?.id || ''}
        onApp={g.setSelectedAppId}
      />
      {g.supportChat ? <SupportChatModal open={chatOpen} enabled onClose={() => setChatOpen(false)} /> : null}
    </Shell>
  )
}

const OK_BUTTON =
  'border-emerald-500/35 bg-emerald-500/10 text-emerald-700 hover:border-emerald-500/50 hover:bg-emerald-500/15 dark:bg-emerald-500/10 dark:text-emerald-300 dark:hover:bg-emerald-500/15'

function StepNumber({ index, state }: { index: number; state: 'done' | 'current' | 'rest' }) {
  return (
    <span
      className={cn(
        'grid size-7 shrink-0 place-items-center rounded-full text-[13.5px] font-extrabold',
        state === 'done' && 'bg-emerald-500 text-white',
        state === 'current' && 'bg-primary text-primary-foreground',
        state === 'rest' && 'bg-foreground/[0.08] text-foreground',
      )}
    >
      {state === 'done' ? <Check size={15} strokeWidth={3} /> : index + 1}
    </span>
  )
}

function Finale({
  desktop,
  text,
  description,
  illustration,
  onAnother,
  onShowSteps,
  onHome,
}: {
  desktop: boolean
  text: GuideText
  description: string
  illustration: ReactNode
  onAnother: () => void
  onShowSteps: () => void
  onHome: (() => void) | null
}) {
  if (desktop) {
    return (
      <div className="cg-rise grid grid-cols-[300px_1fr] items-center gap-7 py-1">
        {illustration}
        <div>
          <h2 className="mb-2 text-[28px] font-bold leading-tight tracking-tight text-foreground">{text.finaleTitle}</h2>
          <p className="mb-4 max-w-[42ch] text-[15px] text-muted-foreground">{description}</p>
          <div className="flex flex-wrap gap-2">
            <Button size="lg" className="px-5" onClick={onAnother}>
              <MonitorSmartphone />
              {text.anotherDevice}
            </Button>
            <Button size="lg" variant="outline" className="px-5" onClick={onShowSteps}>
              {text.showSteps}
            </Button>
          </div>
        </div>
      </div>
    )
  }
  return (
    <div className="cg-rise text-center">
      {illustration}
      <h2 className="mb-1.5 mt-4 text-[25px] font-bold tracking-tight text-foreground">{text.finaleTitleShort}</h2>
      <p className="text-[14.5px] text-muted-foreground">{description}</p>
      <div className="mt-4 flex flex-col gap-2">
        <Button className="h-12 w-full text-base" onClick={onAnother}>
          <MonitorSmartphone />
          {text.anotherDevice}
        </Button>
        {onHome ? (
          <Button variant="outline" className="h-11 w-full" onClick={onHome}>
            {text.toHome}
          </Button>
        ) : null}
      </div>
    </div>
  )
}

/** Заглушка в форме будущего экрана: три карточки на ПК, один шаг на телефоне. */
function GuideSkeleton({ desktop }: { desktop: boolean }) {
  if (desktop) {
    return (
      <div className="space-y-3.5" aria-hidden>
        <Skeleton className="h-6 w-full rounded-md" />
        <div className="grid grid-cols-3 gap-3">
          {[0, 1, 2].map((i) => (
            <Skeleton key={i} className="h-[330px] rounded-2xl" />
          ))}
        </div>
      </div>
    )
  }
  return (
    <div className="space-y-3" aria-hidden>
      <Skeleton className="h-6 w-full rounded-md" />
      <Skeleton className="h-11 w-full rounded-2xl" />
      <Skeleton className="h-[210px] w-full rounded-2xl" />
      <Skeleton className="h-6 w-2/3" />
      <Skeleton className="h-4 w-full" />
      <Skeleton className="h-12 w-full rounded-lg" />
    </div>
  )
}

/**
 * Оболочка публичной страницы приглашения.
 *
 * Без навигации кабинета: у гостя нет сессии, и любая ссылка отсюда привела бы
 * его на экран логина — ровно туда, куда он попасть не может. Сверху только
 * логотип и название, чтобы было видно, чей это сервис.
 */
function InviteShell({ children }: { children: ReactNode }) {
  return (
    <div className="min-h-screen bg-background px-3 py-6 sm:px-4 sm:py-10">
      <div className="mx-auto mb-5 flex w-full max-w-5xl justify-center">
        <Logo size="sm" />
      </div>
      <div className="mx-auto w-full max-w-5xl">{children}</div>
    </div>
  )
}

/** Человеческий текст на коды ошибок /public/connect. */
function inviteErrorText(error: unknown, text: GuideText): string {
  const code = error instanceof ApiError ? error.body.trim() : ''
  switch (code) {
    case 'invite_expired':
      return text.inviteExpired
    case 'invite_invalid':
      return text.inviteInvalid
    case 'no_subscription':
      return text.inviteNoSubscription
    case 'subscription_expired':
      return text.inviteSubscriptionExpired
    default:
      return text.inviteUnavailable
  }
}
