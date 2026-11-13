// Скриншоты к посту релиза: снимки кабинета с жёлтой рамкой и подписью на том,
// что изменилось. tools/release отправляет их альбомом к посту в Telegram.
//
// Кабинет снимается из собранного dist с подставным API — без базы, бота и
// входа, и в кадр не попадают настоящие пользователи. Данные берутся из фикстуры
// (см. README рядом).
//
// Запуск из корня репозитория, после `cd web/cabinet && npm run build`:
//
//   node tools/cabinet-visual-check/release-shots.mjs internal/cabinet/web/dist \
//     tools/cabinet-visual-check/fixtures/release-5-27.json release-screenshots
//
// Под новый релиз меняются только сценарии в конце файла: какую страницу
// открыть, что нажать и что обвести. Здесь — сценарии релиза 5.27.0
// (страница «Статус серверов»: публичная, в кабинете, в поддержке и в админке).
// mark(locator, 'Подпись', side): side — top | bottom | left | inside | inside-low.
// Каждый снимок после съёмки посмотреть глазами: подпись не должна закрывать
// текст интерфейса, рамка — резать соседние строки.

import http from 'node:http'
import fs from 'node:fs'
import path from 'node:path'
import { chromium } from 'playwright'

const [, , distDir, fixturesFile, outDir] = process.argv
const MIME = { '.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css', '.svg': 'image/svg+xml', '.woff2': 'font/woff2', '.png': 'image/png', '.webp': 'image/webp' }
const server = http.createServer((req, res) => {
  const url = (req.url ?? '/').split('?')[0]
  let file = path.join(distDir, decodeURIComponent(url).replace(/^\/cabinet/, ''))
  if (!fs.existsSync(file) || fs.statSync(file).isDirectory()) file = path.join(distDir, 'index.html')
  res.writeHead(200, { 'Content-Type': MIME[path.extname(file)] ?? 'application/octet-stream' })
  res.end(fs.readFileSync(file))
})
await new Promise((r) => server.listen(5604, r))

const base = {
  '/auth/refresh': { access_token: 'stub', token_type: 'Bearer', expires_in: 3600 },
  '/me': { id: 1, email: 'admin@example.com', email_verified: true, language: 'ru', providers: ['password'], has_telegram_link: true, has_password: true, google_oauth_enabled: false, is_admin: true, customer_id: 1 },
  '/admin/bootstrap': { providers: {}, features: {} },
  ...JSON.parse(fs.readFileSync(fixturesFile, 'utf8')),
}
// Время замеров в фикстуре — «4 минуты назад», а не дата записи фикстуры: иначе на снимке «Обновлено 1000 мин назад».
const measuredStamp = new Date(Date.now() - 4 * 60 * 1000).toISOString()
for (const n of base['/public/status']?.nodes ?? []) if (n.probe) n.probe.measured_at = measuredStamp
// Декор-тема приходит в /auth/bootstrap: сцена — та же фикстура с другой темой.
const withDecor = (theme) => ({ ...base, '/auth/bootstrap': { ...base['/auth/bootstrap'], decor_theme: theme } })
fs.mkdirSync(outDir, { recursive: true })

const b = await chromium.launch()
async function open(w, h, fx, route, prefix = '/cabinet') {
  const ctx = await b.newContext({ viewport: { width: w, height: h }, deviceScaleFactor: 2, colorScheme: 'dark', locale: 'ru-RU', timezoneId: 'Europe/Moscow' })
  await ctx.addInitScript(() => {
    localStorage.setItem('onboarding_completed', 'true')
    localStorage.setItem('cab_theme', 'dark')
  })
  await ctx.route('**/cabinet/api/**', (route) => {
    const p = new URL(route.request().url()).pathname.replace('/cabinet/api', '')
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(fx[p] ?? {}) })
  })
  const page = await ctx.newPage()
  const errors = []
  page.on('pageerror', (e) => errors.push(String(e)))
  await page.goto(`http://localhost:5604${prefix}${route}`, { waitUntil: 'networkidle' })
  // Сцены декор-тем проявляются плавно — даём им дорисоваться.
  await page.waitForTimeout(2600)
  return { ctx, page, errors }
}

