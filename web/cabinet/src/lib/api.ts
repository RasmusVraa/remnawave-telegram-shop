/**
 * API-клиент для web-кабинета.
 *
 * Особенности:
 * - Все запросы идут на /cabinet/api/*
 * - CSRF-токен читается из cookie `csrf_token` (как в internal/cabinet/auth/csrf)
 *   и передаётся в X-CSRF-Token; при необходимости fallback на `cab_csrf`
 * - При 401 (кроме /auth/*) автоматически вызывается refresh и запрос повторяется
 * - При неудаче refresh — стор сбрасывает сессию
 */

import { getCookie } from './utils'
import type {
  AdminBootstrapDTO,
  AdminBroadcastAudienceDTO,
  AdminBroadcastMediaDTO,
  AdminBroadcastPreviewDTO,
  AdminBroadcastSendDTO,
  AdminCustomerDTO,
  AdminFortuneStatsDTO,
  AdminLoyaltyStatsDTO,
  AdminLoyaltyTierDTO,
  AdminOkDTO,
  AdminPaymentsDTO,
  AdminPaymentsListDTO,
  AdminPaymentDetailDTO,
  AdminPage,
  AdminPartnerDTO,
  AdminPartnerCustomerDTO,
  AdminPartnerDetailDTO,
  AdminPartnerOperationDTO,
  AdminPartnerPayoutDTO,
  AdminPartnerPendingDTO,
  AdminPartnerTermsInput,
  AdminPromoCodeDTO,
  AdminPromoGetDTO,
  AdminPromoRedemptionsListDTO,
  AdminPromoListDTO,
  AdminPromoStatsDTO,
  AdminReferralsDTO,
  AdminOverviewDTO,
  AdminStatsDTO,
  AdminStatsInsightsDTO,
  AdminStatsTimeSeriesDTO,
  AdminTariffDTO,
  AdminTariffSquadsPreviewDTO,
  AdminTariffSquadsRunDTO,
  AdminUserPanelDTO,
  AdminUsersListDTO,
  AdminDeviceDTO,
  AdminInfraNodesDTO,
  AdminInfraProvidersDTO,
  AdminInfraHistoryDTO,
  AdminInfraSettingsDTO,
  AdminBotSettingsDTO,
  AdminBotSettingsPatchDTO,
} from './types/admin'

/** Имя cookie с double-submit CSRF (совпадает с csrf.CookieName на бэкенде). */
function readCsrfCookie(): string {
  return getCookie('csrf_token') || getCookie('cab_csrf')
}

// --- Типы ------------------------------------------------------------------

/**
 * Окно свежести подписки, триала и устройств.
 *
 * В пределах окна страница рисуется из кэша мгновенно; дальше — фоновое
 * обновление без скелетона. Держим коротким: данные меняются после оплаты
 * и активации триала, а эти сценарии инвалидируют кэш явно.
 */
export const SUBSCRIPTION_STALE_MS = 15_000

/**
 * Расход трафика по дням за расчётный период — график на главной.
 *
 * Период считается от последнего сброса счётчика в панели, а не от начала
 * календарного месяца: только так сумма сходится с лимитом тарифа рядом.
 * `enabled: false` — интеграции нет или пользователь не заведён в панели;
 * пустой `points` при `enabled: true` — законный ответ, трафика просто не было.
 */
export type TrafficUsageResponse = {
  enabled: boolean
  period_start?: string
  period_end?: string
  categories: string[]
  /** Байты по дням. */
  points: number[]
  total_bytes: number
}

export class ApiError extends Error {
  constructor(
    public readonly status: number,
    public readonly body: string,
  ) {
    super(`API ${status}: ${body}`)
    this.name = 'ApiError'
  }
}

/** Ответ login / refresh / смена пароля (совпадает с бэкендом loginResp). */
export interface AuthTokenResponse {
  access_token: string
  access_exp: number
  csrf_token: string
}

/** Ответ GET /auth/bootstrap — до JWT, для экрана логина. */
export interface AuthBootstrapResponse {
  google_oauth_enabled: boolean
  yandex_oauth_enabled?: boolean
  vk_oauth_enabled?: boolean
  telegram_widget_bot?: string
  telegram_oidc_enabled?: boolean
  telegram_web_auth_mode?: 'widget' | 'oidc'
  turnstile_enabled?: boolean
  turnstile_site_key?: string
  /** URL из env бота (SUPPORT_URL, BOT_URL и т.д.), только непустые. */
  site_links?: Record<string, string>
  /** CABINET_BRAND_NAME; по умолчанию на фронте — Cabinet. */
  brand_name?: string
  /** Полный или относительный URL логотипа для <img src>. */
  brand_logo_url?: string
  /** Аватар поддержки в чате (SUPPORT_LOGO_FILE → /cabinet/api/public/support-logo). */
  support_logo_url?: string
  /** PWA feature flag + names from CABINET_PWA_* */
  pwa_enabled?: boolean
  pwa_app_name?: string
  pwa_short_name?: string
  /** false при FORTUNE_ENABLED=false — пункт «Колесо фортуны» в меню кабинета скрыт (маршрут /fortune доступен по ссылке). */
  fortune_nav_visible?: boolean
  /** false при PARTNER_PROGRAM_ENABLED=false — пункт «Партнёрам» скрыт. */
  partner_nav_visible?: boolean
  /** Максимальная ставка партнёрки — бейдж «до N%» рядом с пунктом меню. */
  partner_max_percent?: number
  /** true при SUPPORT_BOT_API=true — чат поддержки в кабинете вместо внешней ссылки. */
  support_chat_enabled?: boolean
  /** false — только тёмная тема, переключатель в шапке скрыт (CABINET_LIGHT_THEME_ENABLED). */
  light_theme_enabled?: boolean
  /** true — при подключении Happ отдавать зашифрованный deep link (happ://crypt5/…) вместо happ://add/ (CABINET_DEEPLINK_HAPP_ENCRYPT). */
  deeplink_happ_encrypt?: boolean
  /** true — при подключении INCY отдавать обфусцированный deep link (incy://crypt1/…) вместо incy://add/ (CABINET_DEEPLINK_INCY_ENCRYPT). */
  deeplink_incy_encrypt?: boolean
  /** true — на странице подписки показывается плашка уровня лояльности (CABINET_SUBSCRIPTION_SHOW_LOYALTY, по умолчанию выключено); раздел /loyalty доступен всегда. */
  subscription_loyalty_visible?: boolean
  /**
   * Тексты и видимость секций лендинга. Пустая строка — брать перевод.
   * Секции по умолчанию включены.
   */
  landing?: {
    hero_title?: string
    hero_subtitle?: string
    note?: string
    stat_traffic_value?: string
    stat_traffic_label?: string
    stat_devices_value?: string
    stat_devices_label?: string
    show_tariffs?: boolean
    show_steps?: boolean
    show_features?: boolean
    show_faq?: boolean
  }
  /** Декоративная тема кабинета (CABINET_DECOR_THEME) */
  decor_theme?:
    | 'off'
    | 'green'
    | 'pink'
    | 'orange'
    | 'yellow'
    | 'violet'
    | 'slate'
    | 'carbon'
    | 'new_year'
    | 'summer'
    | 'neon'
    | 'halloween'
    | 'valentine'
    | 'spring'
    | 'black_friday'
    | 'aurora'
    | 'nebula'
    | 'ocean'
    | 'cyber'
    | 'sunset'
    | 'lavender'
    | 'wine'
  /** Доступные провайдеры оплаты по backend-конфигурации env. */
  payment_providers?: {
    yookassa?: boolean
    cryptopay?: boolean
    telegram?: boolean
    platega_sbp?: boolean
    platega_cards?: boolean
    platega_acquiring?: boolean
    platega_worldwide?: boolean
    platega_crypto?: boolean
    heleket?: boolean
  }
}

