import fs from 'node:fs'
import path from 'node:path'
import type { ServerResponse } from 'node:http'
import type { Plugin } from 'vite'

/**
 * Локальный API кабинета без бота. Только `vite dev`: отдаёт те же JSON,
 * что ждёт интерфейс, чтобы можно было потыкать экраны как после релиза.
 */
export function cabinetDevApi(): Plugin {
  let broadcastOptOut = false
  return {
    name: 'cabinet-dev-api',
    apply: 'serve',
    configureServer(server) {
      server.middlewares.use((req, res, next) => {
        const raw = req.url || ''
        if (!raw.startsWith('/cabinet/api')) {
          next()
          return
        }
        const url = new URL(raw, 'http://127.0.0.1')
        const pathname = url.pathname
        res.setHeader('Set-Cookie', 'csrf_token=devmock; Path=/; SameSite=Lax')

        if (pathname === '/cabinet/api/content/app-config') {
          return sendFile(res, path.resolve(server.config.root, '../../translations/cabinet/app-config.json'))
        }
        if (pathname === '/cabinet/api/content/faq') {
          return sendFile(res, path.resolve(server.config.root, 'public/translations/cabinet/FAQ.json'))
        }
        if (pathname.startsWith('/cabinet/api/content/i18n/')) {
          res.statusCode = 404
          res.end()
          return
        }

        const body = route(req.method || 'GET', pathname, url, () => broadcastOptOut, (v) => {
          broadcastOptOut = v
        })
        if (body == null) {
          sendJSON(res, { ok: true, items: [], enabled: false })
          return
        }
        sendJSON(res, body)
      })
    },
  }
}

function sendFile(res: ServerResponse, file: string) {
  res.setHeader('Content-Type', 'application/json; charset=utf-8')
  res.end(fs.readFileSync(file, 'utf8'))
}

function sendJSON(res: ServerResponse, body: unknown) {
  res.statusCode = 200
  res.setHeader('Content-Type', 'application/json; charset=utf-8')
  res.end(JSON.stringify(body))
}

const GB = 1024 * 1024 * 1024
const day = 24 * 60 * 60 * 1000

function iso(offsetMs: number) {
  return new Date(Date.now() + offsetMs).toISOString()
}

const tariffs = {
  sales_mode: 'tariffs',
  currency: 'RUB',
  show_savings: true,
  price_display: 'monthly',
  savings_badge: 'corner',
  tariffs: [
    {
      id: 1,
      slug: 'start',
      name: 'Старт',
      description: 'Общие серверы, безлимитный трафик.',
      device_limit: 3,
      traffic_limit_bytes: 0,
      prices: [
        { months: 1, amount_rub: 249, monthly_base_rub: 249 },
        { months: 3, amount_rub: 669, monthly_base_rub: 249 },
        { months: 6, amount_rub: 1194, monthly_base_rub: 249 },
        { months: 12, amount_rub: 2148, monthly_base_rub: 249 },
      ],
    },
    {
      id: 2,
      slug: 'own',
      name: 'Свой сервер',
      description: 'Отдельный сервер в выбранной стране.',
      description_detail: 'При оплате выберите страну. Если локация закончилась, её нельзя выбрать.',
      device_limit: 5,
      traffic_limit_bytes: 0,
      locations: [
        { id: 1, name: 'Германия', sold_out: false, country_code: 'de' },
        { id: 2, name: 'Нидерланды', sold_out: false, country_code: 'nl' },
        { id: 3, name: 'Финляндия', sold_out: true, country_code: 'fi' },
        { id: 4, name: 'Эстония', sold_out: false, country_code: 'ee' },
      ],
      prices: [
        { months: 1, amount_rub: 1990, monthly_base_rub: 1990 },
        { months: 3, amount_rub: 5370, monthly_base_rub: 1990 },
        { months: 6, amount_rub: 9540, monthly_base_rub: 1990 },
        { months: 12, amount_rub: 17880, monthly_base_rub: 1990 },
      ],
    },
    {
      id: 3,
      slug: 'premium',
      name: 'Премиум',
      description: 'Больше устройств и приоритет.',
      device_limit: 8,
      traffic_limit_bytes: 0,
      prices: [
        { months: 1, amount_rub: 399, monthly_base_rub: 399 },
        { months: 3, amount_rub: 1077, monthly_base_rub: 399 },
        { months: 6, amount_rub: 1914, monthly_base_rub: 399 },
        { months: 12, amount_rub: 3588, monthly_base_rub: 399 },
      ],
    },
  ],
}