// Жёлтая рамка вокруг элемента и подпись рядом. Пустая подпись — только рамка:
// подпись тогда ставится caption() туда, где она не закроет текст.
async function mark(locator, label, where = 'top') {
  await locator.first().evaluate((el, args) => {
    const [text, side] = args
    const r = el.getBoundingClientRect()
    const pad = 6
    const box = document.createElement('div')
    Object.assign(box.style, {
      position: 'fixed', left: `${r.left - pad}px`, top: `${r.top - pad}px`,
      width: `${r.width + pad * 2}px`, height: `${r.height + pad * 2}px`,
      border: '3px solid #FFB020', borderRadius: '14px', boxShadow: '0 0 0 5px rgba(255,176,32,.22)',
      pointerEvents: 'none', zIndex: 2147483646,
    })
    document.body.appendChild(box)
    if (!text) return
    const tag = document.createElement('div')
    tag.textContent = text
    Object.assign(tag.style, {
      position: 'fixed', background: '#FFB020', color: '#1d1400',
      font: '800 15px/1.2 Manrope, Inter, system-ui, sans-serif', padding: '7px 12px',
      borderRadius: '10px', zIndex: 2147483647, whiteSpace: 'nowrap', boxShadow: '0 6px 18px rgba(0,0,0,.35)',
    })
    document.body.appendChild(tag)
    const th = tag.offsetHeight
    const tw = tag.offsetWidth
    let top = side === 'bottom' || r.top - pad - th - 8 < 4 ? r.bottom + pad + 8 : r.top - pad - th - 8
    let left = Math.min(Math.max(4, r.left - pad), window.innerWidth - tw - 8)
    if (side === 'inside') {
      top = r.top + 14
      left = r.right - tw - 64
    }
    if (side === 'inside-low') {
      top = r.top + 40
      left = r.right - tw - 24
    }
    if (side === 'inline-right') {
      top = r.top + (r.height - th) / 2
      left = r.right - tw - 16
    }
    if (side === 'left') {
      top = r.top + (r.height - th) / 2
      left = r.left - pad - tw - 14
    }
    tag.style.top = `${top}px`
    tag.style.left = `${left}px`
  }, [label, where])
}
// Подпись без рамки — когда изменился весь экран (тема, сцена) и обводить нечего.
async function caption(page, label, top = 128) {
  await page.evaluate(([text, y]) => {
    const tag = document.createElement('div')
    tag.textContent = text
    Object.assign(tag.style, {
      position: 'fixed', top: `${y}px`, left: '50%', transform: 'translateX(-50%)',
      background: '#FFB020', color: '#1d1400', font: '800 17px/1.2 Manrope, Inter, system-ui, sans-serif',
      padding: '8px 14px', borderRadius: '10px', zIndex: 2147483647, whiteSpace: 'nowrap',
      boxShadow: '0 6px 18px rgba(0,0,0,.35)',
    })
    document.body.appendChild(tag)
  }, [label, top])
}
// Прокрутить так, чтобы блок оказался под шапкой, и обвести его.
async function scrollToBlock(page, locator, offset = 120) {
  await locator.first().evaluate((el, off) => {
    const y = el.getBoundingClientRect().top + window.scrollY - off
    window.scrollTo(0, y)
  }, offset)
  await page.waitForTimeout(500)
}
// RAW=1 — снимки без рамок и подписей: посмотреть, что вообще в кадре.
const RAW = process.env.RAW === '1'
const ONLY = (process.env.ONLY ?? '').split(',').filter(Boolean)
const want = (n) => ONLY.length === 0 || ONLY.includes(String(n))
// Имя вида '1/05-…': подпапка — номер альбома, альбом N уходит ответом на N-е сообщение поста.
const shot = (page, name) => {
  const file = path.join(outDir, name.replace('.png', '.jpg'))
  fs.mkdirSync(path.dirname(file), { recursive: true })
  return page.screenshot({ path: file, type: 'jpeg', quality: 86 })
}
const fxDir = path.dirname(fixturesFile)
const loadFx = (name) => JSON.parse(fs.readFileSync(path.join(fxDir, name), 'utf8'))
const scene = (name, extra = {}) => ({ ...base, ...loadFx(name), ...extra })
// Тарифы админки: «Премиум» отмечен популярным.
function adminTariffs() {
  const fx = scene('release-next-tariffs.json')
  for (const t of fx['/admin/tariffs']) t.is_popular = t.slug === 'premium'
  return fx
}
const M = RAW ? async () => {} : mark
const C = RAW ? async () => {} : caption
// В фикстуре обход по дням даёт 2,4 ГБ, а счётчик — 12 ГБ: подгоняем график под счётчик,
// чтобы на снимке сходились «12 из 30» и сумма под графиком.
function fitBypass(fx) {
  const u = fx['/me/traffic-usage']
  const want = fx['/me/traffic'].bypass.used_bytes
  const have = u.bypass_points.reduce((a, b) => a + b, 0)
  const k = want / have
  u.points = u.points.map((p, i) => p + u.bypass_points[i] * (k - 1))
  u.bypass_points = u.bypass_points.map((b) => b * k)
  u.total_bytes += want - have
  return fx
}

