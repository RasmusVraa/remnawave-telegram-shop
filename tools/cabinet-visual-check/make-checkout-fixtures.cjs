// Фикстуры страницы оформления: сценарии из .cursor/work-in-progress/checkout-summary/SPEC.md §5.
// Запуск: node tools/cabinet-visual-check/make-checkout-fixtures.cjs
// Снимок: --route '/cabinet/checkout?tariff=premium&months=1' --fixtures …/fixtures/checkout-<сценарий>.json
const fs = require('fs')
const path = require('path')

const dir = path.join(__dirname, 'fixtures')
const GB = 1024 ** 3

const tariffs = {
  sales_mode: 'tariffs',
  currency: 'RUB',
  price_display: 'monthly',
  tariffs: [
    {
      id: 1, slug: 'basic', name: 'Базовый', device_limit: 2, traffic_limit_bytes: 200 * GB,
      prices: [{ months: 1, amount_rub: 150 }, { months: 3, amount_rub: 420 }],
    },
    {
      id: 2, slug: 'premium', name: 'Премиум', device_limit: 3, traffic_limit_bytes: 0,
      bypass_traffic_limit_bytes: 15 * GB, bypass_label: 'обход',
      prices: [{ months: 1, amount_rub: 250 }, { months: 3, amount_rub: 700 }],
    },
    {
      id: 3, slug: 'ultra', name: 'Ультра', device_limit: 5, traffic_limit_bytes: 0,
      bypass_traffic_limit_bytes: 25 * GB, bypass_label: 'LTE',
      prices: [{ months: 1, amount_rub: 350 }],
    },
  ],
}

const bootstrap = {
  payment_providers: { yookassa: true, platega_sbp: true, platega_crypto: true },
  yookassa_methods: ['sbp', 'bank_card'],
  platega_methods: ['sbp', 'crypto'],
  checkout_default_method: 'yookassa:sbp',
  site_links: { privacy_policy: 'https://example.com/privacy', terms_of_service: 'https://example.com/terms' },
}

function subscription({ extra = 0, trial = false } = {}) {
  return {
    expire_at: '2026-12-15T12:00:00Z',
    subscription_link: 'https://example.invalid/sub/abc',
    is_trial: trial,
    tariff: { id: 2, slug: 'premium', name: 'Премиум', traffic_gb: null, device_limit: 3 },
    loyalty_xp: 0,
    loyalty_tier: null,
    hwid_extra: extra
      ? {
          enabled: true, ui_visible: true, current_limit: 3 + extra, base_limit: 3, max_limit: 10,
          extra_active: extra, can_increase: true, can_decrease: true,
          price_rub_month: 75, stars_price_month: 40, days_left: 30,
        }
      : null,
  }
}

/** Ответ /payments/preview так же, как его собирает Preview в checkout.go. */
function preview({ price, extra = 0, months = 1, scenario = 'renew', loy = 0, promo = 0, cap = 100, sw }) {
  const extraRub = extra * 75 * months
  const base = price + extraRub
  const pct = Math.min(loy + promo, cap)
  const amount = base - Math.round((base * pct) / 100)
  const p = {
    amount, amount_rub: amount, currency: 'RUB', sales_mode: 'tariffs', scenario,
    base_amount_rub: base,
    loyalty_discount_pct: loy || undefined, promo_discount_pct: promo || undefined, total_discount_pct: pct || undefined,
    extra_hwid_active: extra || undefined,
    extra_hwid_included: extra ? true : undefined,
    extra_hwid_amount_rub: extraRub || undefined,
  }
  if (sw) {
    Object.assign(p, {
      tariff_switch_remaining_days: sw.left,
      tariff_switch_bonus_days: sw.bonus,
      tariff_switch_period_days: 30 * months,
      tariff_switch_total_days: 30 * months + sw.bonus,
    })
  }
  return p
}

/** Предпросмотр с тумблером и без: ключи с параметром renew_extra_hwid (см. capture.mjs). */
function previews(opts, extra) {
  const out = { '/payments/preview': preview({ ...opts, extra: 0 }) }
  if (extra) {
    out['/payments/preview?renew_extra_hwid=true'] = preview({ ...opts, extra })
    out['/payments/preview?renew_extra_hwid=false'] = preview({ ...opts, extra: 0 })
  }
  return out
}

