import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { CalendarDays, Dice5, Info, Lock, Pencil, Ticket } from 'lucide-react'

import { cn } from '@/lib/utils'
import { Spinner } from '@/components/ui/spinner'
import { ApiError } from '@/lib/api'
import type { AdminPromoCodeDTO, AdminPromoTypeFilter } from '@/lib/types/admin'
import { AdminModal } from '../components/AdminModal'
import { AdminConfirmModal } from '../components/AdminConfirmModal'
import { AdminCheckboxField } from '../components/AdminCheckbox'
import { AdminDatePicker } from '../components/AdminDatePicker'
import { AdminSelect } from '../components/AdminSelect'
import { useAdminPromoCreate, useAdminPromoUpdate, type CreatePromoInput } from '../hooks/useAdminPromos'
import { formatAdminApiError } from '../utils/formatAdminApiError'
import { hasPromoUseLimit } from '../utils/promoStatus'
import { generatePromoCode, isValidPromoCode } from './generatePromoCode'
import { PromoTypeTile } from './PromoBits'
import { PromoEffectSentence, PromoPreviewTail } from './PromoPreview'
import { discountTtlKind, formatPromoDateTime, PROMO_TYPES, promoTypeLabel } from './promoPresentation'

const NOTE_MAX = 200
const CODE_MAX = 64
const DAY_MS = 24 * 60 * 60 * 1000

export type PromoFormMode =
  | { kind: 'create' }
  | { kind: 'duplicate'; source: AdminPromoCodeDTO }
  | { kind: 'edit'; promo: AdminPromoCodeDTO; focus?: PromoFormFocus }

/** Поле, на котором открыть правку (тап по плитке в листе подробностей). */
export type PromoFormFocus = 'value' | 'validUntil' | 'firstPurchase'

interface FormState {
  type: AdminPromoTypeFilter
  code: string
  note: string
  days: string
  trialDays: string
  hwid: string
  percent: string
  ttl: string
  /** Старая скидка из бота с TTL 0: поле пустое и не считается изменённым, пока его не тронули. */
  ttlLegacy: boolean
  payments: string
  tariffId: string | null
  maxUses: string
  validUntil: Date | null
  firstPurchase: boolean
}

const numStr = (v: number | null | undefined) => (v == null ? '' : String(v))

function stateFromPromo(p: AdminPromoCodeDTO, opts: { duplicate: boolean }): FormState {
  const legacyTtl = p.discount_ttl_hours != null && p.discount_ttl_hours <= 0
  let validUntil: Date | null = p.valid_until ? new Date(p.valid_until) : null
  if (opts.duplicate) {
    // Копия живёт столько же, сколько жил исходный код, но считая от сейчас.
    if (validUntil && validUntil.getTime() > Date.now()) {
      const life = Math.max(1, Math.round((validUntil.getTime() - Date.parse(p.created_at)) / DAY_MS))
      validUntil = endOfDay(new Date(Date.now() + life * DAY_MS))
    } else validUntil = null
  }
  return {
    type: (PROMO_TYPES as string[]).includes(p.type) ? (p.type as AdminPromoTypeFilter) : 'subscription_days',
    code: opts.duplicate ? generatePromoCode() : p.code,
    note: p.note ?? '',
    days: numStr(p.subscription_days),
    trialDays: numStr(p.trial_days),
    hwid: numStr(p.extra_hwid_delta),
    percent: numStr(p.discount_percent),
    ttl: legacyTtl ? '' : numStr(p.discount_ttl_hours),
    ttlLegacy: legacyTtl && !opts.duplicate,
    payments: p.discount_max_subscription_payments_per_customer > 0 ? String(p.discount_max_subscription_payments_per_customer) : '',
    tariffId: p.type === 'subscription_days' && p.tariff_id != null ? String(p.tariff_id) : null,
    maxUses: hasPromoUseLimit(p.max_uses) ? String(p.max_uses) : '',
    validUntil,
    firstPurchase: p.first_purchase_only,
  }
}

function emptyState(): FormState {
  return {
    type: 'subscription_days',
    code: '',
    note: '',
    days: '',
    trialDays: '',
    hwid: '',
    percent: '',
    ttl: '24',
    ttlLegacy: false,
    // Скидка по умолчанию — на одну оплату (раньше молча ставился 0 = без ограничения).
    payments: '1',
    tariffId: null,
    maxUses: '',
    validUntil: null,
    firstPurchase: false,
  }
}

function endOfDay(d: Date): Date {
  const x = new Date(d)
  x.setHours(23, 59, 0, 0)
  return x
}

const toInt = (s: string): number | null => {
  const v = s.trim()
  if (!v) return null
  const n = Number(v)
  return Number.isFinite(n) ? Math.trunc(n) : NaN
}

