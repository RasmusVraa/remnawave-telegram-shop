// Фикстура страницы «Промокоды» в админке: node make-promos-fixture.cjs
// Даты считаются от текущего момента, чтобы «скоро истекает» и «истёк» не устаревали.
// Данные выдуманные.
const fs = require('fs')
const path = require('path')

const now = Date.now()
const H = 3600 * 1000
const D = 24 * H
const iso = (ms) => new Date(ms).toISOString()
// 23:59 по Москве — съёмка идёт в поясе Europe/Moscow.
const endOfDay = (ms) => {
  const d = new Date(ms + 3 * H)
  d.setUTCHours(20, 59, 0, 0)
  return d.toISOString()
}

const base = {
  subscription_days: null,
  trial_days: null,
  extra_hwid_delta: null,
  discount_percent: null,
  discount_ttl_hours: null,
  max_uses: null,
  uses_count: 0,
  valid_until: null,
  active: true,
  first_purchase_only: false,
  require_customer_in_db: false,
  allow_trial_without_payment: true,
  created_at: iso(now - 20 * D),
  discount_max_subscription_payments_per_customer: 1,
  tariff_id: null,
  note: null,
}

const promos = [
  { ...base, id: 47, code: 'MORNING', type: 'subscription_days', subscription_days: 7, max_uses: 6, uses_count: 2, valid_until: endOfDay(now), created_at: iso(now - 3 * H), note: 'Утренняя рассылка в боте' },
  { ...base, id: 46, code: 'HELL26', type: 'subscription_days', subscription_days: 14, max_uses: 100, uses_count: 64, tariff_id: 2, note: 'Хэллоуин · пост в канале' },
  { ...base, id: 45, code: 'CRYPTO', type: 'discount', discount_percent: 15, discount_ttl_hours: 24, max_uses: 100, uses_count: 23, note: 'для @cryptochat' },
  { ...base, id: 44, code: 'TRIAL3', type: 'trial', trial_days: 3, max_uses: 100, uses_count: 87, valid_until: endOfDay(now + 15 * D), first_purchase_only: true },
  { ...base, id: 43, code: 'REGINA', type: 'discount', discount_percent: 50, discount_ttl_hours: 0, max_uses: 1, uses_count: 1, note: 'блогер Регина, сторис' },
  { ...base, id: 42, code: 'FRIEND', type: 'extra_hwid', extra_hwid_delta: 1, max_uses: 20, uses_count: 8, valid_until: iso(now + 16 * D), active: false },
  { ...base, id: 41, code: 'BLACKFRIDAY2026SUPERSALE', type: 'discount', discount_percent: 30, discount_ttl_hours: 72, max_uses: 0, uses_count: 412, discount_max_subscription_payments_per_customer: 3, valid_until: iso(now - 2 * D) },
]

const fx = {
  '/admin/bootstrap': { sales_mode: 'tariffs', loyalty_enabled: true, fortune_enabled: false, providers: {}, features: {} },
  '/admin/tariffs': [
    { id: 1, slug: 'basic', name: 'Базовый', sort_order: 1, is_active: true },
    { id: 2, slug: 'pro', name: 'Pro', sort_order: 2, is_active: true },
  ],
  '/admin/promos': {
    items: promos,
    total: 47,
    page: 1,
    limit: 20,
    counts: {
      status: { all: 47, active: 31, ended: 9, off: 7 },
      type: { all: 47, subscription_days: 18, discount: 16, trial: 8, extra_hwid: 5 },
    },
    summary: { redemptions_24h: 12, redemptions_total: 1284 },
  },
}

const paybacks = {
  47: { paid_customers: 1, payments: 1, revenue_rub: 290, revenue_stars: 0 },
  46: { paid_customers: 9, payments: 11, revenue_rub: 8400, revenue_stars: 0 },
  45: { paid_customers: 12, payments: 14, revenue_rub: 6720, revenue_stars: 350 },
  44: { paid_customers: 21, payments: 25, revenue_rub: 14900, revenue_stars: 0 },
  43: { paid_customers: 0, payments: 0, revenue_rub: 0, revenue_stars: 0 },
  42: { paid_customers: 2, payments: 2, revenue_rub: 980, revenue_stars: 0 },
  41: { paid_customers: 380, payments: 402, revenue_rub: 398000, revenue_stars: 0 },
}
const pending = { 45: 4, 43: 1, 41: 0 }

const users = [
  ['alex_k', null, 482910, 3 * H, 490],
  [null, 'Мария', 77120934, 4 * H, 0],
  ['dm1try', null, 220518, 26 * H, 1290],
  [null, null, 3319047, 29 * H, 0],
  ['nastya.v', null, 90871, 36 * H, 290],
  [null, 'Иван П.', 150234, 2 * D + 5 * H, 0],
]

for (const p of promos) {
  fx[`/admin/promos/${p.id}`] = {
    promo: p,
    redemptions: p.uses_count,
    redemptions_today: Math.min(p.uses_count, 3),
    redemptions_24h: Math.min(p.uses_count, 3),
    pending_discounts: pending[p.id] ?? 0,
    payback: paybacks[p.id],
  }
  fx[`/admin/promos/${p.id}/redemptions`] = {
    items: p.uses_count
      ? users.slice(0, Math.min(6, p.uses_count)).map(([u, n, id, ago, rub]) => ({
          used_at: iso(now - ago),
          customer_id: id,
          telegram_username: u,
          nickname: n,
          paid_count: rub ? 1 : 0,
          paid_rub: rub,
          paid_stars: 0,
        }))
      : [],
    // Строк меньше, чем uses_count: остальные активации — от удалённых аккаунтов (окно это пишет).
    total: Math.min(6, p.uses_count),
    page: 1,
    limit: 20,
  }
}

const out = path.join(__dirname, 'fixtures', 'admin-promos.json')
fs.writeFileSync(out, JSON.stringify(fx, null, 2))
console.log(out)