export interface MeResponse {
  id: number
  email?: string | null
  email_verified: boolean
  language: string
  providers: string[]
  has_telegram_link: boolean
  has_password: boolean
  /** true, если задан пароль (сценарии входа/привязки «как через почту»). */
  can_use_email_password_login?: boolean
  customer_id?: number | null
  telegram_widget_bot?: string
  google_oauth_enabled: boolean
  yandex_oauth_enabled?: boolean
  vk_oauth_enabled?: boolean
  telegram_oidc_enabled?: boolean
  /** ISO 8601 — дата регистрации аккаунта кабинета. */
  registered_at?: string
  /** Управляет видимостью блока удаления аккаунта в профиле. */
  can_delete_account_ui?: boolean
  /** Числовой Telegram user id, если известен. */
  telegram_id?: number | null
  /** Маска почты из OAuth identity (без sub). */
  google_masked_email?: string | null
  yandex_masked_email?: string | null
  vk_masked_email?: string | null
  /** true, если linked Telegram identity == ADMIN_TELEGRAM_ID. */
  is_admin?: boolean
  /** Имя для шапки профиля: Telegram → OAuth. Пусто — показываем ник или почту. */
  display_name?: string
  /** Ник Telegram без «@». */
  username?: string
  /**
   * Аватарка: подписанная ссылка на свой /cabinet/api/avatar (Telegram) либо
   * прямая ссылка провайдера (Google/Яндекс/VK). Пусто — рисуем инициалы.
   */
  avatar_url?: string
  /** Главный способ входа — бейдж на аватарке. Не всегда источник картинки. */
  identity_provider?: 'telegram' | 'google' | 'yandex' | 'vk'
}

export interface MergeCustomerSnapshot {
  id: number
  expire_at?: string
  loyalty_xp: number
  extra_hwid: number
  is_web_only: boolean
  has_subscription: boolean
  current_tariff_id?: number | null
}

export interface MergePreviewResponse {
  customer_web?: MergeCustomerSnapshot
  customer_tg?: MergeCustomerSnapshot
  /** Email-peer merge при привязанном Telegram: customer_web=peer, customer_tg=текущий; UI меняет местами карточки. */
  ui_swap_sides?: boolean
  merged_expire_at?: string | null
  merged_loyalty_xp: number
  merged_extra_hwid: number
  purchases_moved: number
  referrals_moved: number
  is_noop: boolean
  is_dangerous: boolean
  danger_reason?: string
  requires_subscription_choice?: boolean
  claim_expires_at?: string
}

export interface MergeConfirmResponse {
  result: string
  customer_id: number
  purchases_moved?: number
  referrals_moved?: number
}

export interface TariffItem {
  id: number | null
  slug: string
  name: string
  /** Текст из админки (tariff.description), если задан. */
  description?: string | null
  /** Подробное описание для страницы выбора срока (?plan=). */
  description_detail?: string | null
  price_rub: number
  monthly_base_rub: number
  months: number
  device_limit: number
  traffic_gb: number | null
  is_popular?: boolean
}

export interface TariffsResponse {
  tariffs: TariffItem[]
  sales_mode: string
  currency?: string
  show_savings?: boolean
  price_display?: 'monthly' | 'marketing'
  savings_badge?: 'none' | 'corner' | 'old_price'
}

/** Как отдаёт GET /tariffs (internal/cabinet/service/catalog.go): тариф + вложенные prices. */
interface TariffPriceDTO {
  months: number
  amount_rub: number
  monthly_base_rub: number
}

interface TariffViewDTO {
  id: number
  slug: string
  name?: string | null
  description?: string | null
  description_detail?: string | null
  device_limit: number
  traffic_limit_bytes: number
  traffic_limit_reset_strategy?: string
  prices: TariffPriceDTO[]
}

interface TariffsRawResponse {
  sales_mode: string
  currency?: string
  show_savings?: boolean
  price_display?: 'monthly' | 'marketing'
  savings_badge?: 'none' | 'corner' | 'old_price'
  tariffs: unknown[]
}

function trafficBytesToGb(bytes: number): number | null {
  if (!Number.isFinite(bytes) || bytes <= 0) return null
  return Math.round(bytes / (1024 * 1024 * 1024))
}

/**
 * Бэкенд отдаёт витрину как TariffView[] с полем prices[]; UI (тарифы, чекаут)
 * ожидает плоский список строк «период × тариф». Без этого на /tariffs падает
 * рендер (undefined.toLocaleString и т.п.).
 */
export function normalizeTariffsResponse(raw: TariffsRawResponse): TariffsResponse {
  const rows: TariffItem[] = []
  const list = raw.tariffs ?? []
  for (const item of list) {
    if (!item || typeof item !== 'object') continue
    const t = item as Partial<TariffViewDTO> & Partial<TariffItem>
    if (Array.isArray(t.prices) && t.prices.length > 0) {
      const slugStr = String(t.slug ?? '')
      const nameStr =
        t.name != null && String(t.name).trim() !== '' ? String(t.name) : slugStr
      const tid = typeof t.id === 'number' ? t.id : null
      const trafficGb = trafficBytesToGb(Number(t.traffic_limit_bytes) || 0)
      const devLim = typeof t.device_limit === 'number' ? t.device_limit : 0
      const desc =
        t.description != null && String(t.description).trim() !== '' ? String(t.description).trim() : null
      const descDetail =
        t.description_detail != null && String(t.description_detail).trim() !== ''
          ? String(t.description_detail).trim()
          : null
      for (const p of t.prices) {
        if (!p || typeof p.months !== 'number') continue
        const months = p.months
        const amount = typeof p.amount_rub === 'number' ? p.amount_rub : 0
        // Как в боте (renderTariffMonthChoice): периоды с amount_rub <= 0 не продаются.
        if (amount <= 0) continue
        const perMonth = months > 0 ? Math.round(amount / months) : amount
        rows.push({
          id: tid,
          slug: slugStr,
          name: nameStr,
          description: desc,
          description_detail: descDetail,
          price_rub: amount,
          monthly_base_rub: perMonth,
          months,
          device_limit: devLim,
          traffic_gb: trafficGb,
        })
      }
      continue
    }
    if (
      typeof t.slug === 'string' &&
      typeof t.months === 'number' &&
      typeof t.price_rub === 'number' &&
      typeof t.monthly_base_rub === 'number' &&
      t.price_rub > 0
    ) {
      rows.push(item as TariffItem)
    }
  }
  return {
    sales_mode: raw.sales_mode,
    tariffs: rows,
    currency: raw.currency,
    show_savings: raw.show_savings,
    price_display: raw.price_display === 'marketing' ? 'marketing' : 'monthly',
    savings_badge:
      raw.savings_badge === 'corner' || raw.savings_badge === 'old_price'
        ? raw.savings_badge
        : 'none',
  }
}

export interface SubscriptionHwidExtraInfo {
  enabled: boolean
  ui_visible: boolean
  current_limit: number
  base_limit: number
  max_limit: number
  extra_active: number
  can_increase: boolean
  can_decrease: boolean
  price_rub_month: number
  stars_price_month: number
  days_left: number
}

export interface SubscriptionResponse {
  expire_at: string | null
  subscription_link: string | null
  /** Срок оплаченного периода в месяцах (classic и tariffs), если известен. */
  subscription_period_months?: number | null
  traffic_used_gb?: number | null
  traffic_limit_gb?: number | null
  /** Пробный период: есть активная подписка, но нет оплаченных покупок с month>0. */
  is_trial?: boolean
  tariff: {
    id: number | null
    slug: string
    name: string
    traffic_gb: number | null
    device_limit: number
  } | null
  loyalty_xp: number
  loyalty_tier: string | null
  hwid_extra?: SubscriptionHwidExtraInfo | null
}

/** GET /payments/hwid/preview */
export interface HwidExtraPreviewResponse {
  current_limit: number
  target_limit: number
  delta: number
  days_left: number
  amount_rub: number
  base_amount_rub: number
  currency: string
  loyalty_discount_pct?: number
  promo_discount_pct?: number
  total_discount_pct?: number
}

export interface DeviceInfo {
  hwid: string
  platform?: string
  os_version?: string
  device_model?: string
  user_agent?: string
  /** Название, которое пользователь дал устройству в кабинете. */
  custom_name?: string
  created_at?: string
  updated_at?: string
}

export interface DevicesResponse {
  enabled: boolean
  device_limit: number
  connected: number
  devices: DeviceInfo[]
}

/** Ответ GET /me/connect-invite: короткая ссылка на страницу подключения. */
export interface ConnectInviteResponse {
  url: string
  expires_at: string
}

/**
 * Ответ GET /public/connect: чем подключаться на устройстве без кабинета.
 *
 * mode=encrypted — админ включил шифрование deep link, и сырая ссылка
 * подписки наружу не отдаётся: только готовые ссылки по приложениям.
 * mode=plain — шифрование выключено, страница собирает deep link сама.
 */
export interface ConnectResolveResponse {
  mode: 'plain' | 'encrypted'
  subscription_link?: string
  links?: Record<string, string>
}

export interface LoyaltyTierDTO {
  sort_order: number
  xp_min: number
  discount_percent: number
  display_name?: string | null
}

export interface LoyaltyDashboardResponse {
  enabled: boolean
  xp: number
  current?: LoyaltyTierDTO | null
  next?: LoyaltyTierDTO | null
  progress_percent: number
  xp_in_segment: number
  xp_segment_span: number
  xp_until_next: number
  first_discount_xp_min?: number | null
}