function route(
  method: string,
  pathname: string,
  url: URL,
  optOut: () => boolean,
  setOptOut: (v: boolean) => void,
): unknown | null {
  if (pathname === '/cabinet/api/auth/refresh' || pathname === '/cabinet/api/auth/logout') {
    return { access_token: 'dev-mock', access_exp: Math.floor(Date.now() / 1000) + 3600, csrf_token: 'devmock' }
  }
  if (pathname === '/cabinet/api/auth/bootstrap') return bootstrap()
  if (pathname === '/cabinet/api/me' && method === 'GET') return me(optOut())
  if (pathname === '/cabinet/api/me/broadcast' && method === 'PUT') {
    setOptOut(!optOut())
    return { broadcast_opt_out: optOut() }
  }
  if (pathname === '/cabinet/api/tariffs') return tariffs
  if (pathname === '/cabinet/api/me/subscription') return subscription()
  if (pathname === '/cabinet/api/me/trial') {
    return { enabled: true, can_activate: false, days: 3, traffic_gb: 10, device_limit: 1 }
  }
  if (pathname === '/cabinet/api/me/devices') return devices()
  if (pathname === '/cabinet/api/me/traffic-usage') return traffic()
  if (pathname === '/cabinet/api/me/referrals') return referrals()
  if (pathname === '/cabinet/api/me/purchases') return purchases()
  if (pathname === '/cabinet/api/me/loyalty') return loyalty()
  if (pathname === '/cabinet/api/me/loyalty/history') return { items: [] }
  if (pathname === '/cabinet/api/me/partner') return partner()
  if (pathname === '/cabinet/api/promocodes/state') return { has_pending_discount: false, pending_discount: null }
  if (pathname === '/cabinet/api/support/summary') return { has_open_ticket: true, ticket_status: 'open', unread_count: 1 }
  if (pathname === '/cabinet/api/support/conversation') return support()
  if (pathname === '/cabinet/api/fortune/status') return fortune()
  if (pathname === '/cabinet/api/fortune/recent-wins') {
    return { items: [{ spin_at: iso(-3600_000), reward_type: 'days_3', reward_value: 3, masked_name: 'А***а' }] }
  }
  if (pathname === '/cabinet/api/public/status') return status()
  if (pathname === '/cabinet/api/payments/preview') return preview(url)
  if (pathname === '/cabinet/api/payments/checkout') {
    return { payment_url: 'http://127.0.0.1:5173/cabinet/payment/status/1', checkout_id: 1, purchase_id: 1, status: 'pending', provider: 'yookassa' }
  }
  if (/^\/cabinet\/api\/payments\/\d+\/status$/.test(pathname)) {
    return {
      status: 'paid',
      subscription_link: 'https://example.invalid/sub/demo',
      purchase_kind: 'subscription',
      payment_id: 1,
      amount: 1990,
      currency: 'RUB',
      invoice_type: 'yookassa',
      paid_at: iso(0),
      month: 1,
      expire_at: iso(30 * day),
    }
  }
  if (pathname === '/cabinet/api/me/deeplink') {
    return { deeplink: 'happ://add/https://example.invalid/sub/demo' }
  }
  if (pathname === '/cabinet/api/me/connect-invite') {
    return { url: 'http://127.0.0.1:5173/cabinet/connect#t=demo', expires_at: iso(day) }
  }
  return null
}