const sameMinute = (a: Date | null, b: Date | null) =>
  a == null || b == null ? a === b : Math.floor(a.getTime() / 60000) === Math.floor(b.getTime() / 60000)

export function PromoFormModal({
  mode,
  open,
  onClose,
  onSaved,
  tariffs,
  salesModeTariffs,
  loyaltyEnabled,
}: {
  mode: PromoFormMode
  open: boolean
  onClose: () => void
  onSaved: (kind: 'created' | 'saved') => void
  tariffs: { id: number; name: string }[]
  salesModeTariffs: boolean
  loyaltyEnabled: boolean
}) {
  const { t } = useTranslation()
  const create = useAdminPromoCreate()
  const update = useAdminPromoUpdate()
  const isEdit = mode.kind === 'edit'
  const original = mode.kind === 'edit' ? mode.promo : null

  const initial = useMemo(
    () =>
      mode.kind === 'edit'
        ? stateFromPromo(mode.promo, { duplicate: false })
        : mode.kind === 'duplicate'
          ? stateFromPromo(mode.source, { duplicate: true })
          : emptyState(),
    // Форма пересобирается при каждом открытии.
    [open, mode.kind, mode.kind === 'edit' ? mode.promo.id : mode.kind === 'duplicate' ? mode.source.id : 0],
  )
  const [f, setF] = useState<FormState>(initial)
  const [ttlTouched, setTtlTouched] = useState(false)
  const [calendarOpen, setCalendarOpen] = useState(false)
  const [showErrors, setShowErrors] = useState(false)
  const [serverError, setServerError] = useState<{ message: string; codeTaken: boolean } | null>(null)
  const [confirmDiscard, setConfirmDiscard] = useState(false)
  const codeRef = useRef<HTMLInputElement>(null)
  const bodyRef = useRef<HTMLDivElement>(null)
  const [flash, setFlash] = useState<PromoFormFocus | null>(null)
  const focus = mode.kind === 'edit' ? mode.focus : undefined

  useEffect(() => {
    if (!open) return
    setF(initial)
    setTtlTouched(false)
    setCalendarOpen(false)
    setShowErrors(false)
    setServerError(null)
    setConfirmDiscard(false)
    if (mode.kind === 'duplicate') requestAnimationFrame(() => codeRef.current?.select())
  }, [open, initial, mode.kind])

  // Открыли с плитки подробностей — прокрутить к полю, поставить фокус и
  // коротко подсветить. Ждём конца анимации окна: во время сдвига прокрутка
  // промахивается.
  useEffect(() => {
    if (!open || !focus) return
    if (focus === 'validUntil') setCalendarOpen(true)
    const start = window.setTimeout(() => {
      const box = bodyRef.current?.querySelector<HTMLElement>(`[data-promo-field="${focus}"]`)
      if (!box) return
      // К верху поля: у срока под ним раскрывается календарь.
      box.scrollIntoView({ block: 'start', behavior: 'smooth' })
      const el = box.querySelector<HTMLElement>('input, button')
      el?.focus({ preventScroll: true })
      if (el instanceof HTMLInputElement) el.select()
      setFlash(focus)
    }, 260)
    const stop = window.setTimeout(() => setFlash(null), 1800)
    return () => {
      window.clearTimeout(start)
      window.clearTimeout(stop)
      setFlash(null)
    }
  }, [open, focus])
  const fieldBox = (name: PromoFormFocus) => ({
    'data-promo-field': name,
    className: cn('-m-1.5 scroll-mt-12 rounded-[14px] p-1.5 transition-shadow duration-500', flash === name && 'ring-2 ring-primary/60'),
  })

  const set = <K extends keyof FormState>(k: K, v: FormState[K]) => {
    setServerError(null)
    setF((p) => ({ ...p, [k]: v }))
  }

  const tariffName = (id: string | null) => (id ? tariffs.find((x) => String(x.id) === id)?.name ?? `#${id}` : null)

  // ---------- значения для отправки ----------
  const days = toInt(f.days)
  const trialDays = toInt(f.trialDays)
  const hwid = toInt(f.hwid)
  const percent = toInt(f.percent)
  const ttl = toInt(f.ttl)
  const paymentsRaw = toInt(f.payments)
  const payments = paymentsRaw == null ? 0 : paymentsRaw
  const maxUses = toInt(f.maxUses)
  const noteTrim = f.note.trim()
  const ttlUnchangedLegacy = f.ttlLegacy && !ttlTouched

  // ---------- проверка (зеркалит validateCreatePromo / validatePromoUpdateFields) ----------
  const positive = (n: number | null) => n != null && !Number.isNaN(n) && n > 0
  const errors: Partial<Record<'code' | 'main' | 'percent' | 'ttl' | 'payments' | 'maxUses' | 'validUntil' | 'note', string>> = {}
  if (!isEdit && !isValidPromoCode(f.code)) errors.code = t('admin.promos.form.errors.code')
  if (f.type === 'subscription_days' && !positive(days)) errors.main = t('admin.promos.form.errors.value')
  if (f.type === 'trial' && !positive(trialDays)) errors.main = t('admin.promos.form.errors.value')
  if (f.type === 'extra_hwid' && !positive(hwid)) errors.main = t('admin.promos.form.errors.value')
  if (f.type === 'discount') {
    if (percent == null || Number.isNaN(percent) || percent < 1 || percent > 100) errors.percent = t('admin.promos.form.errors.percent')
    if (!ttlUnchangedLegacy && ttl != null && (Number.isNaN(ttl) || ttl <= 0)) errors.ttl = t('admin.promos.form.errors.value')
    if (Number.isNaN(payments) || payments < 0) errors.payments = t('admin.promos.form.errors.payments')
  }
  if (maxUses != null && (Number.isNaN(maxUses) || maxUses <= 0)) errors.maxUses = t('admin.promos.form.errors.maxUses')
  const validUntilChanged = !sameMinute(f.validUntil, original?.valid_until ? new Date(original.valid_until) : null)
  if (f.validUntil && f.validUntil.getTime() <= Date.now() && (!isEdit || validUntilChanged)) errors.validUntil = t('admin.promos.form.errors.past')
  if ([...noteTrim].length > NOTE_MAX) errors.note = t('admin.promos.form.errors.note')
  const hasErrors = Object.keys(errors).length > 0

  const mainFilled =
    f.type === 'subscription_days' ? f.days.trim() !== '' : f.type === 'trial' ? f.trialDays.trim() !== '' : f.type === 'extra_hwid' ? f.hwid.trim() !== '' : f.percent.trim() !== ''

  // ---------- что изменилось (правка) ----------
  const changes = useMemo(() => {
    if (!original) return {} as Record<string, { value: unknown; was: string }>
    const out: Record<string, { value: unknown; was: string }> = {}
    const empty = t('admin.promos.form.wasEmpty')
    const add = (key: string, value: unknown, old: unknown, was: string) => {
      if (value !== old) out[key] = { value, was }
    }
    add('note', noteTrim || null, original.note ?? null, original.note || empty)
    const oldMax = hasPromoUseLimit(original.max_uses) ? original.max_uses : null
    add('max_uses', maxUses, oldMax, oldMax == null ? t('admin.promos.form.maxUsesPlaceholder') : String(oldMax))
    if (original.type === 'subscription_days') {
      add('subscription_days', days, original.subscription_days ?? null, String(original.subscription_days ?? empty))
      if (salesModeTariffs) {
        const newT = f.tariffId ? Number(f.tariffId) : null
        add('tariff_id', newT, original.tariff_id ?? null, tariffName(original.tariff_id != null ? String(original.tariff_id) : null) ?? t('admin.promos.form.anyTariff'))
      }
    }
    if (original.type === 'trial') add('trial_days', trialDays, original.trial_days ?? null, String(original.trial_days ?? empty))
    if (original.type === 'extra_hwid') add('extra_hwid_delta', hwid, original.extra_hwid_delta ?? null, String(original.extra_hwid_delta ?? empty))
    if (original.type === 'discount') {
      add('discount_percent', percent, original.discount_percent ?? null, `${original.discount_percent ?? empty}%`)
      if (!ttlUnchangedLegacy) {
        const oldTtl = original.discount_ttl_hours ?? null
        const wasTtl =
          oldTtl == null ? t('admin.promos.form.ttlEmpty') : oldTtl <= 0 ? t('admin.promos.form.ttlLegacyFirstPay') : t('admin.promos.form.pillHours', { count: oldTtl })
        add('discount_ttl_hours', ttl, oldTtl, wasTtl)
      }
      const oldPay = original.discount_max_subscription_payments_per_customer
      add('discount_max_subscription_payments_per_customer', payments, oldPay, oldPay > 0 ? String(oldPay) : empty)
    }
    if (validUntilChanged) {
      out.valid_until = {
        value: f.validUntil ? f.validUntil.toISOString() : null,
        was: original.valid_until ? formatPromoDateTime(original.valid_until) : t('admin.promos.form.validUntilNever'),
      }
    }
    add('first_purchase_only', f.firstPurchase, original.first_purchase_only, original.first_purchase_only ? t('admin.yes') : t('admin.no'))
    return out
  }, [original, f, ttlTouched, salesModeTariffs, tariffs, t])
  const changedCount = Object.keys(changes).length

  const dirty = isEdit
    ? changedCount > 0
    : mode.kind === 'duplicate'
      ? JSON.stringify({ ...f, code: '' }) !== JSON.stringify({ ...initial, code: '' }) || f.code !== initial.code
      : JSON.stringify(f) !== JSON.stringify(initial)

  const pending = create.isPending || update.isPending

  const requestClose = () => {
    if (pending) return
    if (dirty) setConfirmDiscard(true)
    else onClose()
  }

  const submit = () => {
    setShowErrors(true)
    if (hasErrors) return
    if (isEdit && original) {
      const fields: Record<string, unknown> = {}
      for (const [k, c] of Object.entries(changes)) fields[k] = c.value
      if (!Object.keys(fields).length) return
      update.mutate(
        { id: original.id, fields },
        {
          onSuccess: () => onSaved('saved'),
          onError: (err) => setServerError({ message: formatAdminApiError(err, t), codeTaken: false }),
        },
      )
      return
    }
    const input: CreatePromoInput = {
      code: f.code.trim().toUpperCase(),
      type: f.type,
      note: noteTrim || null,
      max_uses: maxUses,
      valid_until: f.validUntil ? f.validUntil.toISOString() : null,
      first_purchase_only: f.firstPurchase,
    }
    if (f.type === 'subscription_days') {
      input.subscription_days = days
      if (salesModeTariffs && f.tariffId) input.tariff_id = Number(f.tariffId)
    }
    if (f.type === 'trial') input.trial_days = trialDays
    if (f.type === 'extra_hwid') input.extra_hwid_delta = hwid
    if (f.type === 'discount') {
      input.discount_percent = percent
      input.discount_ttl_hours = ttl
      input.discount_max_subscription_payments_per_customer = payments
    }
    create.mutate(input, {
      onSuccess: () => onSaved('created'),
      onError: (err) =>
        setServerError({ message: formatAdminApiError(err, t), codeTaken: err instanceof ApiError && err.status === 409 }),
    })
  }

  const err = (k: keyof typeof errors) => (showErrors ? errors[k] : undefined)
  const was = (key: string) => (changes[key] ? t('admin.promos.form.was', { value: changes[key].was }) : null)

  // ---------- предпросмотр ----------
  const effectValues = {
    type: f.type,
    subscription_days: days && days > 0 ? days : null,
    trial_days: trialDays && trialDays > 0 ? trialDays : null,
    extra_hwid_delta: hwid && hwid > 0 ? hwid : null,
    discount_percent: percent && percent > 0 && percent <= 100 ? percent : null,
    discount_ttl_hours: ttlUnchangedLegacy ? original?.discount_ttl_hours ?? null : ttl != null && ttl > 0 ? ttl : null,
    discount_max_subscription_payments_per_customer: Number.isNaN(payments) ? 0 : payments,
    tariffName: f.type === 'subscription_days' && salesModeTariffs ? tariffName(f.tariffId) : null,
  }
  const effect = <PromoEffectSentence v={effectValues} />

  const legacyTtlPlaceholder =
    original && discountTtlKind(original).kind === 'firstPay' ? t('admin.promos.form.ttlLegacyFirstPay') : t('admin.promos.form.ttlEmpty')

  const title = isEdit ? t('admin.promos.form.editTitle') : t('admin.promos.form.createTitle')

  return (
    <>
      <AdminModal
        open={open}
        onClose={requestClose}
        dismissGuarded={dirty || pending}
        title={title}
        icon={isEdit ? Pencil : Ticket}
        iconAccent={isEdit ? 'amber' : 'violet'}
        panelClassName="sm:max-w-[600px]"
        footer={
          <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
            <span className="text-xs font-semibold text-amber-600 dark:text-amber-400">
              {isEdit && changedCount > 0 && (
                <span className="inline-flex items-center gap-1.5">
                  <span className="size-1.5 rounded-full bg-current" aria-hidden />
                  {t('admin.promos.form.changed', { count: changedCount })}
                </span>
              )}
            </span>
            <div className="flex gap-2.5">
              <button
                type="button"
                onClick={requestClose}
                disabled={pending}
                className="h-11 flex-1 rounded-[12px] border border-border px-4 text-sm font-semibold transition-colors hover:bg-muted disabled:opacity-50 sm:h-10 sm:flex-none"
              >
                {t('admin.promos.form.cancel')}
              </button>
              <button
                type="button"
                onClick={submit}
                disabled={pending || (isEdit ? changedCount === 0 : !f.code.trim() || !mainFilled)}
                className={cn(
                  'inline-flex h-11 flex-1 items-center justify-center gap-2 rounded-[12px] bg-primary px-4 text-sm font-semibold text-primary-foreground transition-colors hover:bg-primary/90 disabled:opacity-50 sm:h-10 sm:flex-none',
                  pending && '!opacity-100',
                )}
              >
                {pending && <Spinner />}
                {isEdit ? t('admin.promos.form.save') : t('admin.promos.form.create')}
              </button>
            </div>
          </div>
        }
      >
        <div ref={bodyRef} className="space-y-4">
          {/* А. Тип */}
          {isEdit && original ? (
            <div className="grid grid-cols-2 gap-3">
              <LockedField label={t('admin.promos.form.typeLabel')} value={promoTypeLabel(original.type, t)} hint={t('admin.promos.form.locked')} />
              <LockedField label={t('admin.promos.form.codeLabel')} value={original.code} mono hint={t('admin.promos.form.locked')} />
            </div>
          ) : (
            <div>
              <FieldLabel>{t('admin.promos.form.typeLabel')}</FieldLabel>
              <div className="grid grid-cols-2 gap-2 sm:grid-cols-4" role="radiogroup" aria-label={t('admin.promos.form.typeLabel')}>
                {PROMO_TYPES.map((ty) => {
                  const on = f.type === ty
                  return (
                    <button
                      key={ty}
                      type="button"
                      role="radio"
                      aria-checked={on}
                      onClick={() => {
                        if (on) return
                        setServerError(null)
                        setF((p) => ({
                          ...p,
                          type: ty,
                          days: '',
                          trialDays: '',
                          hwid: '',
                          percent: '',
                          ttl: '24',
                          payments: '1',
                          tariffId: null,
                        }))
                      }}
                      className={cn(
                        'flex flex-col items-start gap-2 rounded-[14px] border p-3 text-left transition-colors',
                        on ? 'border-primary bg-primary/10 ring-1 ring-inset ring-primary' : 'border-border bg-muted/30 hover:bg-muted/60',
                      )}
                    >
                      <PromoTypeTile type={ty} />
                      <span>
                        <span className="block text-[13.5px] font-bold leading-tight">{t(`admin.promos.types.${ty}`)}</span>
                        <span className="mt-0.5 block text-xs leading-snug text-muted-foreground">{t(`admin.promos.typeHints.${ty}`)}</span>
                      </span>
                    </button>
                  )
                })}
              </div>
            </div>
          )}

          {/* Б. Код */}
          {!isEdit && (
            <div>
              <FieldLabel aside={`${[...f.code].length} / ${CODE_MAX}`}>{t('admin.promos.form.codeLabel')}</FieldLabel>
              <div className="flex gap-2">
                <input
                  ref={codeRef}
                  value={f.code}
                  maxLength={CODE_MAX}
                  autoCapitalize="characters"
                  spellCheck={false}
                  onChange={(e) => set('code', e.target.value.toUpperCase())}
                  aria-invalid={Boolean(err('code') || serverError?.codeTaken)}
                  className={cn(
                    inputCls,
                    'flex-1 font-mono font-bold',
                    (err('code') || serverError?.codeTaken) && 'border-destructive ring-1 ring-destructive',
                  )}
                />
                <button
                  type="button"
                  title={t('admin.promos.form.generate')}
                  aria-label={t('admin.promos.form.generate')}
                  onClick={() => set('code', generatePromoCode())}
                  className="inline-flex size-[42px] shrink-0 items-center justify-center rounded-[12px] border border-border bg-muted/40 transition-colors hover:bg-primary/10"
                >
                  <Dice5 className="size-[18px]" />
                </button>
              </div>
              <FieldError>{serverError?.codeTaken ? t('admin.promos.codeTaken') : err('code')}</FieldError>
            </div>
          )}

          {/* Б2. Заметка */}
          <div>
            <FieldLabel aside={`${[...f.note].length} / ${NOTE_MAX}`} was={was('note')}>
              {t('admin.promos.note.label')}
            </FieldLabel>
            <input
              value={f.note}
              maxLength={NOTE_MAX}
              onChange={(e) => set('note', e.target.value)}
              placeholder={t('admin.promos.note.placeholder')}
              className={cn(inputCls, 'w-full', err('note') && 'border-destructive')}
            />
            <FieldError>{err('note')}</FieldError>
          </div>

          {/* В. Что получит пользователь */}
          <Group title={t('admin.promos.form.groupGets')}>
            {f.type === 'subscription_days' && (
              <div className={cn('grid gap-3', salesModeTariffs && 'sm:grid-cols-2')}>
                <div {...fieldBox('value')}>
                <NumberField
                  label={t('admin.promos.form.daysLabel')}
                  unit={t('admin.promos.form.daysUnit')}
                  value={f.days}
                  onChange={(v) => set('days', v)}
                  pills={[7, 14, 30, 90].map((n) => ({ label: String(n), value: String(n) }))}
                  was={was('subscription_days')}
                  error={err('main')}
                />
                </div>
                {salesModeTariffs && (
                  <div>
                    <FieldLabel was={was('tariff_id')}>{t('admin.promos.form.tariffLabel')}</FieldLabel>
                    <AdminSelect
                      value={f.tariffId}
                      options={tariffs.map((x) => ({ value: String(x.id), label: x.name }))}
                      onChange={(v) => set('tariffId', v)}
                      allowEmpty
                      emptyLabel={t('admin.promos.form.anyTariff')}
                      placeholder={t('admin.promos.form.anyTariff')}
                      ariaLabel={t('admin.promos.form.tariffLabel')}
                    />
                    <Hint>{t('admin.promos.form.tariffHint')}</Hint>
                  </div>
                )}
              </div>
            )}
            {f.type === 'trial' && (
              <>
                <div {...fieldBox('value')}>
                <NumberField
                  label={t('admin.promos.form.trialLabel')}
                  unit={t('admin.promos.form.trialUnit')}
                  value={f.trialDays}
                  onChange={(v) => set('trialDays', v)}
                  pills={[3, 7, 14].map((n) => ({ label: String(n), value: String(n) }))}
                  was={was('trial_days')}
                  error={err('main')}
                />
                </div>
                <InfoLine>{t('admin.promos.form.hintTrial')}</InfoLine>
              </>
            )}
            {f.type === 'extra_hwid' && (
              <>
                <div {...fieldBox('value')}>
                <NumberField
                  label={t('admin.promos.form.hwidLabel')}
                  unit={t('admin.promos.form.hwidUnit')}
                  value={f.hwid}
                  onChange={(v) => set('hwid', v)}
                  pills={[1, 2, 3, 5].map((n) => ({ label: String(n), value: String(n) }))}
                  was={was('extra_hwid_delta')}
                  error={err('main')}
                />
                </div>
                <InfoLine>{t('admin.promos.form.hintHwid')}</InfoLine>
              </>
            )}
            {f.type === 'discount' && (
              <>
                <div className="grid gap-3 sm:grid-cols-2">
                  <div {...fieldBox('value')}>
                  <NumberField
                    label={t('admin.promos.form.percentLabel')}
                    unit="%"
                    value={f.percent}
                    onChange={(v) => set('percent', v)}
                    pills={[10, 15, 20, 30].map((n) => ({ label: `${n}%`, value: String(n) }))}
                    was={was('discount_percent')}
                    error={err('percent')}
                  />
                  </div>
                  <NumberField
                    label={t('admin.promos.form.ttlLabel')}
                    unit={t('admin.promos.form.ttlUnit')}
                    value={f.ttl}
                    placeholder={f.ttlLegacy && !ttlTouched ? legacyTtlPlaceholder : t('admin.promos.form.ttlEmpty')}
                    onChange={(v) => {
                      setTtlTouched(true)
                      set('ttl', v)
                    }}
                    pills={[
                      { label: t('admin.promos.form.pillHours', { count: 24 }), value: '24' },
                      { label: t('admin.promos.form.pillDays', { count: 3 }), value: '72' },
                      { label: t('admin.promos.form.pillDays', { count: 7 }), value: '168' },
                      { label: '∞', value: '' },
                    ]}
                    infinityActive={f.ttl === '' && !(f.ttlLegacy && !ttlTouched)}
                    was={was('discount_ttl_hours')}
                    error={err('ttl')}
                  />
                </div>
                <div className="grid gap-3 sm:grid-cols-2">
                  <NumberField
                    label={t('admin.promos.form.paymentsLabel')}
                    unit={t('admin.promos.form.paymentsUnit')}
                    value={f.payments}
                    onChange={(v) => set('payments', v)}
                    hint={t('admin.promos.form.paymentsHint')}
                    was={was('discount_max_subscription_payments_per_customer')}
                    error={err('payments')}
                  />
                </div>
                {loyaltyEnabled && <InfoLine>{t('admin.promos.form.hintLoyalty')}</InfoLine>}
              </>
            )}
          </Group>

          {/* Г. Ограничения */}
          <Group title={t('admin.promos.form.groupLimits')}>
            <div className="grid gap-3 sm:grid-cols-2">
              <div>
              <NumberField
                label={t('admin.promos.form.maxUsesLabel')}
                value={f.maxUses}
                placeholder={t('admin.promos.form.maxUsesPlaceholder')}
                onChange={(v) => set('maxUses', v)}
                pills={[
                  { label: '10', value: '10' },
                  { label: '50', value: '50' },
                  { label: '100', value: '100' },
                  { label: '∞', value: '' },
                ]}
                infinityActive={f.maxUses === ''}
                was={was('max_uses')}
                error={err('maxUses')}
                hint={
                  original ? (
                    <>
                      {t('admin.promos.form.alreadyUsed', { count: original.uses_count })}
                      {maxUses != null && maxUses > 0 && maxUses <= original.uses_count && <> · {t('admin.promos.form.becomesExhausted')}</>}
                    </>
                  ) : undefined
                }
              />
              </div>
              <div {...fieldBox('validUntil')}>
                <FieldLabel was={was('valid_until')}>{t('admin.promos.form.validUntilLabel')}</FieldLabel>
                <button
                  type="button"
                  onClick={() => setCalendarOpen((v) => !v)}
                  aria-expanded={calendarOpen}
                  className={cn(inputCls, 'flex w-full items-center gap-2 text-left', calendarOpen && 'border-primary ring-2 ring-primary/20', err('validUntil') && 'border-destructive')}
                >
                  <CalendarDays className="size-4 shrink-0 text-muted-foreground" />
                  <span className={cn('truncate tabular-nums', !f.validUntil && 'text-muted-foreground')}>
                    {f.validUntil ? formatValidUntil(f.validUntil) : t('admin.promos.form.validUntilNever')}
                  </span>
                </button>
                <Pills
                  options={[1, 7, 30].map((n) => ({ label: t('admin.promos.form.pillDays', { count: n }), value: String(n) }))
                    .concat([{ label: '∞', value: '' }])}
                  isOn={(v) => (v === '' ? f.validUntil == null : f.validUntil != null && Math.abs(f.validUntil.getTime() - (Date.now() + Number(v) * DAY_MS)) < 60 * 60 * 1000)}
                  onPick={(v) => {
                    setCalendarOpen(false)
                    set('validUntil', v === '' ? null : new Date(Date.now() + Number(v) * DAY_MS))
                  }}
                />
                <FieldError>{err('validUntil')}</FieldError>
              </div>
            </div>
            {calendarOpen && (
              <AdminDatePicker
                className="mt-3"
                value={f.validUntil}
                minDate={new Date()}
                showTime
                onChange={(d) => set('validUntil', f.validUntil == null ? endOfDay(d) : d)}
              />
            )}
            <div className="mt-3.5">
              <div {...fieldBox('firstPurchase')}>
              <AdminCheckboxField
                checked={f.firstPurchase}
                onChange={(v) => set('firstPurchase', v)}
                label={
                  <span className="inline-flex flex-wrap items-center gap-2">
                    {t('admin.promos.form.firstPurchase')}
                    {was('first_purchase_only') && <WasChip>{was('first_purchase_only')}</WasChip>}
                  </span>
                }
              />
              </div>
            </div>
          </Group>

          {/* Д. Предпросмотр */}
          <div className="flex items-start gap-2.5 rounded-[14px] border border-primary/40 bg-primary/10 px-3.5 py-3 text-[13.5px] leading-relaxed">
            <Info className="mt-0.5 size-4 shrink-0 text-primary" />
            <div className="min-w-0">
              {isEdit ? (
                <>
                  {t('admin.promos.form.preview.after')} {effect}
                  {original?.type === 'discount' && (
                    <p className="mt-1 text-[12.5px] text-muted-foreground">{t('admin.promos.form.preview.discountEditNote')}</p>
                  )}
                </>
              ) : (
                <>
                  {effect}
                  {mainFilled && '. '}
                  <PromoPreviewTail validUntil={f.validUntil} maxUses={maxUses && maxUses > 0 ? maxUses : null} firstPurchaseOnly={f.firstPurchase} />
                </>
              )}
            </div>
          </div>

          {serverError && !serverError.codeTaken && (
            <div role="alert" className="rounded-[12px] border border-destructive/40 bg-destructive/10 px-3.5 py-2.5 text-sm text-destructive">
              {serverError.message}
            </div>
          )}
        </div>
      </AdminModal>

      <AdminConfirmModal
        open={confirmDiscard}
        onClose={() => setConfirmDiscard(false)}
        onConfirm={() => {
          setConfirmDiscard(false)
          onClose()
        }}
        title={t('admin.promos.form.discard.title')}
        message={t('admin.promos.form.discard.body')}
        confirmLabel={t('admin.promos.form.discard.close')}
        cancelLabel={t('admin.promos.form.discard.stay')}
        variant="destructive"
      />
    </>
  )
}

