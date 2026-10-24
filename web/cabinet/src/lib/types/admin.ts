/**
 * DTO для admin API (`/cabinet/api/admin/*`).
 * Синхронизированы с Go handlers в internal/cabinet/http/handlers/admin_*.go
 */

export interface AdminBootstrapDTO {
  sales_mode: string
  loyalty_enabled: boolean
  /** false при PARTNER_PROGRAM_ENABLED=false — раздел «Партнёры» скрыт. */
  partner_enabled?: boolean
  fortune_enabled: boolean
  /** Версия сборки: «5.3.0» для релиза, «dev-2fdc211» для main. */
  version?: string
  commit?: string
}

export interface AdminStatsDTO {
  captured_at: string
  total_customers: number
  active_subscriptions: number
  new_today: number
  new_week: number
  new_month: number
  new_prev_month: number
  new_half_year: number
  new_year: number
  trial_active: number
  paid_active: number
  inactive: number
  inactive_paid: number
  inactive_unpaid: number
  sales_sub_today: number
  sales_sub_week: number
  sales_sub_month: number
  sales_sub_prev_month: number
  sales_sub_half_year: number
  sales_sub_year: number
  revenue_month_rub: number
  revenue_today_rub: number
  revenue_week_rub: number
  revenue_half_year_rub: number
  revenue_year_rub: number
  revenue_all_time_rub: number
  revenue_subs_month_rub: number
  transactions_today: number
  transactions_week: number
  transactions_month: number
  transactions_half_year: number
  transactions_year: number
  unique_payers_day: number
  unique_payers_week: number
  unique_payers_month: number
  unique_payers_half_year: number
  unique_payers_year: number
  payment_rub_by_invoice: Record<string, number>
  distinct_referrers: number
  active_referrers: number
  ref_bonus_days_all: number
  ref_bonus_days_today: number
  ref_bonus_days_week: number
  ref_bonus_days_month: number
  ref_bonus_days_half_year: number
  ref_bonus_days_year: number
  top_referrers: {
    referrer_id: number
    customer_id: number
    telegram_username?: string | null
    nickname?: string | null
    referees: number
    paid_referees: number
    revenue_rub: number
    bonus_days: number
  }[]
  tariff_breakdown: {
    tariff_id: number
    display_name: string
    sales_today: number
    sales_week: number
    sales_month: number
    sales_half_year: number
    sales_year: number
    subs_revenue_month: number
    revenue_today: number
    revenue_week: number
    revenue_half_year: number
    revenue_year: number
    revenue_all: number
    active_paid_users: number
  }[]
}

export interface AdminBandwidthDTO {
  /** Панель отдаёт уже отформатированные строки: «287.01 GiB», «-162.73 GiB». */
  current: string
  previous: string
  difference: string
}

export interface AdminOverviewDTO {
  captured_at: string
  shop: {
    total_customers: number
    active_subscriptions: number
    revenue_today_rub: number
    revenue_month_rub: number
    sales_today: number
    payers_today: number
  }
  attention: {
    partner_applications: number
    partner_payouts: number
    open_invoices: number
    billing_overdue: number
    billing_due_urgent: number
    billing_due_soon: number
    /** Дни, роздан­ные колесом сверх собранных за месяц. 0 — выключено или в плюсе. */
    fortune_net_loss_days: number
  }
  panel: {
    available: boolean
    /** not_configured | unreachable */
    reason?: string
    traffic: {
      today: AdminBandwidthDTO
      last_seven_days: AdminBandwidthDTO
      last_thirty_days: AdminBandwidthDTO
      calendar_month: AdminBandwidthDTO
      current_year: AdminBandwidthDTO
    }
    online: {
      now: number
      today: number
      week: number
      never_online: number
    }
    system: {
      nodes_online: number
      total_bytes_lifetime: string
      memory_used: number
      memory_total: number
      cpu_cores: number
      uptime_seconds: number
    }
    panel_users: {
      total: number
      status_counts: Record<string, number>
    }
  }
}

export interface AdminStatsFunnelDTO {
  registered: number
  invoiced: number
  paid: number
  invoices_created: number
  invoices_paid: number
}

export interface AdminStatsHeatCellDTO {
  /** 1 — понедельник, 7 — воскресенье */
  weekday: number
  hour: number
  revenue_rub: number
  sales: number
}

export interface AdminStatsLifetimeDTO {
  paying_customers: number
  avg_lifetime_days: number
  avg_paid_months: number
  avg_purchases: number
}

export interface AdminStatsRenewalsDTO {
  first_count: number
  first_revenue: number
  renewal_count: number
  renewal_revenue: number
}