export interface LoyaltyHistoryItem {
  purchase_id: number
  fortune_spin_id?: number
  /** Пусто — начисление с оплаты; `fortune_wheel` — колесо фортуны */
  source?: string
  paid_at?: string
  xp_gained: number
  amount: number
  currency: string
  invoice_type: string
  purchase_kind: string
  running_xp: number
}

export interface LoyaltyHistoryResponse {
  items: LoyaltyHistoryItem[]
}

export interface TrialInfoResponse {
  enabled: boolean
  can_activate: boolean
  days: number
  traffic_gb: number
  device_limit: number
}

export interface CheckoutResponse {
  payment_url: string
  checkout_id: number
  purchase_id: number
  status: string
  provider?: string
  reused?: boolean
}

/** GET /payments/preview — сумма и сценарий как при создании счёта (апгрейд/даунгрейд). */
export interface PaymentPreviewResponse {
  amount: number
  currency: 'RUB' | 'STARS' | string
  amount_rub: number
  sales_mode: string
  scenario: string
  purchase_kind?: string
  is_early_downgrade?: boolean
  list_price_rub?: number
  base_amount_rub?: number
  loyalty_discount_pct?: number
  promo_discount_pct?: number
  total_discount_pct?: number
  /** Апгрейд/даунгрейд: разбивка дней (как в боте). */
  tariff_switch_remaining_days?: number
  tariff_switch_bonus_days?: number
  tariff_switch_period_days?: number
  tariff_switch_total_days?: number
  extra_hwid_active?: number
  extra_hwid_included?: boolean
  extra_hwid_amount_rub?: number
}

export interface PaymentStatusResponse {
  status: 'new' | 'pending' | 'paid' | 'failed' | 'expired'
  subscription_link: string | null
  /** paid: subscription | extra_hwid | tariff_upgrade — для текста успеха. */
  purchase_kind?: string
  /** Реквизиты чека на экране успешной оплаты (см. StatusResult в checkout.go). */
  payment_id?: number
  amount?: number
  currency?: string
  invoice_type?: string
  paid_at?: string
  month?: number
  extra_hwid?: number
  /** Дата окончания подписки после оплаты. */
  expire_at?: string
}

export interface ReferralsStatsResponse {
  total: number
  paid: number
  active: number
  conversion_pct: number
  earned_days_total: number
  earned_days_last_month: number
  referral_days_per_paid_default: number
}

/** Одно начисление в ленте «откуда взялись дни». */
export interface ReferralLedgerRow {
  /** Кто принёс дни — уже замаскировано сервером: «@i***k», «d***y@mail.ru». */
  actor: string
  days: number
  /** Длина оплаченного периода в месяцах; 0 — неизвестна (строки бэкфилла). */
  months?: number
  kind: 'first_referrer' | 'repeat_referrer' | 'default_referrer' | 'manual' | string
  created_at: string
}

export interface ReferralsResponse {
  referrer_telegram_id: number
  stats: ReferralsStatsResponse
  /** `name` приходит уже замаскированным: полных ников и почт в ответе нет. */
  referees: {
    telegram_id_masked: string
    name: string
    /** Сколько дней принёс этот приглашённый. */
    earned_days: number
    active: boolean
  }[]
  ledger: ReferralLedgerRow[]
  bot_start_link?: string
  cabinet_register_link?: string
  referral_mode: string
  referral_bonus_days_default?: number
  referral_first_referrer_days?: number
  referral_first_referee_days?: number
  referral_repeat_referrer_days?: number
  /** Помесячное начисление бонуса пригласившему. */
  referral_scale_by_months?: boolean
}

/* --- Партнёрская программа ------------------------------------------------
 *
 * Не путать с реферальной: рефералка платит днями подписки, партнёрка —
 * деньгами, и у неё есть баланс, холд и заявки на вывод. Клиент состоит ровно
 * в одной из программ.
 */

/** Условия программы — значения по умолчанию из настроек бота. */
export interface PartnerTerms {
  first_percent: number
  renewal_percent: number
  hold_days: number
  min_payout: number
  payout_cooldown_days: number
  max_links: number
  count_extra_hwid: boolean
}

export interface PartnerApplication {
  about?: string
  channels?: string
  expected?: string
  submitted_at?: string
  /** Комментарий админа: при отказе объясняет причину. */
  admin_note?: string
}

/** Ссылка партнёра. Основная (is_default) и потоки — одна сущность. */
export interface PartnerLinkDTO {
  id: number
  code: string
  name: string
  is_default: boolean
  archived: boolean
  bot_link?: string
  web_link?: string
  customers: number
  paying: number
  earned: number
  /** Пустой поток удаляется физически. */
  can_delete: boolean
  /** Поток с историей можно только заархивировать. */
  can_archive: boolean
}

export interface PartnerSummaryDTO {
  customers: number
  customers_last_week: number
  paying: number
  active: number
  conversion_pct: number
  earned_total: number
  earned_last_month: number
}

export interface PartnerMonthDTO {
  /** YYYY-MM */
  month: string
  amount: number
}

export interface PartnerAccountDTO {
  balance: number
  hold_balance: number
  reserved_balance: number
  total_earned: number
  total_paid: number
  /** Эффективные проценты партнёра: индивидуальные либо общие. */
  first_percent: number
  renewal_percent: number
  next_hold_release_at?: string
  can_withdraw: boolean
  /** Заполнено, пока действует кулдаун между выплатами. */
  payout_available_at?: string
  has_open_payout: boolean
  payout_method?: string
  payout_details?: string
  links_used: number
  links_limit: number
  summary: PartnerSummaryDTO
  months: PartnerMonthDTO[]
  links: PartnerLinkDTO[]
}

export type PartnerStatus = 'none' | 'pending' | 'active' | 'suspended' | 'rejected'

export interface PartnerStateResponse {
  enabled: boolean
  applications_enabled: boolean
  status: PartnerStatus
  terms: PartnerTerms
  application?: PartnerApplication
  /** Заполнено только у партнёра в работе. */
  partner?: PartnerAccountDTO
}

export interface PartnerCustomerDTO {
  /** Маскированная подпись: контакт клиента партнёру не отдаётся. */
  label: string
  active: boolean
  has_paid: boolean
  earned: number
  link_name?: string
  attached_at: string
}

export interface PartnerEarningDTO {
  id: number
  amount: number
  percent: number
  base_amount: number
  base_currency: string
  base_amount_rub: number
  kind: 'first' | 'renewal' | 'adjustment'
  status: 'hold' | 'available' | 'cancelled'
  hold_until?: string
  note?: string
  customer_label?: string
  link_name?: string
  created_at: string
}

export interface PartnerPayoutDTO {
  id: number
  amount: number
  status: 'pending' | 'approved' | 'paid' | 'rejected'
  method?: string
  admin_comment?: string
  external_ref?: string
  requested_at: string
  processed_at?: string
}

export interface SupportMessageDTO {
  id: number
  direction: 'in' | 'out'
  text: string
  author_label?: string
  delivery_status?: 'pending' | 'sent' | 'failed'
  created_at: string
}

export interface SupportSummaryResponse {
  has_open_ticket: boolean
  ticket_status?: string
  unread_count: number
}

export interface SupportConversationResponse extends SupportSummaryResponse {
  messages: SupportMessageDTO[]
}

export interface FortuneSectorDTO {
  index: number
  reward_type: string
  display_days?: number
  display_percent?: number
}

export interface FortuneStatusResponse {
  enabled: boolean
  panel_ready: boolean
  can_spin: boolean
  reason_code?: string
  spins_used_today: number
  max_spins_per_day: number
  /** FORTUNE_DAILY_FREE_SPIN */
  daily_free_enabled?: boolean
  /** Сегодня по UTC ещё не использован ежедневный бесплатный спин */
  daily_free_available?: boolean
  min_subscription_days: number
  /** FORTUNE_SPIN_COST_DAYS: платный спин списывает столько дней подписки. */
  spin_cost_days?: number
  subscription_remain_hours?: number
  /** Целые дни остатка подписки (ceil от часов), для UI без даты окончания. */
  subscription_remain_days?: number
  expire_at?: string
  sectors: FortuneSectorDTO[]
  /** Лента победителей: FORTUNE_WINNER_TICKER_ENABLED и fake-fill. */
  winner_feed?: FortuneWinnerFeedMeta | null
}

export interface FortuneWinnerFeedMeta {
  enabled: boolean
  fake_fill: boolean
}

export interface FortuneRecentWinItem {
  spin_at: string
  reward_type: string
  reward_value: number
  masked_name: string
}

