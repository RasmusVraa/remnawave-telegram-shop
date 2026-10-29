import { useEffect, useState, type FormEvent, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Megaphone, PenLine, Users, X } from 'lucide-react'

import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useSheetDragDismiss } from '@/hooks/useSheetDragDismiss'
import { api, ApiError, type PartnerApplication } from '@/lib/api'
import { cn } from '@/lib/utils'

import { PARTNER_INPUT } from './layout'
import { PARTNER_STATE_KEY } from './partnerKeys'

/** Над шапкой и нижней навигацией кабинета, как у остальных модалок раздела. */
const MODAL_Z = 2600

/**
 * Заявка на партнёрство — в модальном окне.
 *
 * Раньше форма стояла третьим блоком страницы и конкурировала с оффером: тот,
 * кто ещё решает, видел её раньше, чем успевал понять условия. Теперь страница
 * целиком про предложение, а форма открывается по кнопке — то есть после того,
 * как человек уже согласился.
 */
export function PartnerApplyModal({
  open,
  onClose,
  application,
}: {
  open: boolean
  onClose: () => void
  /** Прошлая заявка: после отказа поля предзаполняются, чтобы не писать заново. */
  application?: PartnerApplication
}) {
  const { t } = useTranslation()
  const qc = useQueryClient()

  const [about, setAbout] = useState(application?.about ?? '')
  const [channels, setChannels] = useState(application?.channels ?? '')
  const [expected, setExpected] = useState(application?.expected ?? '')
  const [error, setError] = useState<string | null>(null)
  const [entered, setEntered] = useState(false)

  const apply = useMutation({
    mutationFn: () =>
      api.partnerApply({ about: about.trim(), channels: channels.trim(), expected: expected.trim() }),
    onSuccess: async () => {
      setError(null)
      // Закрываем сразу: страница под модалкой перерисуется в «заявка на
      // рассмотрении», и держать поверх неё форму было бы враньём.
      onClose()
      await qc.invalidateQueries({ queryKey: PARTNER_STATE_KEY })
    },
    onError: (e) => {
      if (e instanceof ApiError) {
        const raw = e.body || ''
        if (raw.includes('already_partner')) return setError(t('partnerPage.errors.alreadyPartner'))
        if (raw.includes('already_pending')) return setError(t('partnerPage.errors.alreadyPending'))
        if (raw.includes('about_required')) return setError(t('partnerPage.errors.aboutRequired'))
        if (raw.includes('too_long')) return setError(t('partnerPage.errors.tooLong'))
      }
      setError(t('partnerPage.errors.generic'))
    },
  })

  const drag = useSheetDragDismiss<HTMLDivElement, HTMLButtonElement>({
    open,
    onDismiss: onClose,
    desktopQuery: '(min-width: 768px)',
    disabled: apply.isPending,
  })

  // Въезд снизу отыгрывается кадром позже: без этого браузер применит конечное
  // состояние сразу и перехода видно не будет.
  useEffect(() => {
    if (!open) {
      setEntered(false)
      return
    }
    const id = window.requestAnimationFrame(() => {
      window.requestAnimationFrame(() => setEntered(true))
    })
    return () => window.cancelAnimationFrame(id)
  }, [open])

  useEffect(() => {
    if (!open || typeof document === 'undefined') return
    const prev = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    return () => {
      document.body.style.overflow = prev
    }
  }, [open])

  // Escape закрывает, пока заявка не отправляется: обрывать запрос на середине
  // нечем, и закрытие оставило бы пользователя без ответа об исходе.
  useEffect(() => {
    if (!open) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && !apply.isPending) onClose()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [open, onClose, apply.isPending])

  if (!open || typeof document === 'undefined') return null

  function onSubmit(e: FormEvent) {
    e.preventDefault()
    if (!about.trim()) {
      setError(t('partnerPage.errors.aboutRequired'))
      return
    }
    setError(null)
    apply.mutate()
  }

  return createPortal(
    <div
      className="fixed inset-0 flex flex-col justify-end md:items-center md:justify-center md:p-6"
      style={{ zIndex: MODAL_Z }}
      role="dialog"
      aria-modal="true"
      aria-labelledby="partner-apply-title"
    >
      <button
        ref={drag.backdropRef}
        type="button"
        className="absolute inset-0 bg-neutral-950/65 backdrop-blur-sm dark:bg-black/70"
        aria-label={t('common.close')}
        onClick={() => !apply.isPending && onClose()}
      />

      <div
        ref={drag.sheetRef}
        className={cn(
          'relative z-[1] w-full border border-border bg-card shadow-2xl transition-[transform,opacity] duration-300 ease-out motion-reduce:transition-none',
          'max-h-[min(90dvh,680px)] overflow-y-auto rounded-t-3xl p-5 pb-[max(1.25rem,calc(0.75rem+var(--cabinet-sheet-safe-bottom)))]',
          'md:max-h-[90vh] md:w-full md:max-w-lg md:rounded-2xl md:p-7',
          entered
            ? 'translate-y-0 opacity-100 md:scale-100'
            : 'translate-y-full opacity-95 md:translate-y-2 md:scale-[0.97] md:opacity-0',
        )}
      >
        {/* Ручка шторки: за неё и за заголовок окно тянут вниз, чтобы закрыть. */}
        <div className="-mx-5 -mt-5 mb-1 flex cursor-grab touch-none justify-center pb-2 pt-3 md:hidden" {...drag.handlers}>
          <span className="h-1 w-10 rounded-full bg-muted-foreground/30" aria-hidden />
        </div>
        <div className="flex touch-none items-start justify-between gap-3 md:touch-auto" {...drag.handlers}>
          <h2 id="partner-apply-title" className="text-lg font-semibold tracking-tight">
            {application ? t('partnerPage.form.titleAgain') : t('partnerPage.form.title')}
          </h2>
          <button
            type="button"
            onClick={() => !apply.isPending && onClose()}
            className="-mr-1 -mt-1 rounded-lg p-1.5 text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground"
            aria-label={t('common.close')}
          >
            <X size={18} />
          </button>
        </div>

        <form className="mt-4 space-y-3" onSubmit={onSubmit}>
          <div className="space-y-1.5">
            <FieldLabel htmlFor="partner-about" icon={<PenLine size={14} />}>
              {t('partnerPage.form.about')}
            </FieldLabel>
            {/*
              Без autoFocus намеренно. На iOS фокус поля с font-size меньше 16px
              заставляет WebKit приблизить страницу, и с autoFocus это
              происходило само в момент открытия модалки: клавиатура вылезала
              сразу, а страница оставалась увеличенной и уезжала за правый край
              экрана даже после закрытия формы. Масштаб на время ввода теперь
              фиксирует lib/ios-input-zoom, но открывать клавиатуру за
              пользователя всё равно незачем — сначала он читает форму.
            */}
            <textarea
              id="partner-about"
              value={about}
              onChange={(e) => setAbout(e.target.value)}
              rows={4}
              maxLength={2000}
              className="w-full rounded-lg border border-border bg-secondary px-3 py-2 text-sm focus:border-primary focus:outline-none"
              placeholder={t('partnerPage.form.aboutPlaceholder')}
            />
          </div>

          <div className="space-y-1.5">
            <FieldLabel htmlFor="partner-channels" icon={<Megaphone size={14} />}>
              {t('partnerPage.form.channels')}
            </FieldLabel>
            <Input
              id="partner-channels"
              value={channels}
              onChange={(e) => setChannels(e.target.value)}
              maxLength={1000}
              placeholder={t('partnerPage.form.channelsPlaceholder')}
              className={PARTNER_INPUT}
            />
          </div>

          <div className="space-y-1.5">
            <FieldLabel htmlFor="partner-expected" icon={<Users size={14} />}>
              {t('partnerPage.form.expected')}
            </FieldLabel>
            <Input
              id="partner-expected"
              value={expected}
              onChange={(e) => setExpected(e.target.value)}
              maxLength={200}
              placeholder={t('partnerPage.form.expectedPlaceholder')}
              className={PARTNER_INPUT}
            />
          </div>

          {error ? (
            <Alert variant="destructive">
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          ) : null}

          <p className="text-xs text-muted-foreground">{t('partnerPage.form.note')}</p>

          <Button type="submit" className="w-full" disabled={apply.isPending}>
            {apply.isPending ? t('partnerPage.form.submitting') : t('partnerPage.form.submit')}
          </Button>
        </form>
      </div>
    </div>,
    document.body,
  )
}

/**
 * Подпись поля с иконкой.
 *
 * Иконки намеренно приглушённые и мельче заголовка: заголовок задаёт раздел, а
 * эти лишь помогают взглядом находить нужное поле.
 */
function FieldLabel({ htmlFor, icon, children }: { htmlFor: string; icon: ReactNode; children: ReactNode }) {
  return (
    <Label htmlFor={htmlFor} className="flex items-center gap-1.5">
      <span className="text-muted-foreground">{icon}</span>
      {children}
    </Label>
  )
}