function formatValidUntil(d: Date): string {
  const s = formatPromoDateTime(d)
  // 23:59 — «до конца дня»: время не показываем, чтобы поле читалось как дата.
  return d.getHours() === 23 && d.getMinutes() === 59 ? s.slice(0, 8) : s
}

const inputCls =
  'h-[42px] rounded-[12px] border border-border bg-background px-3.5 dark:bg-input text-sm outline-none transition-colors placeholder:text-muted-foreground/70 focus:border-primary focus:ring-2 focus:ring-primary/20'

function FieldLabel({ children, aside, was }: { children: ReactNode; aside?: string; was?: string | null }) {
  return (
    <div className="mb-1.5 flex items-center justify-between gap-2 text-[13px] font-semibold">
      <span className="truncate">{children}</span>
      {was ? <WasChip>{was}</WasChip> : aside ? <span className="shrink-0 font-medium text-muted-foreground tabular-nums">{aside}</span> : null}
    </div>
  )
}

function WasChip({ children }: { children: ReactNode }) {
  return (
    <span className="inline-flex shrink-0 items-center gap-1.5 text-[11.5px] font-semibold text-amber-600 dark:text-amber-400">
      <span className="size-1.5 rounded-full bg-current" aria-hidden />
      {children}
    </span>
  )
}

