// Генератор фикстур главной (редизайн «Главная + Подписка»): все состояния из SPEC §6.
// Даты считаются от сегодняшнего дня, чтобы кольцо дней показывало нужное число.
const fs = require('fs')
const path = require('path')
const day = 864e5
const iso = (d) => new Date(Date.now() + d * day).toISOString()
const devs = [
  { hwid: 'h1', platform: 'iOS', os_version: '18.1', device_model: 'iPhone 13' },
  { hwid: 'h2', platform: 'Android', os_version: '14', device_model: 'Redmi Note 12' },
  { hwid: 'h3', platform: 'Windows', os_version: '11', device_model: 'Ноутбук' },
  { hwid: 'h4', platform: 'AndroidTV', os_version: '11', device_model: 'Mi TV' },
  { hwid: 'h5', platform: 'macOS', os_version: '15.1', device_model: 'MacBook Air' },
]
const GB = 1024 ** 3
const shape = [0.34, 0.22, 0.16, 0.17, 0.16, 1, 0.2, 0.18, 0.3]
function usage(totalGb) {
  const sum = shape.reduce((a, b) => a + b, 0)
  const points = shape.map((s) => Math.round((s / sum) * totalGb * GB))
  const cats = shape.map((_, i) => new Date(Date.now() - (shape.length - 1 - i) * day).toISOString().slice(0, 10))
  return { enabled: true, period_start: cats[0], period_end: cats[cats.length - 1], categories: cats, points, total_bytes: totalGb * GB }
}
const hwid = (limit, connected) => ({
  enabled: true, ui_visible: true, current_limit: limit, base_limit: 3, max_limit: 8, extra_active: limit - 3,
  can_increase: true, can_decrease: limit > 3, price_rub_month: 75, stars_price_month: 40, days_left: 41,
})
function fx({ days, trial = false, tariff = 'Премиум', limit = 5, nDev = 4, used = 0.8, trafficLimit = null, strategy = null }) {
  return {
    '/me': { id: 1, email: 'user@example.invalid', email_verified: true, language: 'ru', providers: ['password', 'telegram'], has_telegram_link: true, has_password: true, is_admin: true, customer_id: 1 },
    '/auth/bootstrap': { site_links: { channel: 'https://t.me/example', feedback: 'https://t.me/example_reviews' }, partner_max_percent: 20, support_chat_enabled: true },
    '/me/subscription': {
      expire_at: iso(days + 0.5), subscription_link: 'https://example.invalid/sub/abc', subscription_period_months: 3,
      traffic_used_gb: used, traffic_limit_gb: trafficLimit, is_trial: trial,
      tariff: { id: 1, slug: 'premium', name: trial ? '' : tariff, traffic_gb: trafficLimit, device_limit: limit },
      loyalty_xp: 0, loyalty_tier: null,
      hwid_extra: trial ? null : hwid(limit, nDev),
      traffic_reset_strategy: strategy, next_traffic_reset_at: strategy ? iso(18) : null,
    },
    '/me/devices': { enabled: true, device_limit: limit, connected: nDev, devices: devs.slice(0, nDev) },
    '/me/traffic-usage': nDev ? usage(trafficLimit ? used : used) : { enabled: true, categories: [], points: [], total_bytes: 0 },
    '/me/trial': { enabled: true, can_activate: false, days: 3 },
    '/me/connect-invite': { url: 'https://example.invalid/c/abc', expires_at: iso(7) },
  }
}
const set = {
  regular: fx({ days: 89 }),
  trial: fx({ days: 3, trial: true, limit: 1, nDev: 0, used: 0 }),
  warn: fx({ days: 6 }),
  crit: fx({ days: 2 }),
  expired: { ...fx({ days: -2 }) },
  full: fx({ days: 89, nDev: 5 }),
  limit: fx({ days: 18, tariff: 'Стандарт', nDev: 3, used: 32, trafficLimit: 100, strategy: 'MONTH' }),
  tlimitwarn: fx({ days: 18, tariff: 'Стандарт', nDev: 3, used: 85, trafficLimit: 100, strategy: 'MONTH' }),
  tout: fx({ days: 18, tariff: 'Стандарт', nDev: 3, used: 100, trafficLimit: 100, strategy: 'MONTH' }),
}
const out = path.join(__dirname, 'fixtures')
for (const [k, v] of Object.entries(set)) fs.writeFileSync(path.join(out, `home-${k}.json`), JSON.stringify(v, null, 2) + '\n')
console.log(Object.keys(set).map((k) => `home-${k}.json`).join(' '))