export interface AdminStatsGatewayDTO {
  invoice_type: string
  revenue_rub: number
  payments: number
}

export interface AdminStatsWindowDTO {
  revenue_rub: number
  sales: number
  new_users: number
  transactions: number
  unique_payers: number
}

export interface AdminPartnerTopDTO {
  partner_id: number
  customer_id: number
  telegram_id: number
  telegram_username?: string | null
  nickname?: string | null
  customers: number
  paying_customers: number
  earned: number
}

export interface AdminPartnerProgramDTO {
  partners_total: number
  partners_active: number
  partners_pending: number
  partners_suspended: number
  customers: number
  paying_customers: number
  active_customers: number
  earned_total: number
  earned_period: number
  earned_first: number
  earned_renewal: number
  hold_balance: number
  available_balance: number
  reserved_balance: number
  paid_total: number
  open_payouts: number
  open_payouts_amount: number
  top: AdminPartnerTopDTO[]
}

export interface AdminStatsInsightsDTO {
  captured_at: string
  period: string
  from: string
  to: string
  tz_offset_minutes: number
  funnel: AdminStatsFunnelDTO
  heatmap: AdminStatsHeatCellDTO[]
  lifetime: AdminStatsLifetimeDTO
  renewals: AdminStatsRenewalsDTO
  gateways: AdminStatsGatewayDTO[]
  current: AdminStatsWindowDTO
  previous: AdminStatsWindowDTO
  partners?: AdminPartnerProgramDTO | null
}

export interface AdminStatsTimeSeriesPointDTO {
  date: string
  revenue_rub: number
  sales: number
  new_users: number
  transactions: number
}

export interface AdminTariffTimeSeriesPointDTO {
  date: string
  sales: number
  revenue_rub: number
}

export interface AdminTariffTimeSeriesDTO {
  tariff_id: number
  display_name: string
  points: AdminTariffTimeSeriesPointDTO[]
}

export interface AdminStatsTimeSeriesDTO {
  captured_at: string
  period: string
  granularity: 'day' | 'week' | 'month'
  from: string
  to: string
  points: AdminStatsTimeSeriesPointDTO[]
  tariff_series: AdminTariffTimeSeriesDTO[]
}

export interface AdminFortunePeriodDTO {
  distinct_users: number
  total_spins: number
  free_spins: number
  paid_spins: number
  paid_cost_days_sum: number
  won_subs_days_sum: number
  won_loyalty_xp_sum: number
  won_discount_pct_sum: number
  by_reward: Record<string, number>
}

export interface AdminFortuneStatsDTO {
  captured_at: string
  month: AdminFortunePeriodDTO
  today: AdminFortunePeriodDTO
  all_time: AdminFortunePeriodDTO
}

export interface AdminLoyaltyTierStatDTO {
  sort_order: number
  xp_min: number
  discount_percent: number
  display_name?: string | null
  user_count: number
}

export interface AdminLoyaltyStatsDTO {
  captured_at: string
  enabled: boolean
  tiers: AdminLoyaltyTierStatDTO[]
}

export interface AdminPromoStatsTopDTO {
  id: number
  code: string
  active: boolean
  uses_count: number
  redemptions: number
  /** subscription_days | trial | extra_hwid | discount */
  type: string
  subscription_days?: number | null
  trial_days?: number | null
  extra_hwid_delta?: number | null
  discount_percent?: number | null
}

export interface AdminPromoStatsDTO {
  captured_at: string
  total: number
  active: number
  inactive: number
  total_redemptions: number
  redemptions_today: number
  top_by_redemptions: AdminPromoStatsTopDTO[]
}

export interface AdminCustomerDTO {
  id: number
  /** Строка, а не число: синтетические ID web-юзеров превышают Number.MAX_SAFE_INTEGER. */
  telegram_id: string
  telegram_username?: string | null
  language: string
  expire_at?: string | null
  created_at: string
  subscription_link?: string | null
  extra_hwid: number
  extra_hwid_expires_at?: string | null
  current_tariff_id?: number | null
  subscription_period_start?: string | null
  subscription_period_months?: number | null
  loyalty_xp: number
  loyalty_level?: number | null
  loyalty_discount_percent?: number | null
  is_web_only: boolean
  status: 'active' | 'expired' | 'trial' | 'disabled'
  rw_status?: string | null
  /** Логин web-клиента в панели ("<id>_<local-part email>"). У Telegram-клиентов отсутствует. */
  panel_login?: string | null
  /**
   * Подписанная ссылка на аватарку из Telegram. Приходит только в карточке
   * одного пользователя (GET /admin/users/{id}), в списке её нет. Картинки
   * может не быть — тогда сам запрос отвечает 404, и UI рисует инициалы.
   */
  avatar_url?: string | null
}