function bootstrap() {
  return {
    google_oauth_enabled: true,
    yandex_oauth_enabled: true,
    vk_oauth_enabled: false,
    telegram_oidc_enabled: false,
    telegram_web_auth_mode: 'widget',
    turnstile_enabled: false,
    brand_name: 'StarlitVPN',
    fortune_nav_visible: true,
    partner_nav_visible: true,
    partner_max_percent: 30,
    support_chat_enabled: true,
    light_theme_enabled: true,
    subscription_loyalty_visible: true,
    decor_theme: 'halloween',
    site_links: {
      support: 'https://t.me/starlitsupportbot',
      channel: 'https://t.me/starlitvpn',
    },
    landing: { show_tariffs: true, show_steps: true, show_features: true, show_faq: true },
    payment_providers: {
      yookassa: true,
      cryptopay: true,
      telegram: true,
      heleket: false,
    },
  }
}

function me(optOut: boolean) {
  return {
    id: 7,
    email: 'demo@starlit.vpn',
    email_verified: true,
    language: 'ru',
    providers: ['email', 'telegram'],
    has_telegram_link: true,
    has_password: true,
    can_use_email_password_login: true,
    customer_id: 42,
    google_oauth_enabled: true,
    yandex_oauth_enabled: true,
    vk_oauth_enabled: false,
    telegram_oidc_enabled: false,
    registered_at: iso(-120 * day),
    can_delete_account_ui: true,
    telegram_id: 100200300,
    is_admin: false,
    display_name: 'Михаил',
    username: 'starlit',
    identity_provider: 'telegram',
    broadcast_opt_out: optOut,
  }
}

function subscription() {
  return {
    expire_at: iso(46 * day),
    subscription_link: 'https://example.invalid/sub/demo',
    subscription_period_months: 1,
    traffic_used_gb: 38,
    traffic_limit_gb: null,
    is_trial: false,
    tariff: { id: 1, slug: 'start', name: 'Старт', traffic_gb: null, device_limit: 3 },
    loyalty_xp: 420,
    loyalty_tier: 'Silver',
    hwid_extra: {
      enabled: true,
      ui_visible: true,
      current_limit: 3,
      base_limit: 3,
      max_limit: 8,
      extra_active: 0,
      can_increase: true,
      can_decrease: false,
      price_rub_month: 79,
      stars_price_month: 50,
      days_left: 46,
    },
  }
}

function devices() {
  return {
    enabled: true,
    device_limit: 3,
    connected: 2,
    devices: [
      { hwid: 'iphone-1', platform: 'ios', device_model: 'iPhone', custom_name: 'iPhone', created_at: iso(-20 * day) },
      { hwid: 'win-1', platform: 'windows', device_model: 'PC', custom_name: 'Компьютер', created_at: iso(-8 * day) },
    ],
  }
}

function traffic() {
  const categories: string[] = []
  const points: number[] = []
  for (let i = 13; i >= 0; i--) {
    const d = new Date(Date.now() - i * day)
    categories.push(d.toISOString().slice(0, 10))
    points.push(Math.round((0.4 + ((i * 3) % 5) / 5) * GB))
  }
  return {
    enabled: true,
    period_start: iso(-30 * day),
    period_end: iso(0),
    categories,
    points,
    total_bytes: points.reduce((a, b) => a + b, 0),
  }
}

function referrals() {
  return {
    referrer_telegram_id: 100200300,
    stats: {
      total: 3,
      paid: 1,
      active: 1,
      conversion_pct: 33,
      earned_days_total: 14,
      earned_days_last_month: 7,
      referral_days_per_paid_default: 7,
    },
    referees: [{ telegram_id_masked: '12***89', name: 'А***а', earned_days: 7, active: true }],
    ledger: [{ actor: '@a***a', days: 7, months: 1, kind: 'first_referrer', created_at: iso(-6 * day) }],
    bot_start_link: 'https://t.me/starlitvpnbot?start=ref_100200300',
    cabinet_register_link: 'http://127.0.0.1:5173/cabinet/register?ref=ref_100200300',
    referral_mode: 'days',
    referral_bonus_days_default: 7,
    referral_first_referrer_days: 7,
    referral_first_referee_days: 3,
    referral_repeat_referrer_days: 3,
  }
}