if (want(1)) {
  // 1. Главная: два кольца трафика — обычные серверы и обход.
  const fx = fitBypass(scene('split-ok.json'))
  const { ctx, page, errors } = await open(1440, 900, fx, '/dashboard')
  await page.mouse.move(2, 880)
  await M(page.getByText(/12\/30/).first().locator('xpath=ancestor::*[self::button or self::a or @role="button"][1]'), 'Безлимит + отдельный лимит обхода', 'top')
  await shot(page, '1/01-home-split.png')
  console.log('1', errors)
  await ctx.close()
}
if (want(2)) {
  // 2. Шторка «Расход трафика» на телефоне.
  const fx = fitBypass(scene('split-ok.json'))
  const { ctx, page, errors } = await open(390, 844, fx, '/dashboard')
  await page.mouse.move(2, 840)
  await page.getByText(/ГБ/).filter({ hasText: '/' }).first().click().catch((e) => console.log('usage', String(e).slice(0, 90)))
  await page.waitForTimeout(1300)
  await page.mouse.move(380, 470)
  await C(page, 'Расход по дням: обычные и обход', 480)
  await shot(page, '1/02-traffic-sheet.png')
  console.log('2', errors)
  await ctx.close()
}
if (want(3)) {
  // 3. Обход исчерпан: статус и окно докупки.
  const fx = scene('split-pack-discount.json')
  const { ctx, page, errors } = await open(1440, 900, fx, '/dashboard')
  await page.getByRole('button', { name: /Докупить/i }).first().click().catch((e) => console.log('click', String(e).slice(0, 80)))
  await page.waitForTimeout(1000)
  await page.mouse.move(2, 880)
  await C(page, 'Докупка пакета до конца месяца', 120)
  await shot(page, '1/03-traffic-pack.png')
  console.log('3', errors)
  await ctx.close()
}
if (want(4)) {
  // 4. Редактор тарифа, вкладка «Трафик»: обход, сквад, ноды, пакеты.
  const fx = adminTariffs()
  const { ctx, page, errors } = await open(1440, 1100, fx, '/admin/tariffs')
  await page.locator('[role="button"]').filter({ hasText: 'Стандарт' }).first().click()
  await page.waitForTimeout(700)
  await page.getByRole('tab', { name: 'Трафик' }).click().catch(() => page.getByText('Трафик', { exact: true }).first().click())
  await page.waitForTimeout(1000)
  await page.mouse.move(2, 1080)
  await M(page.getByText('Обход — отдельный лимит', { exact: true }).locator('xpath=ancestor::*[.//*[starts-with(normalize-space(text()), "Ноды — по ним")]][1]'), 'Лимит обхода, сквад и ноды', 'left')
  await shot(page, '1/04-admin-tariff-traffic.png')
  console.log('4', errors)
  await ctx.close()
}
if (want(5)) {
  // 5. Новая страница «Ноды».
  const fx = scene('admin-nodes.json')
  const { ctx, page, errors } = await open(1440, 1000, fx, '/admin/nodes')
  await page.mouse.move(2, 980)
  await C(page, 'Кто на какой ноде — не заходя в панель', 84)
  await shot(page, '1/08-admin-nodes.png')
  console.log('5', errors)
  await ctx.close()
}
if (want(6)) {
  // 6. Оплата в кабинете: кнопки Platega по способам.
  const fx = scene('checkout.json')
  fx['/auth/bootstrap'] = {
    ...fx['/auth/bootstrap'],
    payment_providers: { yookassa: true, platega: true, platega_sbp: true, platega_acquiring: true, platega_crypto: true, telegram: true, cryptopay: true },
    yookassa_methods: ['sbp'],
    platega_methods: ['all', 'sbp', 'acquiring', 'crypto'],
  }
  const slug = fx['/tariffs'].tariffs[0].slug
  const { ctx, page, errors } = await open(1440, 1340, fx, `/checkout?tariff=${slug}&months=1`)
  await page.mouse.move(2, 1320)
  await M(page.getByText('Все способы', { exact: true }).first().locator('xpath=ancestor::button[1]'), 'Platega: кнопка на каждый способ', 'left')
  await shot(page, '1/09-checkout-platega.png')
  console.log('6', errors)
  await ctx.close()
}
if (want(7)) {
  // 7. Настройки бота: новые вкладки, способы оплаты одинаковыми списками.
  const fx = scene('admin-settings-full.json')
  for (const g of fx['/admin/settings'].groups ?? []) {
    for (const s of g.fields ?? []) {
      if (s.key === 'PLATEGA_PAYMENT_METHODS') s.value = 'sbp,acquiring,all'
      if (s.key === 'YOOKASA_PAYMENT_METHODS') s.value = 'sbp,bank_card'
      if (s.key === 'PLATEGA_ENABLED' || s.key === 'YOOKASA_ENABLED') s.value = 'true'
    }
  }
  const { ctx, page, errors } = await open(1440, 1300, fx, '/admin/settings')
  await page.getByRole('tab', { name: 'Платежи' }).click()
  await page.waitForTimeout(500)
  await page.getByText('Способы оплаты', { exact: true }).first().click().catch((e) => console.log('card', String(e).slice(0, 90)))
  await page.waitForTimeout(700)
  await page.mouse.move(2, 1280)
  await scrollToBlock(page, page.getByText(/^Platega: кнопки$/i), 150)
  await M(page.getByText(/^Platega: кнопки$/i).first(), 'Кнопки Platega: порядок и включение', 'inline-right')
  await shot(page, '2/02-admin-settings.png')
  console.log('7', errors)
  await ctx.close()
}
if (want(8)) {
  // 8. Тарифы в админке на ПК: витрина.
  const fx = adminTariffs()
  const { ctx, page, errors } = await open(1440, 900, fx, '/admin/tariffs')
  await page.mouse.move(2, 880)
  await page.waitForTimeout(400)
  await M(page.locator('[role="button"]').filter({ hasText: 'Стандарт' }).first().locator('span', { hasText: '+30' }).first(), '', 'bottom')
  await C(page, 'Витрина тарифов: обход, докупка, «Популярный»', 84)
  await shot(page, '2/01-admin-tariffs.png')
  console.log('8', errors)
  await ctx.close()
}
if (want(9)) {
  // 9. Пользователи: сортировка по заголовкам и фильтр статуса в шапке.
  const fx = scene('admin-users-list.json')
  // Выдуманные пользователи: ни ников, ни Telegram ID настоящих людей в кадре.
  const tpl = fx['/admin/users'].items[0]
  const rows = [
    [2041, 'alex_demo', 'active', '2027-01-29', 2], [2038, 'maria_k', 'active', '2026-12-14', 3],
    [2031, null, 'trial', '2026-11-09', null], [2027, 'night_owl', 'active', '2026-11-08', 1],
    [2019, 'ivan_test', 'expired', '2026-10-30', 1], [2012, 'kate_vpn', 'active', '2027-03-02', 2],
    [2004, 'demo_user', 'inactive', '2026-09-12', null],
  ]
  fx['/admin/users'] = { ...fx['/admin/users'], total: rows.length, items: rows.map(([id, un, st, exp, t], i) => ({
    ...tpl, id, telegram_id: String(100000001 + i * 1111), telegram_username: un, status: st,
    rw_status: st === 'expired' ? 'EXPIRED' : 'ACTIVE', expire_at: `${exp}T10:00:00Z`, current_tariff_id: t,
  })) }
  const { ctx, page, errors } = await open(1440, 900, fx, '/admin/users')
  await page.locator('th').filter({ hasText: 'Статус' }).locator('button').last().click().catch((e) => console.log('filter', String(e).slice(0, 80)))
  await page.waitForTimeout(600)
  await M(page.locator('thead').first(), '', 'top')
  await C(page, 'Сортировка по столбцам и фильтр статуса', 84)
  await shot(page, '2/03-admin-users.png')
  console.log('9', errors)
  await ctx.close()
}
if (want(10)) {
  // 10. Статус серверов: один столбик — один замер.
  const fx = scene('release-next-status.json')
  const { ctx, page, errors } = await open(1440, 1000, fx, '/status', '')
  await page.mouse.move(2, 900)
  await scrollToBlock(page, page.getByText('Локация', { exact: true }), 230)
  await page.waitForTimeout(1200)
  await M(page.getByText('Последние 30 замеров', { exact: true }).first(), '', 'top')
  await C(page, 'Столбик — один замер, а не целый день', 120)
  await shot(page, '2/04-status-bars.png')
  console.log('10', errors)
  await ctx.close()
}
if (want(11)) {
  // 11. Тарифы у клиента на ПК: популярный — единственная цветная карточка, у обхода свой лимит.
  const fx = scene('tariffs.json')
  const GB = 1024 ** 3
  const prices = (a) => [1, 3, 6, 12].map((months, i) => ({ months, amount_rub: a[i] }))
  fx['/tariffs'] = { ...fx['/tariffs'], tariffs: [
    { id: 1, slug: 'base', name: 'Базовый', description: '2 устройства\nЛимит 200 ГБ в месяц', device_limit: 2, traffic_limit_bytes: 200 * GB, prices: prices([150, 400, 750, 1320]) },
    { id: 2, slug: 'standard', name: 'Стандарт', description: '3 устройства\nБезлимит на обычных серверах', device_limit: 3, traffic_limit_bytes: 0, bypass_traffic_limit_bytes: 30 * GB, bypass_label: 'обход', prices: prices([200, 510, 960, 1800]) },
    { id: 3, slug: 'premium', name: 'Премиум', is_popular: true, description: '5 устройств\nВсе серверы, обход можно докупить', device_limit: 5, traffic_limit_bytes: 0, bypass_traffic_limit_bytes: 50 * GB, bypass_label: 'обход', prices: prices([300, 810, 1500, 2700]) },
  ] }
  const { ctx, page, errors } = await open(1440, 1000, fx, '/tariffs')
  await page.mouse.move(2, 980)
  await page.waitForTimeout(500)
  await M(page.locator('.tariff-popular-badge >> visible=true').first(), 'Популярный тариф — отмечает админ', 'left')
  await M(page.getByText(/Безлимит \+ 30/).locator('visible=true').first(), 'Лимит обхода прямо на карточке', 'left')
  await shot(page, '1/05-tariffs-client.png')
  console.log('11', errors)
  globalThis.__clientTariffs = fx
  await ctx.close()
}
if (want(12)) {
  // 12. Те же тарифы на телефоне: с подпиской первым текущий, за ним популярный.
  const fx = globalThis.__clientTariffs
  const { ctx, page, errors } = await open(390, 844, fx, '/tariffs')
  await page.mouse.move(2, 840)
  await page.waitForTimeout(500)
  await shot(page, '1/06-tariffs-phone.png')
  console.log('12', errors)
  await ctx.close()
}
if (want(13)) {
  // 13. Редактор тарифа, «Основное»: тумблер «Популярный тариф» и предупреждение о переносе отметки.
  const fx = adminTariffs()
  const { ctx, page, errors } = await open(1440, 1100, fx, '/admin/tariffs')
  await page.locator('[role="button"]').filter({ hasText: 'Стандарт' }).first().click()
  await page.waitForTimeout(700)
  const row = page.getByText('Популярный тариф', { exact: true }).first()
  await row.scrollIntoViewIfNeeded()
  await row.locator('xpath=ancestor::*[.//*[@role="switch"] or .//input[@type="checkbox"]][1]').locator('[role="switch"], input[type="checkbox"]').first().click().catch((e) => console.log('toggle', String(e).slice(0, 90)))
  await page.waitForTimeout(500)
  await page.mouse.move(2, 1080)
  await M(row.locator('xpath=ancestor::*[.//*[contains(text(), "перейдёт")]][1]'), 'Один популярный тариф на витрине', 'left')
  await shot(page, '1/07-admin-popular.png')
  console.log('13', errors)
  await ctx.close()
}
if (want(14)) {
  // 14. Рассылка: картинку можно вставить из буфера.
  const fx = scene('admin-broadcast.json')
  const { ctx, page, errors } = await open(1440, 1000, fx, '/admin/broadcast')
  await page.mouse.move(2, 980)
  await M(page.getByText(/Ctrl\+V/).first(), 'Скриншот — прямо из буфера', 'top')
  await shot(page, '2/05-admin-broadcast.png')
  console.log('14', errors)
  await ctx.close()
}
await b.close()
server.close()