export interface FortuneRecentWinsResponse {
  items: FortuneRecentWinItem[]
}

export interface FortuneSpinResponse {
  sector_index: number
  reward_type: string
  reward_value: number
  cost_days: number
  is_free_spin: boolean
  is_daily_free?: boolean
  new_expire_at?: string
  loyalty_xp_new?: number
}

export interface PurchaseHistoryItem {
  id: number
  amount: number
  currency: string
  status: string
  invoice_type: string
  purchase_kind: string
  month: number
  /** Число доп. слотов HWID в строке покупки (как в боте). */
  extra_hwid?: number
  paid_at?: string
  created_at: string
}

export interface PurchasesResponse {
  items: PurchaseHistoryItem[]
}

export interface PromoStateResponse {
  has_pending_discount: boolean
  pending_discount?: {
    promo_code_id: number
    percent: number
    until_first_purchase: boolean
    subscription_payments_remaining: number
    expires_at?: string
  } | null
}

export interface PromoApplyResponse {
  applied: boolean
  type: 'subscription_days' | 'trial' | 'extra_hwid' | 'discount'
  subscription_days?: number
  trial_days?: number
  extra_hwid_delta?: number
  discount_percent?: number
  trial_skipped_active_sub?: boolean
}

/** GET /admin/bootstrap — feature flags для SPA-админки. */
export type AdminBootstrapResponse = AdminBootstrapDTO

// --- Singleton клиент -------------------------------------------------------

const BASE = '/cabinet/api'

// Ленивый импорт стора (избегаем circular dep: store → api → store).
type AuthStoreRef = {
  getAccessToken: () => string | null
  setToken: (token: string) => void
  logout: () => void
}

let _authRef: AuthStoreRef | null = null

export function setAuthStoreRef(ref: AuthStoreRef) {
  _authRef = ref
}

// Состояние refresh-in-progress для deduplication.
let _refreshing: Promise<string | null> | null = null

async function doRefresh(): Promise<string | null> {
  if (_refreshing) return _refreshing
  _refreshing = (async () => {
    try {
      const csrf = readCsrfCookie()
      const res = await fetch(`${BASE}/auth/refresh`, {
        method: 'POST',
        headers: csrf ? { 'X-CSRF-Token': csrf } : {},
        credentials: 'include',
      })
      if (!res.ok) return null
      const data: AuthTokenResponse = await res.json()
      _authRef?.setToken(data.access_token)
      return data.access_token
    } catch {
      return null
    } finally {
      _refreshing = null
    }
  })()
  return _refreshing
}

async function request<T>(
  method: string,
  path: string,
  body?: unknown,
  extraHeaders?: Record<string, string>,
  retrying = false,
): Promise<T> {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...extraHeaders,
  }

  const csrf = readCsrfCookie()
  if (csrf) headers['X-CSRF-Token'] = csrf

  const token = _authRef?.getAccessToken()
  if (token) headers['Authorization'] = `Bearer ${token}`

  const res = await fetch(BASE + path, {
    method,
    headers,
    body: body !== undefined ? JSON.stringify(body) : undefined,
    credentials: 'include',
  })

  // Auto-refresh: только один раз, только для защищённых запросов.
  if (res.status === 401 && !retrying && !path.startsWith('/auth/')) {
    const newToken = await doRefresh()
    if (newToken) {
      return request<T>(method, path, body, extraHeaders, true)
    }
    _authRef?.logout()
    throw new ApiError(401, 'Session expired')
  }

  if (!res.ok) {
    const text = await res.text().catch(() => '')
    throw new ApiError(res.status, text)
  }

  // 204 No Content
  if (res.status === 204) return undefined as T

  return res.json() as Promise<T>
}

/** Собирает ?limit=&offset= для листингов с пагинацией. */
function pageQuery(params?: { limit?: number; offset?: number }): string {
  const q = new URLSearchParams()
  if (params?.limit != null) q.set('limit', String(params.limit))
  if (params?.offset != null) q.set('offset', String(params.offset))
  return q.toString() ? `?${q.toString()}` : ''
}

// --- Методы ----------------------------------------------------------------

