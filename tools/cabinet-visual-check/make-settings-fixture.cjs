// Полная фикстура «Настроек бота»: все группы реестра с примерными значениями.
// node make-settings-fixture.cjs > fixtures/admin-settings-full.json
const fs = require('fs')
const path = require('path')
const root = path.resolve(__dirname, '../..')
const src = fs.readFileSync(path.join(root, 'internal/config/settings_registry.go'), 'utf8') +
  fs.readFileSync(path.join(root, 'internal/config/cabinet_push.go'), 'utf8')
const order = ['cabinet', 'status', 'cabinet_connect', 'tariffs', 'trial', 'hwid', 'traffic', 'referral', 'partner', 'stars', 'loyalty', 'payments', 'payments_notify', 'moynalog', 'access', 'links', 'tags', 'lifecycle', 'notifications', 'fortune']
const enums = {
  CABINET_DECOR_THEME: ['off', 'green', 'halloween_2', 'new_year'],
  CABINET_DEEPLINK_HAPP_CRYPT_VERSION: ['crypt5', 'crypt4'],
  CABINET_TARIFF_PRICE_DISPLAY: ['monthly', 'marketing'],
  CABINET_TARIFF_SAVINGS_BADGE: ['none', 'corner', 'old_price'],
  TRIAL_TRAFFIC_LIMIT_RESET_STRATEGY: ['day', 'week', 'month', 'month_rolling', 'never'],
  TRAFFIC_LIMIT_RESET_STRATEGY: ['day', 'week', 'month', 'month_rolling', 'never'],
}
const textVals = {
  CABINET_DECOR_SCHEDULE: '[]',
  CABINET_HOME_QUICK_ACTIONS: '',
  YOOKASA_PAYMENT_METHODS: '',
  PLATEGA_PAYMENT_METHODS: '',
  CABINET_CHECKOUT_DEFAULT_METHOD: '',
  MOYNALOG_RECEIPT_FOR: 'yookassa, platega',
  PAYMENTS_NOTIFY_EVENTS: 'paid, cancel',
  REMNAWAVE_TAG: 'MEOWS',
}
const byGroup = {}
const seen = new Set()
for (const m of src.matchAll(/Key:\s*"([A-Z0-9_]+)",\s*Group:\s*"(\w+)",\s*Type:\s*Setting(\w+)/g)) {
  const [, key, group, t] = m
  if (seen.has(key)) continue
  seen.add(key)
  const type = t === 'CSVInt' ? 'csv_int' : t.toLowerCase()
  const f = { key, type, value: type === 'bool' ? 'true' : type === 'int' || type === 'float' ? '3' : (textVals[key] ?? ''), source: 'default', instant: type === 'bool' }
  if (enums[key]) { f.enum_values = enums[key]; f.value = enums[key][0] }
  ;(byGroup[group] ||= []).push(f)
}
// push-настройки из cabinet_push.go
for (const m of src.matchAll(/CABINET_PUSH_[A-Z_]+/g)) {
  if (seen.has(m[0])) continue
  seen.add(m[0])
  ;(byGroup.notifications ||= []).push({ key: m[0], type: 'bool', value: 'true', source: 'default', instant: true })
}
const groups = order.filter((id) => byGroup[id]).map((id) => ({ id, fields: byGroup[id] }))
process.stdout.write(JSON.stringify({ '/admin/settings': { groups } }, null, 1))