function FieldError({ children }: { children?: ReactNode }) {
  if (!children) return null
  return <p className="mt-1.5 text-xs text-destructive">{children}</p>
}

function Hint({ children }: { children: ReactNode }) {
  return <p className="mt-1.5 text-xs text-muted-foreground">{children}</p>
}

function InfoLine({ children }: { children: ReactNode }) {
  return (
    <p className="mt-2.5 flex items-start gap-1.5 text-[12.5px] text-muted-foreground">
      <Info className="mt-0.5 size-3.5 shrink-0" />
      <span>{children}</span>
    </p>
  )
}

function Group({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="space-y-3 rounded-[16px] border border-border/60 bg-muted/25 p-3.5">
      <h4 className="text-xs font-semibold uppercase tracking-[0.07em] text-muted-foreground">{title}</h4>
      {children}
    </section>
  )
}

function LockedField({ label, value, mono, hint }: { label: string; value: string; mono?: boolean; hint: string }) {
  return (
    <div>
      <FieldLabel>{label}</FieldLabel>
      <div
        title={hint}
        aria-disabled
        className="flex h-[42px] items-center justify-between gap-2 rounded-[12px] border border-border/70 bg-muted/50 px-3.5 text-sm text-muted-foreground"
      >
        <span className={cn('truncate', mono && 'font-mono font-bold')}>{value}</span>
        <Lock className="size-3.5 shrink-0" />
      </div>
    </div>
  )
}