export interface AdminUsersListDTO {
  items: AdminCustomerDTO[]
  total: number
  page: number
  limit: number
}

export interface AdminPurchaseDTO {
  id: number
  amount: number
  currency: string
  paid_at?: string | null
  month: number
  invoice_type: string
  purchase_kind: string
  tariff_id?: number | null
  promo_code_id?: number | null
  discount_percent?: number | null
}

export interface AdminPaymentsDTO {
  items: AdminPurchaseDTO[]
  total: number
  rub_count: number
  rub_sum: number
  stars_count: number
  stars_sum: number
  rub_per_star: number
  stars_rub_equiv: number
}

/** Раздел «Платежи» в админке — все покупки (все статусы), см. GET /admin/payments. */
export interface AdminPaymentListItemDTO {
  id: number
  customer_id: number
  telegram_id: string
  telegram_username?: string | null
  panel_login?: string | null
  amount: number
  currency: string
  month: number
  extra_hwid: number
  invoice_type: string
  purchase_kind: string
  status: 'new' | 'pending' | 'paid' | 'cancel'
  created_at: string
  paid_at?: string | null
  tariff_id?: number | null
  tariff_name?: string | null
  tariff_location_name?: string | null
  promo_code_id?: number | null
  promo_code?: string | null
}

export interface AdminPaymentsListDTO {
  items: AdminPaymentListItemDTO[]
  total: number
  page: number
  limit: number
}

export interface AdminPaymentDetailDTO extends AdminPaymentListItemDTO {
  discount_percent?: number | null
  is_early_downgrade: boolean
  expire_at?: string | null
  crypto_invoice_id?: number | null
  crypto_invoice_url?: string | null
  yookasa_id?: string | null
  yookasa_url?: string | null
  platega_id?: string | null
  platega_url?: string | null
  heleket_id?: string | null
  heleket_url?: string | null
  provider_txn_id?: string | null
  idempotency_key?: string | null
  checkout_provider?: string | null
}

export interface AdminPromoCodeDTO {
  id: number
  code: string
  type: string
  subscription_days?: number | null
  trial_days?: number | null
  extra_hwid_delta?: number | null
  discount_percent?: number | null
  discount_ttl_hours?: number | null
  max_uses?: number | null
  uses_count: number
  valid_until?: string | null
  active: boolean
  first_purchase_only: boolean
  require_customer_in_db: boolean
  allow_trial_without_payment: boolean
  created_at: string
  discount_max_subscription_payments_per_customer: number
  tariff_id?: number | null
}

export interface AdminPromoListDTO {
  items: AdminPromoCodeDTO[]
  total: number
  page: number
  limit: number
}

export interface AdminPromoGetDTO {
  promo: AdminPromoCodeDTO
  redemptions: number
  redemptions_today: number
}

export interface AdminPromoRedemptionDTO {
  used_at: string
  customer_id: number
  telegram_username?: string | null
  nickname?: string | null
}

export interface AdminPromoRedemptionsListDTO {
  items: AdminPromoRedemptionDTO[]
  total: number
  page: number
  limit: number
}

export interface AdminTariffPriceDTO {
  tariff_id: number
  months: number
  amount_rub: number
  amount_stars?: number | null
}

export interface AdminTariffLocationDTO {
  id: number
  name: string
  sort_order: number
  sold_out: boolean
  country_code?: string | null
}

export interface AdminTariffDTO {
  id: number
  slug: string
  name?: string | null
  sort_order: number
  is_active: boolean
  device_limit: number
  traffic_limit_bytes: number
  traffic_limit_reset_strategy: string
  active_internal_squad_uuids: string
  external_squad_uuid?: string | null
  remnawave_tag?: string | null
  tier_level?: number | null
  description?: string | null
  description_detail?: string | null
  prices: AdminTariffPriceDTO[]
  locations?: AdminTariffLocationDTO[]
}

/** Ход применения состава сквадов тарифа к действующим подписчикам. */
export interface AdminTariffSquadsRunDTO {
  tariff_id: number
  add: string[]
  remove: string[]
  status: 'running' | 'done' | 'failed'
  total: number
  processed: number
  changed: number
  already_ok: number
  not_found: number
  failed: number
  error?: string
  failures?: string[]
  started_at: string
  finished_at?: string
}

