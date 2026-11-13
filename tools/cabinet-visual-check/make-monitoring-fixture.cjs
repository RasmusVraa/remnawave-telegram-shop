// Фикстура страницы «Мониторинг»: node tools/cabinet-visual-check/make-monitoring-fixture.cjs
// Данные выдуманные. Время — относительно запуска, чтобы «Сегодня» и таймеры были живыми.
const fs = require('fs')
const path = require('path')

const now = Date.now()
const ago = (min) => new Date(now - min * 60_000).toISOString()

let id = 900
const ev = (min, type, extra) => ({
  id: id--,
  at: ago(min),
  type,
  level: 1,
  category: 'money',
  source: 'bot',
  payload: {},
  ...extra,
})

const items = [
  ev(1, 'payment.subscription_renew', { customer_id: 11, customer: '@anna_k', payload: { amount: 549, currency: 'RUB', provider: 'platega', months: 3, tariff: 'Базовый' } }),
  ev(2, 'support.message', { category: 'support', source: 'cabinet', customer_id: 12, customer: '@dmitry88', payload: { preview: 'не подключается на айфоне после обновления, уже переустановил приложение', images: 0 } }),
  ev(3, 'bot.button', { level: 2, category: 'bot', customer_id: 13, customer: '@sergey_pro', payload: { data: 'buy' } }),
  ev(4, 'user.registered', { category: 'users', customer_id: 13, customer: '@sergey_pro' }),
  ev(6, 'node.down', { category: 'nodes', source: 'panel', payload: { name: 'NL-Amsterdam-2', country: 'NL' } }),
  ev(8, 'payment.failed', { customer_id: 14, customer: '@kirill_v', payload: { amount: 299, currency: 'RUB', provider: 'yookasa' } }),
  ev(9, 'fortune.spin', { category: 'promo', source: 'cabinet', customer_id: 15, customer: '@marina_z', payload: { reward_type: 'days_3', reward_value: 3 } }),
  ev(11, 'payment.tariff_upgrade', { customer_id: 16, customer: '@olga_m', payload: { amount: 350, currency: 'RUB', provider: 'yookasa', tariff: 'Семейный' } }),
  ev(13, 'promo.activated', { category: 'promo', customer_id: 17, customer: '@ivan_ivanov', payload: { code: 'AUTUMN30', discount: 30 } }),
  ev(15, 'payment.extra_hwid', { customer_id: 18, customer: '@lena_s', payload: { amount: 99, currency: 'RUB', provider: 'telegram', devices: 1 } }),
  ev(18, 'payment.extra_traffic', { customer_id: 19, customer: '@pavel_t', payload: { amount: 149, currency: 'RUB', provider: 'platega', traffic_bytes: 50 * 1024 ** 3 } }),
  ev(21, 'user.login', { category: 'users', source: 'cabinet', customer_id: 20, customer: '@nikita_w', payload: { method: 'telegram' } }),
  ev(25, 'admin.action', { category: 'admin', source: 'cabinet', customer_id: 21, customer: '@test_user', actor: '@meows', actor_tg_id: 1, payload: { action: 'users/:id/extend', method: 'POST', params: { days: 7 } } }),
  ev(60 * 9, 'subscription.expired_digest', { category: 'subs', source: 'system', payload: { expired: 14, not_renewed: 9 } }),
  ev(60 * 30, 'system.broadcast', { category: 'admin', source: 'system', payload: { status: 'done', total: 3400, sent: 3388, blocked: 10, failed: 2 } }),
  ev(60 * 31, 'node.up', { category: 'nodes', source: 'panel', payload: { name: 'DE-Frankfurt-1', down_minutes: 7 } }),
]

const fixture = {
  '/admin/attention': {
    items: [
      { key: 'node_down', kind: 'incident', severity: 'crit', count: 1, since: ago(12), params: { name: 'NL-Amsterdam-2' } },
      { key: 'support_waiting', kind: 'incident', severity: 'warn', count: 2, since: ago(41) },
    ],
  },
  '/admin/monitoring/summary': {
    period: 'hour', payments: 12, revenue: { RUB: 4870, XTR: 150 }, failed: 3, failed_provider: 'yookasa',
    registrations_bot: 6, registrations_cabinet: 3, node_downs: 1, nodes_online: 7, nodes_total: 8, nodes_known: true,
  },
  '/admin/monitoring/events': { items, bot_actions_enabled: true },
}

const out = path.join(__dirname, 'fixtures', 'admin-monitoring.json')
fs.writeFileSync(out, JSON.stringify(fixture, null, 2) + '\n')
console.log(out)