function Pills({
  options,
  isOn,
  onPick,
}: {
  options: { label: string; value: string }[]
  isOn: (v: string) => boolean
  onPick: (v: string) => void
}) {
  return (
    <div className="mt-2 flex flex-wrap gap-1.5">
      {options.map((o) => {
        const on = isOn(o.value)
        return (
          <button
            key={o.label}
            type="button"
            aria-pressed={on}
            onClick={() => onPick(o.value)}
            className={cn(
              'rounded-full border px-3 py-1 text-[13px] font-medium transition-colors',
              on ? 'border-primary bg-primary/10 text-foreground' : 'border-border text-muted-foreground hover:text-foreground',
            )}
          >
            {o.label}
          </button>
        )
      })}
    </div>
  )
}

function NumberField({
  label,
  unit,
  value,
  onChange,
  pills,
  placeholder,
  hint,
  was,
  error,
  infinityActive,
}: {
  label: string
  unit?: string
  value: string
  onChange: (v: string) => void
  pills?: { label: string; value: string }[]
  placeholder?: string
  hint?: ReactNode
  was?: string | null
  error?: string
  /** Пилюля «∞» подсвечена (пустое поле значит «без ограничения»). */
  infinityActive?: boolean
}) {
  return (
    <div>
      <FieldLabel was={was}>{label}</FieldLabel>
      <div className="relative">
        <input
          inputMode="numeric"
          aria-label={label}
          value={value}
          placeholder={placeholder}
          onChange={(e) => onChange(e.target.value.replace(/[^\d]/g, ''))}
          className={cn(inputCls, 'w-full', was && 'border-primary', error && 'border-destructive')}
          style={unit ? { paddingRight: `calc(${unit.length * 0.6}em + 1.25rem)` } : undefined}
        />
        {unit && (
          <span className="pointer-events-none absolute right-3.5 top-1/2 -translate-y-1/2 text-[13px] text-muted-foreground">{unit}</span>
        )}
      </div>
      {pills && (
        <Pills
          options={pills}
          isOn={(v) => (v === '' ? Boolean(infinityActive) : value !== '' && Number(value) === Number(v))}
          onPick={onChange}
        />
      )}
      {error ? <FieldError>{error}</FieldError> : hint ? <Hint>{hint}</Hint> : null}
    </div>
  )
}
