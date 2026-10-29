import { useEffect, useState } from 'react'
import { createPortal } from 'react-dom'
import { animate, motion, useAnimation } from 'framer-motion'
import { useTranslation } from 'react-i18next'
import { Sparkles } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { useSheetDragDismiss } from '@/hooks/useSheetDragDismiss'
import { cn } from '@/lib/utils'
import { FORTUNE_SECTOR_ICONS } from '@/features/fortune/fortuneSectorIcons'
import { sectorIconClass } from '@/features/fortune/fortunePrizeVisuals'

const MODAL_Z = 2600

type FortunePrizeModalProps = {
  open: boolean
  rewardType: string
  rewardValue: number
  wasFree: boolean
  onClaim: () => void
}

export function FortunePrizeModal({ open, rewardType, rewardValue, wasFree, onClaim }: FortunePrizeModalProps) {
  const { t } = useTranslation()
  const [entered, setEntered] = useState(false)
  const [displayValue, setDisplayValue] = useState(0)
  const modalControls = useAnimation()
  // Смахнуть шторку — то же, что тап по фону: приз забирается.
  const drag = useSheetDragDismiss<HTMLDivElement, HTMLButtonElement>({
    open,
    onDismiss: onClaim,
    desktopQuery: '(min-width: 768px)',
  })

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
    if (!open) return
    setDisplayValue(0)
    const controls = animate(0, rewardValue, {
      duration: 0.9,
      ease: [0.22, 1, 0.36, 1],
      onUpdate: (v) => setDisplayValue(Math.round(v)),
    })
    void modalControls.start({
      scale: [0.92, 1.03, 1],
      y: [10, -4, 0],
      transition: { duration: 0.55, ease: [0.22, 1, 0.36, 1] },
    })
    return () => controls.stop()
  }, [open, rewardValue, modalControls])

  useEffect(() => {
    if (!open || typeof document === 'undefined') return
    const prev = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    return () => {
      document.body.style.overflow = prev
    }
  }, [open])

  if (!open) return null
  if (typeof document === 'undefined') return null

  const description = t(`fortune.result.${rewardType}`, { value: displayValue })
  const amountToken = `+${displayValue}`
  const descriptionParts = description.split(amountToken)
  const PrizeIcon = FORTUNE_SECTOR_ICONS[rewardType] ?? Sparkles
  const prizeIconCls = sectorIconClass(rewardType)

  return createPortal(
    <div
      className="fixed inset-0 flex flex-col justify-end md:items-center md:justify-center md:p-6"
      style={{ zIndex: MODAL_Z }}
      role="dialog"
      aria-modal="true"
      aria-labelledby="fortune-prize-title"
    >
      <button
        ref={drag.backdropRef}
        type="button"
        className="absolute inset-0 bg-neutral-950/65 backdrop-blur-sm transition-opacity dark:bg-black/70"
        aria-label={t('common.close')}
        onClick={onClaim}
      />
      <motion.div
        ref={drag.sheetRef}
        animate={modalControls}
        initial={false}
        className={cn(
          'relative z-[1] w-full border border-border bg-card shadow-2xl transition-[transform,opacity] duration-300 ease-out motion-reduce:transition-none',
          'max-h-[min(88dvh,560px)] overflow-y-auto rounded-t-3xl p-6 pb-[max(1.25rem,calc(0.75rem+var(--cabinet-sheet-safe-bottom)))] md:max-h-[90vh] md:w-full md:max-w-md md:rounded-2xl md:p-8 md:pb-8',
          entered
            ? 'translate-y-0 opacity-100 md:translate-y-0 md:scale-100'
            : 'translate-y-full opacity-95 md:translate-y-2 md:scale-[0.97] md:opacity-0',
        )}
      >
        {/* Ручка шторки: за неё и за заголовок окно тянут вниз, чтобы закрыть. */}
        <div className="-mx-6 -mt-6 mb-1 flex cursor-grab touch-none justify-center pb-2 pt-3 md:hidden" {...drag.handlers}>
          <span className="h-1 w-10 rounded-full bg-muted-foreground/30" aria-hidden />
        </div>
        <h2
          id="fortune-prize-title"
          className="touch-none text-center text-xl font-semibold tracking-tight md:touch-auto"
          {...drag.handlers}
        >
          {t('fortune.prizeModalTitle')}
        </h2>
        <div className="mt-4 flex justify-center" aria-hidden>
          <div className="fortune-prize-shine relative overflow-hidden rounded-2xl border border-border/80 bg-muted/40 p-4 shadow-inner">
            <PrizeIcon className={cn('size-14 sm:size-16', prizeIconCls)} strokeWidth={2} />
            <span className="absolute inset-0 bg-[linear-gradient(120deg,transparent_26%,rgba(255,255,255,0.58)_48%,transparent_68%)] animate-[fortune-prize-shine_1.2s_ease-out]" />
          </div>
        </div>
        <p className="mt-4 text-center text-sm text-muted-foreground">
          {descriptionParts.length > 1 ? (
            descriptionParts.map((part, idx) => (
              <span key={idx}>
                {part}
                {idx < descriptionParts.length - 1 ? (
                  <span className="font-semibold text-emerald-600 dark:text-emerald-300">{amountToken}</span>
                ) : null}
              </span>
            ))
          ) : (
            description
          )}
        </p>
        {wasFree && (
          <p className="mt-2 text-center text-xs font-medium text-primary">{t('fortune.wasFree')}</p>
        )}
        <Button type="button" size="lg" className="mt-6 w-full max-w-none md:max-w-full" onClick={onClaim}>
          {t('fortune.claimButton')}
        </Button>
      </motion.div>
    </div>,
    document.body,
  )
}