function purchases() {
  return {
    items: [
      {
        id: 18,
        amount: 249,
        currency: 'RUB',
        status: 'paid',
        invoice_type: 'yookassa',
        purchase_kind: 'subscription',
        month: 1,
        paid_at: iso(-20 * day),
        created_at: iso(-20 * day),
      },
    ],
  }
}

function loyalty() {
  return {
    enabled: true,
    xp: 420,
    current: { sort_order: 2, xp_min: 300, discount_percent: 5, display_name: 'Silver' },
    next: { sort_order: 3, xp_min: 800, discount_percent: 10, display_name: 'Gold' },
    progress_percent: 24,
    xp_in_segment: 120,
    xp_segment_span: 500,
    xp_until_next: 380,
    first_discount_xp_min: 100,
  }
}

function partner() {
  return {
    enabled: true,
    applications_enabled: true,
    status: 'none',
    terms: {
      first_percent: 30,
      renewal_percent: 15,
      hold_days: 14,
      min_payout: 1000,
      payout_cooldown_days: 7,
      max_links: 5,
      count_extra_hwid: false,
    },
  }
}

function support() {
  return {
    has_open_ticket: true,
    ticket_status: 'open',
    unread_count: 1,
    messages: [
      { id: 1, direction: 'out', text: 'Не открывается сайт с телефона.', created_at: iso(-2 * 3600_000) },
      { id: 2, direction: 'in', text: 'Проверьте, что подписка импортирована в приложение.', author_label: 'Поддержка', created_at: iso(-3600_000) },
    ],
  }
}

function fortune() {
  return {
    enabled: true,
    panel_ready: true,
    can_spin: true,
    spins_used_today: 0,
    max_spins_per_day: 3,
    daily_free_enabled: true,
    daily_free_available: true,
    min_subscription_days: 1,
    spin_cost_days: 1,
    subscription_remain_hours: 46 * 24,
    subscription_remain_days: 46,
    expire_at: iso(46 * day),
    sectors: [
      { index: 0, reward_type: 'days_3', display_days: 3 },
      { index: 1, reward_type: 'xp', display_days: 0 },
      { index: 2, reward_type: 'discount_5', display_percent: 5 },
      { index: 3, reward_type: 'days_7', display_days: 7 },
      { index: 4, reward_type: 'days_1', display_days: 1 },
      { index: 5, reward_type: 'discount_3', display_percent: 3 },
    ],
    winner_feed: { enabled: true, fake_fill: false },
  }
}

function status() {
  const node = (name: string, country: string) => ({
    id: country,
    name,
    country,
    state: 'online',
    probe: { world_ok: 4, world_total: 4, russia_ok: 3, russia_total: 3, world_ping_ms: 40, history: [1, 1, 1, 1, 1] },
  })
  return {
    available: true,
    updated_at: iso(0),
    online: 4,
    total: 4,
    show_map: true,
    probes_enabled: true,
    title: 'Статус серверов',
    nodes: [node('Германия', 'DE'), node('Нидерланды', 'NL'), node('Финляндия', 'FI'), node('Эстония', 'EE')],
  }
}

function preview(url: URL) {
  const period = Number(url.searchParams.get('period') || '1')
  const tariffId = Number(url.searchParams.get('tariff_id') || '1')
  const prices: Record<number, number[]> = {
    1: [249, 669, 1194, 2148],
    2: [1990, 5370, 9540, 17880],
    3: [399, 1077, 1914, 3588],
  }
  const idx = [1, 3, 6, 12].indexOf(period)
  const amount = (prices[tariffId] || prices[1])[idx >= 0 ? idx : 0]
  const stars = url.searchParams.get('provider') === 'telegram'
  return {
    amount: stars ? Math.round(amount / 2) : amount,
    currency: stars ? 'STARS' : 'RUB',
    amount_rub: amount,
    sales_mode: 'tariffs',
    scenario: tariffId === 1 ? 'renew' : 'upgrade',
    purchase_kind: 'subscription',
    base_amount_rub: amount,
    list_price_rub: amount,
  }
}