export interface AdminTariffSquadsPreviewDTO {
  /** Фактический состав тарифа: пустой список в БД разворачивается во все сквады панели. */
  tariff_squads: string[]
  /** Сколько клиентов с активной подпиской сейчас на этом тарифе. */
  active_count: number
  run: AdminTariffSquadsRunDTO | null
}

export interface AdminBroadcastPreviewDTO {
  recipient_count: number
  status?: string
}

/** Чем вложение уйдёт в Telegram. Решает сервер по тому, во что его превратил Bot API. */
export type AdminBroadcastMediaKind = 'photo' | 'video' | 'document'

export interface AdminBroadcastMediaDTO {
  file_id: string
  kind: AdminBroadcastMediaKind
}

export interface AdminBroadcastSendDTO {
  status: string
  recipient_count: number
}

export interface AdminBroadcastAudienceDTO {
  audience: string
  label: string
  count: number
}

export interface AdminOkDTO {
  ok?: boolean
  status?: string
}

export interface AdminSquadDTO {
  uuid: string
  name: string
}

export interface AdminRWPanelDTO {
  /** Числовой id профиля в панели Remnawave. До 3.0.0 здесь был `uuid: string`. */
  id: number
  username: string
  status: string
  subscription_url: string
  expire_at?: string | null
  traffic_used_bytes: number
  traffic_limit_bytes: number
  traffic_limit_strategy: string
  hwid_device_limit?: number | null
  description?: string | null
  tag?: string | null
  last_traffic_reset_at?: string | null
  online_at?: string | null
  active_squads?: AdminSquadDTO[] | null
}

export interface AdminTariffBriefDTO {
  id: number
  slug: string
  name: string
}

export interface AdminUserPanelDTO {
  customer: AdminCustomerDTO
  has_rw_user: boolean
  rw?: AdminRWPanelDTO | null
  available_squads: AdminSquadDTO[]
  traffic_presets_gb: number[]
  strategies: string[]
  tariffs?: AdminTariffBriefDTO[]
}

export interface AdminDeviceDTO {
  hwid: string
  platform?: string | null
  os_version?: string | null
  device_model?: string | null
  created_at: string
}

export interface AdminReferralStatsDTO {
  total: number
  paid: number
  active: number
  conversion: number
  earned_total: number
  earned_last_month: number
}

export interface AdminRefereeDTO {
  /** Строка, а не число: синтетические ID web-юзеров превышают Number.MAX_SAFE_INTEGER. */
  telegram_id: string
  telegram_username?: string | null
  active: boolean
  email?: string | null
}

export interface AdminReferralsDTO {
  stats: AdminReferralStatsDTO
  referees: AdminRefereeDTO[]
}

export interface AdminLoyaltyTierDTO {
  id: number
  sort_order: number
  xp_min: number
  discount_percent: number
  display_name?: string | null
}

/** Remnawave infra-billing API (camelCase JSON). */
export interface AdminInfraProviderShortDTO {
  uuid: string
  name: string
  faviconLink: string
  loginUrl: string
}

export interface AdminInfraNodeShortDTO {
  uuid: string
  name: string
  countryCode: string
}

export interface AdminInfraBillingNodeDTO {
  uuid: string
  nodeUuid: string
  providerUuid: string
  nextBillingAt: string
  createdAt: string
  updatedAt: string
  provider: AdminInfraProviderShortDTO
  node: AdminInfraNodeShortDTO
}

export interface AdminInfraNodesStatsDTO {
  upcomingNodesCount: number
  currentMonthPayments: number
  totalSpent: number
}

export interface AdminInfraAvailableNodeDTO {
  uuid: string
  name: string
  countryCode: string
}

export interface AdminInfraNodesDTO {
  totalBillingNodes: number
  totalAvailableBillingNodes: number
  billingNodes: AdminInfraBillingNodeDTO[]
  availableBillingNodes: AdminInfraAvailableNodeDTO[]
  stats: AdminInfraNodesStatsDTO
}

export interface AdminInfraProviderHistoryAggDTO {
  totalAmount: number
  totalBills: number
}

export interface AdminInfraProviderNodeDTO {
  nodeUuid: string
  name: string
  countryCode: string
}

export interface AdminInfraProviderDTO {
  uuid: string
  name: string
  faviconLink: string
  loginUrl: string
  createdAt: string
  updatedAt: string
  billingHistory: AdminInfraProviderHistoryAggDTO
  billingNodes: AdminInfraProviderNodeDTO[]
}

export interface AdminInfraProvidersDTO {
  total: number
  providers: AdminInfraProviderDTO[]
}

export interface AdminInfraHistoryRecordDTO {
  uuid: string
  providerUuid: string
  amount: number
  billedAt: string
  provider: AdminInfraProviderShortDTO
}