function make(name, { extra = 0, trial = false, last = 'yookassa:sbp', preview: pv, delay }) {
  const f = {
    '/tariffs': tariffs,
    '/auth/bootstrap': bootstrap,
    '/me/subscription': subscription({ extra, trial }),
    '/payments/last-method': { method: last },
    ...previews(pv, extra),
  }
  if (delay) f.__delay = delay
  fs.writeFileSync(path.join(dir, `checkout-${name}.json`), JSON.stringify(f, null, 2))
}

make('renew', { extra: 4, preview: { price: 250, loy: 8 } })
make('renew-plain', { extra: 4, preview: { price: 250 } })
make('downgrade', { extra: 4, preview: { price: 150, loy: 8, scenario: 'downgrade', sw: { left: 39, bonus: 64 } } })
make('upgrade', { extra: 4, preview: { price: 350, loy: 8, scenario: 'upgrade', sw: { left: 39, bonus: 27 } } })
make('no-extra', { preview: { price: 250, loy: 8 } })
make('new', { last: '', preview: { price: 250, promo: 10, scenario: 'new' } })
make('two-discounts', { extra: 4, preview: { price: 250, loy: 8, promo: 10 } })
make('trial', { trial: true, last: '', preview: { price: 250, scenario: 'renew' } })
make('3months', { extra: 1, preview: { price: 700, months: 3 } })
make('cap', { preview: { price: 250, loy: 15, promo: 20, cap: 25 } })
// Пересчёт: ответ без мест не приходит, после выключения тумблера висит прошлый.
// Задержка короче таймаута снимка: иначе прогон падает с TimeoutError.
make('recalc', { extra: 4, preview: { price: 250, loy: 8 }, delay: { '/payments/preview?renew_extra_hwid=false': 12000 } })

// Classic: один «тариф» без имени (normalizeTariffsResponse подставит slug), route ?tariff=classic.
{
  const f = JSON.parse(fs.readFileSync(path.join(dir, 'checkout-no-extra.json'), 'utf8'))
  f['/tariffs'] = {
    sales_mode: 'classic',
    currency: 'RUB',
    tariffs: [{ id: 0, slug: 'classic', name: '', device_limit: 3, traffic_limit_bytes: 0, prices: [{ months: 1, amount_rub: 199 }] }],
  }
  f['/payments/preview'] = { ...preview({ price: 199, loy: 8, scenario: 'classic_renew' }), sales_mode: 'classic' }
  fs.writeFileSync(path.join(dir, 'checkout-classic.json'), JSON.stringify(f, null, 2))
}

// Докупка мест и трафика.
const sub = subscription({ extra: 1 })
fs.writeFileSync(
  path.join(dir, 'checkout-slots.json'),
  JSON.stringify(
    {
      '/auth/bootstrap': bootstrap,
      '/me/subscription': sub,
      '/payments/last-method': { method: 'yookassa:sbp' },
      '/payments/hwid/preview': {
        current_limit: 4, target_limit: 6, delta: 2, days_left: 30, amount_rub: 132, base_amount_rub: 150,
        currency: 'RUB', loyalty_discount_pct: 2, promo_discount_pct: 10, total_discount_pct: 12,
      },
    },
    null,
    2,
  ),
)
fs.writeFileSync(
  path.join(dir, 'checkout-traffic.json'),
  JSON.stringify(
    {
      '/auth/bootstrap': bootstrap,
      '/me/subscription': sub,
      '/payments/last-method': { method: 'yookassa:sbp' },
      '/me/traffic': {
        enabled: true, can_buy: true, label: 'обход', reset_at: '2026-12-01T00:00:00Z',
        main: { limit_bytes: 0, base_bytes: 0, extra_bytes: 0, used_bytes: 18 * GB },
        bypass: { limit_bytes: 30 * GB, base_bytes: 30 * GB, extra_bytes: 0, used_bytes: 26 * GB },
        extras: [], bypass_blocked: false,
        packs: [{ id: 12, kind: 'bypass', bytes: 20 * GB, amount_rub: 199 }],
      },
      '/payments/traffic/preview': {
        pack_id: 12, bytes: 20 * GB, kind: 'bypass', amount_rub: 183, base_amount_rub: 199, currency: 'RUB', loyalty_discount_pct: 8, total_discount_pct: 8,
      },
    },
    null,
    2,
  ),
)
console.log('ok')