export const api = {
  // Auth (отдельный fetch: не кэшировать, иначе после смены env/nginx долго «нет Telegram»).
  authBootstrap: async (signal?: AbortSignal): Promise<AuthBootstrapResponse> => {
    const res = await fetch(`${BASE}/auth/bootstrap`, {
      method: 'GET',
      credentials: 'include',
      cache: 'no-store',
      headers: { Accept: 'application/json' },
      signal,
    })
    if (!res.ok) {
      const text = await res.text().catch(() => '')
      throw new ApiError(res.status, text)
    }
    return res.json() as Promise<AuthBootstrapResponse>
  },

  login: (email: string, password: string, turnstileToken?: string) =>
    request<AuthTokenResponse>(
      'POST',
      '/auth/login',
      { email, password },
      turnstileToken ? { 'X-Turnstile-Token': turnstileToken } : undefined,
    ),

  register: (email: string, password: string, referralCode?: string, turnstileToken?: string) =>
    request<{ message?: string }>(
      'POST',
      '/auth/register',
      {
        email,
        password,
        ...(referralCode ? { referral_code: referralCode } : {}),
      },
      turnstileToken ? { 'X-Turnstile-Token': turnstileToken } : undefined,
    ),

  logout: () =>
    request<void>('POST', '/auth/logout'),

  refresh: () =>
    request<AuthTokenResponse>('POST', '/auth/refresh'),

  /** Автовход из Telegram Mini App (без Bearer). */
  telegramAuthMiniApp: (initData: string, referralCode?: string) =>
    request<AuthTokenResponse>('POST', '/auth/telegram', {
      source: 'miniapp',
      init_data: initData,
      ...(referralCode ? { referral_code: referralCode } : {}),
    }),

  /** Вход через Telegram Login Widget (без Bearer). */
  telegramAuthWidget: (
    user: {
      id: number
      first_name?: string
      last_name?: string
      username?: string
      photo_url?: string
      auth_date: number
      hash: string
    },
    referralCode?: string,
  ) =>
    request<AuthTokenResponse>('POST', '/auth/telegram', {
      source: 'widget',
      id: user.id,
      first_name: user.first_name,
      last_name: user.last_name,
      username: user.username,
      photo_url: user.photo_url,
      auth_date: user.auth_date,
      hash: user.hash,
      ...(referralCode ? { referral_code: referralCode } : {}),
    }),

  confirmEmail: (token: string) =>
    request<AuthTokenResponse>('POST', '/auth/email/verify/confirm', { token }),

  resendVerifyEmail: () =>
    request<void>('POST', '/me/email/verify/resend'),

  /** Повторная отправка кода подтверждения email без JWT (после регистрации). */
  resendVerifyEmailPublic: (email: string, turnstileToken?: string) =>
    request<void>(
      'POST',
      '/auth/email/verify/resend-public',
      { email },
      turnstileToken ? { 'X-Turnstile-Token': turnstileToken } : undefined,
    ),

  forgotPassword: (email: string, turnstileToken?: string) =>
    request<void>(
      'POST',
      '/auth/password/forgot',
      { email },
      turnstileToken ? { 'X-Turnstile-Token': turnstileToken } : undefined,
    ),

  resetPassword: (token: string, newPassword: string) =>
    request<{ message?: string }>('POST', '/auth/password/reset', { token, new_password: newPassword }),

  // Me
  me: () =>
    request<MeResponse>('GET', '/me'),

  putLanguage: (language: string) =>
    request<void>('PUT', '/me/language', { language }),

  changePassword: (currentPassword: string, newPassword: string) =>
    request<AuthTokenResponse>('PUT', '/me/password', {
      current_password: currentPassword,
      new_password: newPassword,
    }),

  /** Мягкое снятие привязки google/yandex/vk/email (Telegram отключён на бэкенде). */
  identityUnlink: (provider: 'google' | 'yandex' | 'vk' | 'telegram' | 'email') =>
    request<{ ok: boolean; soft_unlinked?: boolean; rows?: number }>('POST', '/me/identities/unlink', {
      provider,
    }),

  /** Привязка email+пароля к текущему аккаунту (OAuth/Telegram). */
  linkEmail: (email: string, password: string, passwordConfirm: string) =>
    request<{ status: string; reason_code?: string; masked_email?: string; message?: string }>('POST', '/me/email/link', {
      email,
      password,
      password_confirm: passwordConfirm,
    }),
  /** Подтверждение merge-кода (если email занят OAuth-only аккаунтом без пароля). */
  confirmEmailMergeCode: (code: string) =>
    request<{ status: string; reason_code?: string; masked_email?: string }>('POST', '/me/email/link/verify-code', { code }),

  /** Удаление аккаунта кабинета (необратимо). */
  deleteAccount: () =>
    request<{ message: string }>('POST', '/me/account/delete'),

  // Subscription
  subscription: () =>
    request<SubscriptionResponse>('GET', '/me/subscription'),

  /** Зашифрованный deep link подключения (happ://crypt5/… или incy://crypt1/…). */
  deeplink: (app: 'happ' | 'incy') =>
    request<{ deeplink: string }>('GET', `/me/deeplink?app=${encodeURIComponent(app)}`),

  /** Ссылка-приглашение на устройство, с которого в кабинет не зайти. */
  connectInvite: () =>
    request<ConnectInviteResponse>('GET', '/me/connect-invite'),

  /**
   * Резолв приглашения на публичной странице /connect.
   *
   * Мимо request(): у гостя нет сессии, а тот на 401 дёргает refresh и
   * разлогинивает — на странице приглашения это лишнее. Код ошибки
   * (invite_expired и т.п.) достаём из тела и кладём в ApiError.body.
   */
  publicConnect: async (token: string): Promise<ConnectResolveResponse> => {
    const res = await fetch(`${BASE}/public/connect?t=${encodeURIComponent(token)}`, {
      method: 'GET',
      cache: 'no-store',
      headers: { Accept: 'application/json' },
    })
    const raw = await res.text().catch(() => '')
    if (!res.ok) {
      let code = raw
      try {
        code = (JSON.parse(raw) as { error?: string }).error || raw
      } catch {
        /* тело не JSON — отдаём как есть */
      }
      throw new ApiError(res.status, code)
    }
    return JSON.parse(raw) as ConnectResolveResponse
  },

  loyalty: () => request<LoyaltyDashboardResponse>('GET', '/me/loyalty'),

  loyaltyHistory: (params?: { limit?: number; offset?: number }) => {
    const q = new URLSearchParams()
    if (params?.limit != null) q.set('limit', String(params.limit))
    if (params?.offset != null) q.set('offset', String(params.offset))
    const suffix = q.toString() ? `?${q.toString()}` : ''
    return request<LoyaltyHistoryResponse>('GET', `/me/loyalty/history${suffix}`)
  },

  referrals: () =>
    request<ReferralsResponse>('GET', '/me/referrals'),

  partnerState: () => request<PartnerStateResponse>('GET', '/me/partner'),

  partnerApply: (body: { about: string; channels: string; expected: string }) =>
    request<{ status: PartnerStatus }>('POST', '/me/partner/apply', body),

  partnerCustomers: (params?: { limit?: number; offset?: number }) =>
    request<{ items: PartnerCustomerDTO[]; total: number }>(
      'GET',
      `/me/partner/customers${pageQuery(params)}`,
    ),

  partnerEarnings: (params?: { limit?: number; offset?: number }) =>
    request<{ items: PartnerEarningDTO[]; total: number }>(
      'GET',
      `/me/partner/earnings${pageQuery(params)}`,
    ),

  partnerCreateLink: (name: string) =>
    request<PartnerLinkDTO>('POST', '/me/partner/links', { name }),

  partnerUpdateLink: (id: number, body: { name?: string; archived?: boolean }) =>
    request<{ ok: boolean }>('PATCH', `/me/partner/links/${id}`, body),

  partnerDeleteLink: (id: number) =>
    request<{ ok: boolean }>('DELETE', `/me/partner/links/${id}`),

  partnerSavePayoutDetails: (method: string, details: string) =>
    request<{ ok: boolean }>('PUT', '/me/partner/payout-details', { method, details }),

  partnerPayouts: (params?: { limit?: number; offset?: number }) =>
    request<{ items: PartnerPayoutDTO[]; total: number }>('GET', `/me/partner/payouts${pageQuery(params)}`),

  partnerRequestPayout: (amount: number) =>
    request<PartnerPayoutDTO>('POST', '/me/partner/payouts', { amount }),

  supportSummary: () => request<SupportSummaryResponse>('GET', '/support/summary'),

  supportConversation: () => request<SupportConversationResponse>('GET', '/support/conversation'),

  supportSendMessage: (text: string) =>
    request<SupportMessageDTO>('POST', '/support/messages', { text }),

  supportMarkRead: () => request<{ ok: boolean }>('POST', '/support/read'),

  fortuneStatus: () => request<FortuneStatusResponse>('GET', '/fortune/status'),

  fortuneRecentWins: () => request<FortuneRecentWinsResponse>('GET', '/fortune/recent-wins'),

  fortuneSpin: () => request<FortuneSpinResponse>('POST', '/fortune/spin', {}),

  promoState: () => request<PromoStateResponse>('GET', '/promocodes/state'),
  applyPromoCode: (code: string) =>
    request<PromoApplyResponse>('POST', '/promocodes/apply', { code }),

  purchases: (params?: { limit?: number; offset?: number }) => {
    const q = new URLSearchParams()
    if (params?.limit != null) q.set('limit', String(params.limit))
    if (params?.offset != null) q.set('offset', String(params.offset))
    const suffix = q.toString() ? `?${q.toString()}` : ''
    return request<PurchasesResponse>('GET', `/me/purchases${suffix}`)
  },

  trialInfo: () =>
    request<TrialInfoResponse>('GET', '/me/trial'),

  activateTrial: () =>
    request<{ ok: boolean; subscription_link?: string; message?: string }>('POST', '/me/trial/activate'),

  devices: () =>
    request<DevicesResponse>('GET', '/me/devices'),

  trafficUsage: () =>
    request<TrafficUsageResponse>('GET', '/me/traffic-usage'),

  deleteDevice: (hwid: string) =>
    request<{ ok: boolean }>('POST', '/me/devices/delete', { hwid }),

  /** Пустое name сбрасывает название к исходному из Remnawave. */
  renameDevice: (hwid: string, name: string) =>
    request<{ ok: boolean; custom_name: string }>('POST', '/me/devices/rename', { hwid, name }),

  // Tariffs
  tariffs: () => request<TariffsRawResponse>('GET', '/tariffs').then(normalizeTariffsResponse),

  publicStatus: () =>
    request<{
      available: boolean
      updated_at?: string
      online: number
      total: number
      show_map?: boolean
      probes_enabled?: boolean
      probe_world?: number
      probe_russia?: number
      probe_interval_min?: number
      title?: string
      lead?: string
      nodes: {
        name: string
        country?: string
        note?: string
        whitelist?: boolean
        state: string
        probe?: {
          world_ping_ms?: number
          world_ok: number
          world_total: number
          russia_ok: number
          russia_total: number
          history?: Array<number | null>
          hits?: {
            city?: string
            network?: string
            country?: string
            lat?: number
            lon?: number
            ping_ms?: number
            ok?: boolean
            kind?: string
            answer?: string
          }[]
        }
      }[]
    }>('GET', '/public/status'),

  // Payments — тело как в internal/cabinet/http/handlers/payments.go: period, tariff_id, provider.
  checkout: (
    input: { period: number; provider: string; tariffId?: number | null; renewExtraHwid?: boolean },
    idempotencyKey: string,
  ) => {
    const body: Record<string, unknown> = {
      period: input.period,
      provider: input.provider,
      renew_extra_hwid: Boolean(input.renewExtraHwid),
    }
    if (input.tariffId != null && input.tariffId > 0) {
      body.tariff_id = input.tariffId
    }
    return request<CheckoutResponse>('POST', '/payments/checkout', body, { 'Idempotency-Key': idempotencyKey })
  },

  paymentPreview: (period: number, tariffId?: number | null, provider?: string, renewExtraHwid?: boolean) => {
    const q = new URLSearchParams()
    q.set('period', String(period))
    if (tariffId != null && tariffId > 0) q.set('tariff_id', String(tariffId))
    if (provider) q.set('provider', provider)
    if (renewExtraHwid != null) q.set('renew_extra_hwid', String(renewExtraHwid))
    return request<PaymentPreviewResponse>('GET', `/payments/preview?${q.toString()}`)
  },

  hwidExtraPreview: (targetLimit: number, provider?: string) => {
    const q = new URLSearchParams()
    q.set('target_limit', String(targetLimit))
    if (provider) q.set('provider', provider)
    return request<HwidExtraPreviewResponse>('GET', `/payments/hwid/preview?${q.toString()}`)
  },

  hwidExtraCheckout: (targetLimit: number, provider: string, idempotencyKey: string) =>
    request<CheckoutResponse>('POST', '/payments/hwid/checkout', { target_limit: targetLimit, provider }, {
      'Idempotency-Key': idempotencyKey,
    }),

  hwidExtraApply: (targetLimit: number) =>
    request<{ ok: boolean; device_limit: number }>('POST', '/me/hwid-extra/apply', { target_limit: targetLimit }),

  paymentStatus: (id: number) =>
    request<PaymentStatusResponse>('GET', `/payments/${id}/status`),

  // Admin
  adminBootstrap: () =>
    request<AdminBootstrapResponse>('GET', '/admin/bootstrap'),

  adminOverview: (tz?: string) => {
    const q = tz ? `?tz=${encodeURIComponent(tz)}` : ''
    return request<AdminOverviewDTO>('GET', `/admin/overview${q}`)
  },
  adminStats: () => request<AdminStatsDTO>('GET', '/admin/stats'),
  adminStatsTimeSeries: (params: { period: string } | { from: string; to: string }) => {
    const q = new URLSearchParams()
    if ('from' in params && 'to' in params) {
      q.set('from', params.from)
      q.set('to', params.to)
    } else {
      q.set('period', params.period)
    }
    return request<AdminStatsTimeSeriesDTO>('GET', `/admin/stats/timeseries?${q.toString()}`)
  },
  adminStatsInsights: (
    params: ({ period: string } | { from: string; to: string }) & { tzOffsetMinutes?: number },
  ) => {
    const q = new URLSearchParams()
    if ('from' in params && 'to' in params) {
      q.set('from', params.from)
      q.set('to', params.to)
    } else {
      q.set('period', params.period)
    }
    if (params.tzOffsetMinutes != null) q.set('tz', String(params.tzOffsetMinutes))
    return request<AdminStatsInsightsDTO>('GET', `/admin/stats/insights?${q.toString()}`)
  },
  adminFortuneStats: () => request<AdminFortuneStatsDTO>('GET', '/admin/stats/fortune'),
  adminLoyaltyStats: () => request<AdminLoyaltyStatsDTO>('GET', '/admin/stats/loyalty'),
  adminPromoStats: () => request<AdminPromoStatsDTO>('GET', '/admin/stats/promos'),

  adminUsers: (params?: { scope?: string; page?: number; limit?: number }) => {
    const q = new URLSearchParams()
    q.set('scope', params?.scope ?? 'all')
    if (params?.page != null) q.set('page', String(params.page))
    if (params?.limit != null) q.set('limit', String(params.limit))
    const suffix = q.toString() ? `?${q.toString()}` : ''
    return request<AdminUsersListDTO>('GET', `/admin/users${suffix}`)
  },
  adminUserSearch: (query: string) =>
    request<{ items: AdminCustomerDTO[] }>('GET', `/admin/users/search?q=${encodeURIComponent(query)}`),
  adminUser: (id: number) =>
    request<AdminCustomerDTO>('GET', `/admin/users/${id}`),
  adminUserExtend: (id: number, days: number) =>
    request<AdminOkDTO>('POST', `/admin/users/${id}/extend`, { days }),
  adminUserDisable: (id: number) =>
    request<AdminOkDTO>('POST', `/admin/users/${id}/disable`),
  adminUserEnable: (id: number) =>
    request<AdminOkDTO>('POST', `/admin/users/${id}/enable`),
  adminUserDelete: (id: number) =>
    request<AdminOkDTO>('POST', `/admin/users/${id}/delete`),
  adminUserResetTraffic: (id: number) =>
    request<AdminOkDTO>('POST', `/admin/users/${id}/reset-traffic`),
  adminUserSetExpire: (id: number, expireAt: string) =>
    request<AdminOkDTO>('PATCH', `/admin/users/${id}/expire`, { expire_at: expireAt }),
  adminUserSetHwidLimit: (id: number, limit: number) =>
    request<AdminOkDTO>('PATCH', `/admin/users/${id}/hwid-limit`, { limit }),
  adminUserPayments: (id: number, params?: { page?: number; limit?: number }) => {
    const q = new URLSearchParams()
    if (params?.page != null) q.set('page', String(params.page))
    if (params?.limit != null) q.set('limit', String(params.limit))
    const suffix = q.toString() ? `?${q.toString()}` : ''
    return request<AdminPaymentsDTO>('GET', `/admin/users/${id}/payments${suffix}`)
  },
  adminUserReferrals: (id: number, params?: { page?: number; limit?: number }) => {
    const q = new URLSearchParams()
    if (params?.page != null) q.set('page', String(params.page))
    if (params?.limit != null) q.set('limit', String(params.limit))
    const suffix = q.toString() ? `?${q.toString()}` : ''
    return request<AdminReferralsDTO>('GET', `/admin/users/${id}/referrals${suffix}`)
  },
  adminUserPanel: (id: number) =>
    request<AdminUserPanelDTO>('GET', `/admin/users/${id}/panel`),
  adminUserSetSquads: (id: number, squadUuids: string[]) =>
    request<AdminOkDTO>('PATCH', `/admin/users/${id}/squads`, { squad_uuids: squadUuids }),
  adminUserSetTraffic: (id: number, limitBytes: number) =>
    request<AdminOkDTO>('PATCH', `/admin/users/${id}/traffic`, { limit_bytes: limitBytes }),
  adminUserSetStrategy: (id: number, strategy: string) =>
    request<AdminOkDTO>('PATCH', `/admin/users/${id}/strategy`, { strategy }),
  adminUserSetDescription: (id: number, description: string | null) =>
    request<AdminOkDTO>('PATCH', `/admin/users/${id}/description`, { description }),
  adminUserSetTariff: (id: number, tariffId: number) =>
    request<AdminOkDTO>('PATCH', `/admin/users/${id}/tariff`, { tariff_id: tariffId }),
  adminUserDevices: (id: number) =>
    request<{ items: AdminDeviceDTO[] }>('GET', `/admin/users/${id}/devices`),
  adminUserDeleteDevice: (id: number, hwid: string) =>
    request<AdminOkDTO>('DELETE', `/admin/users/${id}/devices/${encodeURIComponent(hwid)}`),
  adminUserExtraHwid: (id: number, delta: number) =>
    request<AdminOkDTO>('POST', `/admin/users/${id}/extra-hwid`, { delta }),

  adminSquads: () => request<{ items: { uuid: string; name: string }[] }>('GET', '/admin/squads'),

  adminPayments: (params?: { status?: string; q?: string; page?: number; limit?: number }) => {
    const q = new URLSearchParams()
    if (params?.status) q.set('status', params.status)
    if (params?.q) q.set('q', params.q)
    if (params?.page != null) q.set('page', String(params.page))
    if (params?.limit != null) q.set('limit', String(params.limit))
    const suffix = q.toString() ? `?${q.toString()}` : ''
    return request<AdminPaymentsListDTO>('GET', `/admin/payments${suffix}`)
  },
  adminPayment: (id: number) => request<AdminPaymentDetailDTO>('GET', `/admin/payments/${id}`),
  /** Скачивает CSV с учётом текущего фильтра и триггерит сохранение файла в браузере. */
  adminPaymentsExportCsv: async (params?: { status?: string; q?: string }) => {
    const q = new URLSearchParams()
    if (params?.status) q.set('status', params.status)
    if (params?.q) q.set('q', params.q)
    const suffix = q.toString() ? `?${q.toString()}` : ''

    const headers: Record<string, string> = {}
    const csrf = readCsrfCookie()
    if (csrf) headers['X-CSRF-Token'] = csrf
    const token = _authRef?.getAccessToken()
    if (token) headers['Authorization'] = `Bearer ${token}`

    const res = await fetch(`${BASE}/admin/payments/export${suffix}`, {
      method: 'GET',
      headers,
      credentials: 'include',
    })
    if (!res.ok) {
      const text = await res.text().catch(() => '')
      throw new ApiError(res.status, text)
    }
    const blob = await res.blob()
    const disposition = res.headers.get('Content-Disposition') ?? ''
    const match = /filename="?([^"]+)"?/.exec(disposition)
    const filename = match?.[1] ?? 'payments.csv'

    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = filename
    document.body.appendChild(a)
    a.click()
    a.remove()
    URL.revokeObjectURL(url)
  },

  adminPromos: (params?: { page?: number; limit?: number }) => {
    const q = new URLSearchParams()
    if (params?.page != null) q.set('page', String(params.page))
    if (params?.limit != null) q.set('limit', String(params.limit))
    const suffix = q.toString() ? `?${q.toString()}` : ''
    return request<AdminPromoListDTO>('GET', `/admin/promos${suffix}`)
  },
  adminPromoGet: (id: number) => request<AdminPromoGetDTO>('GET', `/admin/promos/${id}`),
  adminPromoRedemptions: (id: number, params?: { page?: number; limit?: number }) => {
    const q = new URLSearchParams()
    if (params?.page != null) q.set('page', String(params.page))
    if (params?.limit != null) q.set('limit', String(params.limit))
    const suffix = q.toString() ? `?${q.toString()}` : ''
    return request<AdminPromoRedemptionsListDTO>('GET', `/admin/promos/${id}/redemptions${suffix}`)
  },
  adminPromoCreate: (body: unknown) => request<AdminPromoCodeDTO>('POST', '/admin/promos', body),
  adminPromoUpdate: (id: number, fields: Record<string, unknown>) =>
    request<AdminPromoCodeDTO>('PATCH', `/admin/promos/${id}`, fields),
  adminPromoDelete: (id: number) => request<AdminOkDTO>('DELETE', `/admin/promos/${id}`),

  adminTariffs: () => request<AdminTariffDTO[]>('GET', '/admin/tariffs'),
  adminTariffGet: (id: number) => request<AdminTariffDTO>('GET', `/admin/tariffs/${id}`),
  adminTariffCreate: (body: unknown) => request<AdminTariffDTO>('POST', '/admin/tariffs', body),
  adminTariffUpdate: (id: number, fields: Record<string, unknown>) =>
    request<AdminTariffDTO>('PATCH', `/admin/tariffs/${id}`, fields),
  adminTariffDelete: (id: number) => request<AdminOkDTO>('DELETE', `/admin/tariffs/${id}`),

  // Состав сквадов тарифа: сколько людей уже на тарифе и как идёт применение.
  adminTariffSquads: (id: number) =>
    request<AdminTariffSquadsPreviewDTO>('GET', `/admin/tariffs/${id}/squads`),
  adminTariffSquadsApply: (id: number, body: { add: string[]; remove: string[] }) =>
    request<AdminTariffSquadsRunDTO>('POST', `/admin/tariffs/${id}/squads/apply`, body),

  // --- Партнёрская программа ---

  adminPartners: (params?: { status?: string; limit?: number; offset?: number }) => {
    const q = new URLSearchParams()
    if (params?.status) q.set('status', params.status)
    if (params?.limit != null) q.set('limit', String(params.limit))
    if (params?.offset != null) q.set('offset', String(params.offset))
    const suffix = q.toString() ? `?${q.toString()}` : ''
    return request<{ items: AdminPartnerDTO[]; total: number }>('GET', `/admin/partners${suffix}`)
  },

  adminPartnerPending: () => request<AdminPartnerPendingDTO>('GET', '/admin/partners/pending'),

  adminPartnerDetail: (id: number) =>
    request<AdminPartnerDetailDTO>('GET', `/admin/partners/${id}`),

  adminPartnerCustomers: (id: number, params?: { limit?: number; offset?: number }) =>
    request<AdminPage<AdminPartnerCustomerDTO>>('GET', `/admin/partners/${id}/customers${pageQuery(params)}`),

  adminPartnerOperations: (id: number, params?: { limit?: number; offset?: number }) =>
    request<AdminPage<AdminPartnerOperationDTO>>('GET', `/admin/partners/${id}/operations${pageQuery(params)}`),

  adminPartnerPayoutHistory: (id: number, params?: { limit?: number; offset?: number }) =>
    request<AdminPage<AdminPartnerPayoutDTO>>('GET', `/admin/partners/${id}/payouts${pageQuery(params)}`),

  adminPartnerApprove: (id: number, body: AdminPartnerTermsInput) =>
    request<AdminOkDTO>('POST', `/admin/partners/${id}/approve`, body),

  adminPartnerReject: (id: number, comment: string) =>
    request<AdminOkDTO>('POST', `/admin/partners/${id}/reject`, { comment }),

  adminPartnerSetStatus: (id: number, status: string, comment = '') =>
    request<AdminOkDTO>('POST', `/admin/partners/${id}/status`, { status, comment }),

  adminPartnerUpdateTerms: (id: number, body: AdminPartnerTermsInput) =>
    request<AdminOkDTO>('PATCH', `/admin/partners/${id}`, body),

  adminPartnerAdjust: (id: number, amount: number, comment: string) =>
    request<AdminOkDTO>('POST', `/admin/partners/${id}/adjust`, { amount, comment }),

  adminPartnerGrant: (body: {
    customer_id?: number
    telegram_id?: number
    first_percent?: number | null
    renewal_percent?: number | null
    comment?: string
  }) => request<AdminOkDTO>('POST', '/admin/partners/grant', body),

  adminPartnerPayouts: (params?: { status?: string; limit?: number; offset?: number }) => {
    const q = new URLSearchParams()
    if (params?.status) q.set('status', params.status)
    if (params?.limit != null) q.set('limit', String(params.limit))
    if (params?.offset != null) q.set('offset', String(params.offset))
    const suffix = q.toString() ? `?${q.toString()}` : ''
    return request<{ items: AdminPartnerPayoutDTO[]; total: number }>('GET', `/admin/partners/payouts${suffix}`)
  },

  adminPartnerPayoutAction: (
    id: number,
    action: 'approve' | 'paid' | 'reject',
    body: { external_ref?: string; comment?: string } = {},
  ) =>
    request<AdminOkDTO>('POST', `/admin/partners/payouts/${id}/${action}`, {
      external_ref: body.external_ref ?? '',
      comment: body.comment ?? '',
    }),

  adminLoyaltyTiers: () => request<AdminLoyaltyTierDTO[]>('GET', '/admin/loyalty/tiers'),
  adminLoyaltyCreateTier: (body: unknown) =>
    request<AdminLoyaltyTierDTO>('POST', '/admin/loyalty/tiers', body),
  adminLoyaltyUpdateTier: (id: number, body: Record<string, unknown>) =>
    request<AdminLoyaltyTierDTO>('PATCH', `/admin/loyalty/tiers/${id}`, body),
  adminLoyaltyDeleteTier: (id: number) => request<AdminOkDTO>('DELETE', `/admin/loyalty/tiers/${id}`),
  adminLoyaltyRecalc: () => request<AdminOkDTO>('POST', '/admin/loyalty/recalc'),

  adminBroadcastAudiences: () =>
    request<{ audiences: AdminBroadcastAudienceDTO[] }>('GET', '/admin/broadcast/audiences'),
  adminBroadcastTariffs: () =>
    request<{ tariffs: { id: number; name: string; slug: string }[] }>('GET', '/admin/broadcast/tariffs'),
  adminBroadcastPreview: (body: {
    audience: string
    tariff_id?: number | null
    text?: string
  }) => request<AdminBroadcastPreviewDTO>('POST', '/admin/broadcast/preview', body),
  adminBroadcastUploadMedia: async (file: File): Promise<AdminBroadcastMediaDTO> => {
    const form = new FormData()
    form.append('media', file)
    const headers: Record<string, string> = {}
    const csrf = readCsrfCookie()
    if (csrf) headers['X-CSRF-Token'] = csrf
    const token = _authRef?.getAccessToken()
    if (token) headers['Authorization'] = `Bearer ${token}`

    const res = await fetch(`${BASE}/admin/broadcast/upload-media`, {
      method: 'POST',
      headers,
      body: form,
      credentials: 'include',
    })
    if (!res.ok) {
      const text = await res.text().catch(() => '')
      throw new ApiError(res.status, text)
    }
    return res.json() as Promise<AdminBroadcastMediaDTO>
  },
  adminBroadcastSend: (body: {
    audience: string
    tariff_id?: number | null
    text: string
    buttons?: {
      buy?: boolean
      connect?: boolean
      promo?: boolean
      main_menu?: boolean
      links?: string[]
    }
    media?: {
      file_id: string
      kind: string
    } | null
  }) => request<AdminBroadcastSendDTO>('POST', '/admin/broadcast/send', body),

  adminInfraNodes: () => request<AdminInfraNodesDTO>('GET', '/admin/infra/nodes'),
  adminInfraCreateNode: (body: { provider_uuid: string; node_uuid: string; next_billing_at?: string }) =>
    request<AdminInfraNodesDTO>('POST', '/admin/infra/nodes', body),
  adminInfraPatchNode: (body: { uuid: string; next_billing_at: string }) =>
    request<AdminInfraNodesDTO>('PATCH', '/admin/infra/nodes', body),
  adminInfraDeleteNode: (uuid: string) => request<AdminInfraNodesDTO>('DELETE', `/admin/infra/nodes/${uuid}`),
  adminInfraProviders: () => request<AdminInfraProvidersDTO>('GET', '/admin/infra/providers'),
  adminInfraCreateProvider: (body: { name: string; favicon_link?: string; login_url?: string }) =>
    request<AdminInfraProvidersDTO>('POST', '/admin/infra/providers', body),
  adminInfraPatchProvider: (body: { uuid: string; name?: string; favicon_link?: string; login_url?: string }) =>
    request<AdminInfraProvidersDTO>('PATCH', '/admin/infra/providers', body),
  adminInfraDeleteProvider: (uuid: string) =>
    request<AdminInfraProvidersDTO>('DELETE', `/admin/infra/providers/${uuid}`),
  adminInfraHistory: (start?: number, size?: number) => {
    const q = new URLSearchParams()
    if (start != null) q.set('start', String(start))
    if (size != null) q.set('size', String(size))
    const suffix = q.toString() ? `?${q.toString()}` : ''
    return request<AdminInfraHistoryDTO>('GET', `/admin/infra/history${suffix}`)
  },
  adminInfraCreateHistory: (body: { provider_uuid: string; amount: number; billed_at: string }) =>
    request<AdminInfraHistoryDTO>('POST', '/admin/infra/history', body),
  adminInfraDeleteHistory: (uuid: string) =>
    request<AdminInfraHistoryDTO>('DELETE', `/admin/infra/history/${uuid}`),
  adminInfraSettings: () => request<AdminInfraSettingsDTO>('GET', '/admin/infra/settings'),
  adminInfraUpdateSettings: (body: { days: number; enabled: boolean }) =>
    request<AdminOkDTO>('PATCH', '/admin/infra/settings', body),

  adminStatusTargets: () =>
    request<{
      targets: {
        id: string
        name: string
        country: string
        address: string
        enabled: boolean
        interval_min: number
        world_probes: number
        russia_probes: number
        note?: string
        whitelist?: boolean
      }[]
    }>('GET', '/admin/status/targets'),

  saveAdminStatusTargets: (
    targets: {
      id: string
      name: string
      country: string
      address: string
      enabled: boolean
      interval_min: number
      world_probes: number
      russia_probes: number
      note?: string
      whitelist?: boolean
    }[],
  ) =>
    request<{ targets: { id: string }[] }>('PUT', '/admin/status/targets', { targets }),

  adminStatusProbe: () => request<{ ok: boolean }>('POST', '/admin/status/probe'),

  adminBotSettings: () => request<AdminBotSettingsDTO>('GET', '/admin/settings'),
  adminBotSettingsPatch: (body: { settings: Record<string, string> }) =>
    request<AdminBotSettingsPatchDTO>('PATCH', '/admin/settings', body),

  adminSync: () => request<AdminOkDTO>('POST', '/admin/sync'),

  // Link / Merge
  linkTelegramStart: () =>
    request<{ nonce: string }>('POST', '/link/telegram/start'),

  linkTelegramConfirm: (payload: {
    source: 'widget' | 'miniapp'
    nonce: string
    id?: number
    first_name?: string
    last_name?: string
    username?: string
    photo_url?: string
    auth_date?: number
    hash?: string
    init_data?: string
  }) =>
    request<{ telegram_id: number; telegram_username?: string; has_merge_candidate: boolean; customer_tg_id?: number }>(
      'POST',
      '/link/telegram/confirm',
      payload,
    ),

  mergePreview: () =>
    request<MergePreviewResponse>('POST', '/link/merge/preview'),

  mergeConfirm: (idempotencyKey: string, opts?: { force?: boolean; keep_subscription?: 'web' | 'tg' }) =>
    request<MergeConfirmResponse>(
      'POST',
      '/link/merge/confirm',
      { force: opts?.force ?? false, keep_subscription: opts?.keep_subscription },
      { 'Idempotency-Key': idempotencyKey },
    ),

  /**
   * Привязка Telegram (OIDC): браузерный переход на oauth.telegram.org.
   * Нельзя делать location.href на /me/telegram/link/start — эндпоинт требует Bearer,
   * при полной навигации заголовок не отправляется → 401.
   * Редирект на другой origin через fetch+redirect:manual даёт opaqueredirect без Location —
   * поэтому запрашиваем JSON с redirect_url.
   */
  startTelegramOIDCLink: async (): Promise<void> => {
    const run = (token: string) => {
      const csrf = readCsrfCookie()
      const headers: Record<string, string> = {
        Authorization: `Bearer ${token}`,
        Accept: 'application/json',
      }
      if (csrf) headers['X-CSRF-Token'] = csrf
      return fetch(`${BASE}/me/telegram/link/start`, {
        method: 'GET',
        headers,
        credentials: 'include',
      })
    }
    let token = _authRef?.getAccessToken()
    if (!token) throw new ApiError(401, 'not signed in')
    let res = await run(token)
    if (res.status === 401) {
      const newTok = await doRefresh()
      if (!newTok) {
        _authRef?.logout()
        throw new ApiError(401, 'Session expired')
      }
      res = await run(newTok)
    }
    if (!res.ok) {
      const text = await res.text().catch(() => '')
      throw new ApiError(res.status, text || 'telegram link start failed')
    }
    const data = (await res.json().catch(() => null)) as { redirect_url?: string } | null
    const url = data?.redirect_url?.trim()
    if (url) {
      window.location.assign(url)
      return
    }
    throw new ApiError(res.status, 'telegram link start failed')
  },

  /**
   * Привязка Google к текущему аккаунту: JSON с redirect_url (аналогично Telegram OIDC link).
   */
  startGoogleOAuthLink: async (): Promise<void> => {
    const run = (token: string) => {
      const csrf = readCsrfCookie()
      const headers: Record<string, string> = {
        Authorization: `Bearer ${token}`,
        Accept: 'application/json',
      }
      if (csrf) headers['X-CSRF-Token'] = csrf
      return fetch(`${BASE}/me/google/link/start`, {
        method: 'GET',
        headers,
        credentials: 'include',
      })
    }
    let token = _authRef?.getAccessToken()
    if (!token) throw new ApiError(401, 'not signed in')
    let res = await run(token)
    if (res.status === 401) {
      const newTok = await doRefresh()
      if (!newTok) {
        _authRef?.logout()
        throw new ApiError(401, 'Session expired')
      }
      res = await run(newTok)
    }
    if (!res.ok) {
      const text = await res.text().catch(() => '')
      throw new ApiError(res.status, text || 'google link start failed')
    }
    const data = (await res.json().catch(() => null)) as { redirect_url?: string } | null
    const url = data?.redirect_url?.trim()
    if (url) {
      window.location.assign(url)
      return
    }
    throw new ApiError(res.status, 'google link start failed')
  },
  startYandexOAuthLink: async (): Promise<void> => {
    const run = (token: string) => {
      const csrf = readCsrfCookie()
      const headers: Record<string, string> = { Authorization: `Bearer ${token}`, Accept: 'application/json' }
      if (csrf) headers['X-CSRF-Token'] = csrf
      return fetch(`${BASE}/me/yandex/link/start`, { method: 'GET', headers, credentials: 'include' })
    }
    let token = _authRef?.getAccessToken()
    if (!token) throw new ApiError(401, 'not signed in')
    let res = await run(token)
    if (res.status === 401) {
      const newTok = await doRefresh()
      if (!newTok) {
        _authRef?.logout()
        throw new ApiError(401, 'Session expired')
      }
      res = await run(newTok)
    }
    if (!res.ok) throw new ApiError(res.status, (await res.text().catch(() => '')) || 'yandex link start failed')
    const data = (await res.json().catch(() => null)) as { redirect_url?: string } | null
    const u = data?.redirect_url?.trim()
    if (!u) throw new ApiError(res.status, 'yandex link start failed')
    window.location.assign(u)
  },
  startVKOAuthLink: async (): Promise<void> => {
    const run = (token: string) => {
      const csrf = readCsrfCookie()
      const headers: Record<string, string> = { Authorization: `Bearer ${token}`, Accept: 'application/json' }
      if (csrf) headers['X-CSRF-Token'] = csrf
      return fetch(`${BASE}/me/vk/link/start`, { method: 'GET', headers, credentials: 'include' })
    }
    let token = _authRef?.getAccessToken()
    if (!token) throw new ApiError(401, 'not signed in')
    let res = await run(token)
    if (res.status === 401) {
      const newTok = await doRefresh()
      if (!newTok) {
        _authRef?.logout()
        throw new ApiError(401, 'Session expired')
      }
      res = await run(newTok)
    }
    if (!res.ok) throw new ApiError(res.status, (await res.text().catch(() => '')) || 'vk link start failed')
    const data = (await res.json().catch(() => null)) as { redirect_url?: string } | null
    const u = data?.redirect_url?.trim()
    if (!u) throw new ApiError(res.status, 'vk link start failed')
    window.location.assign(u)
  },
}