export interface AdminInfraHistoryDTO {
  records: AdminInfraHistoryRecordDTO[]
  total: number
}

export interface AdminInfraSettingsDTO {
  notify_before_1: boolean
  notify_before_3: boolean
  notify_before_7: boolean
  notify_before_14: boolean
}

export type AdminSettingFieldType =
  | 'bool'
  | 'int'
  | 'float'
  | 'text'
  | 'url'
  | 'enum'
  | 'csv_int'
  | 'csv'

export interface AdminSettingFieldDTO {
  key: string
  type: AdminSettingFieldType
  value: string
  source: 'db' | 'env' | 'default'
  instant: boolean
  enum_values?: string[]
  min_int?: number
  max_int?: number
}

export interface AdminSettingGroupDTO {
  id: string
  fields: AdminSettingFieldDTO[]
}

export interface AdminBotSettingsDTO {
  groups: AdminSettingGroupDTO[]
}

export interface AdminBotSettingsPatchDTO {
  ok: boolean
  changed: string[]
}

/* --- Партнёрская программа -------------------------------------------------
 *
 * Партнёр — это клиент со статусом и денежным контуром: балансом, холдом и
 * заявками на вывод. Заявки на партнёрство отдельной сущностью не хранятся,
 * это тот же партнёр в статусе pending.
 */

export type AdminPartnerStatus = 'pending' | 'active' | 'suspended' | 'rejected'

export interface AdminPartnerDTO {
  id: number
  status: AdminPartnerStatus
  /** Маскированная подпись клиента: @username, e***l@mail.ru либо часть id. */
  label: string

  /** null — действуют глобальные проценты из настроек бота. */
  first_percent?: number | null
  renewal_percent?: number | null
  effective_first_percent: number
  effective_renewal_percent: number
  links_limit?: number | null
  /** Действующий лимит потоков: индивидуальный либо подставленный глобальный. */
  effective_links_limit: number

  balance: number
  hold_balance: number
  reserved_balance: number
  total_earned: number
  total_paid: number

  customers: number
  paying_customers: number
  open_payouts: number

  /** Он же как клиент магазина — по этим числам видно, живой ли аккаунт. */
  customer_since: string
  customer_paid_count: number
  customer_paid_sum: number

  app_about?: string
  app_channels?: string
  app_expected?: string
  app_submitted_at?: string
  admin_note?: string

  payout_method?: string
  payout_details?: string
  created_at: string
  approved_at?: string
}

export interface AdminPartnerOperationDTO {
  at: string
  /** earning — начисление либо ручная правка; payout — выплата. */
  kind: 'earning' | 'payout'
  detail?: string
  amount: number
  status: string
  ref?: string
  note?: string
}

export interface AdminPartnerLinkDTO {
  id: number
  code: string
  name: string
  is_default: boolean
  archived: boolean
  bot_link?: string
  customers: number
  paying: number
  earned: number
}

export interface AdminPartnerCustomerDTO {
  label: string
  active: boolean
  has_paid: boolean
  earned: number
  link_name?: string
  attached_at: string
}

/**
 * Карточка партнёра.
 *
 * Журналы (клиенты, операции, выплаты) сюда не приходят: у активного партнёра
 * там сотни строк. Вкладки грузят их постранично, а `counts` нужен только для
 * счётчиков на самих вкладках.
 */
export interface AdminPartnerDetailDTO {
  partner: AdminPartnerDTO
  links: AdminPartnerLinkDTO[]
  counts: {
    customers: number
    operations: number
    payouts: number
  }
}

export interface AdminPage<T> {
  items: T[]
  total: number
}

export interface AdminPartnerPayoutDTO {
  id: number
  partner_id: number
  partner_label: string
  amount: number
  status: 'pending' | 'approved' | 'paid' | 'rejected'
  method?: string
  /** Реквизиты на момент подачи — их и копирует админ для перевода. */
  details_snapshot?: string
  admin_comment?: string
  external_ref?: string
  requested_at: string
  processed_at?: string
  partner_total_earned: number
  partner_total_paid: number
  /** Какая это по счёту заявка партнёра. */
  payout_index: number
}

export interface AdminPartnerPendingDTO {
  applications: number
  payouts: number
  total: number
  /** Начисления, пропущенные из-за незаданного RUB_PER_STAR. */
  skipped_stars_earnings: number
}

/** null в проценте означает «вернуть к глобальному значению». */
export interface AdminPartnerTermsInput {
  first_percent?: number | null
  renewal_percent?: number | null
  links_limit?: number | null
  comment?: string
}
