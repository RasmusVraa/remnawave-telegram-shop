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
//     tools/cabinet-visual-check/fixtures/release-next.json release-screenshots
//
// Под новый релиз меняются только сценарии в конце файла: какую страницу
// открыть, что нажать и что обвести. Здесь — сценарии предрелиза после 5.27.0
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
const shot = (page, name) => page.screenshot({ path: path.join(outDir, name.replace('.png', '.jpg')), type: 'jpeg', quality: 84 })

// Сцена со своими данными поверх общей фикстуры.
const withSub = (patch) => ({ ...base, '/me/subscription': { ...base['/me/subscription'], ...patch } })


const fxDir = path.dirname(fixturesFile)
const loadFx = (name) => JSON.parse(fs.readFileSync(path.join(fxDir, name), 'utf8'))
const scene = (name, extra = {}) => ({ ...base, ...loadFx(name), ...extra })

{
  // 1. Тарифы в админке на ПК: карточка с цветной шапкой, лимитом обхода и точкой докупки.
  const fx = scene('release-next-tariffs.json')
  const { ctx, page, errors } = await open(1440, 900, fx, '/admin/tariffs')
  await page.mouse.move(2, 880)
  await page.waitForTimeout(400)
  await mark(page.locator('[role="button"]').filter({ hasText: 'Стандарт' }).first().locator('span', { hasText: '+30' }).first(), '', 'bottom')
  await caption(page, 'Лимит обхода и докупка — прямо на карточке', 78)
  await shot(page, '01-admin-tariffs-pc.png')
  console.log('1', errors)
  await ctx.close()
}
{
  // 2. Те же тарифы на телефоне: две плитки в ряд, цена «от».
  const fx = scene('release-next-tariffs.json')
  const { ctx, page, errors } = await open(390, 844, fx, '/admin/tariffs')
  // подпись — в свободной клетке справа от последней плитки
  await page.evaluate(() => {
    const tag = document.createElement('div')
    tag.innerHTML = 'Плитки: два тарифа в ряд,<br>цена «от» за месяц'
    Object.assign(tag.style, { position: 'fixed', left: '208px', top: '540px', width: '160px', background: '#FFB020', color: '#1d1400', font: '800 15px/1.25 Manrope, Inter, system-ui, sans-serif', padding: '10px 12px', borderRadius: '12px', zIndex: 2147483647, boxShadow: '0 6px 18px rgba(0,0,0,.35)' })
    document.body.appendChild(tag)
  })
  await shot(page, '02-admin-tariffs-phone.png')
  console.log('2', errors)
  await ctx.close()
}
{
  // 3. Редактор тарифа: вкладка «Трафик», кнопка «Удалить» внутри.
  const fx = scene('release-next-tariffs.json')
  const { ctx, page, errors } = await open(1440, 1000, fx, '/admin/tariffs')
  await page.locator('[role="button"]').filter({ hasText: 'Стандарт' }).first().click()
  await page.waitForTimeout(600)
  await page.getByRole('tab', { name: 'Трафик' }).click().catch(() => page.getByText('Трафик', { exact: true }).first().click())
  await page.waitForTimeout(900)
  await page.mouse.move(2, 980)
  await mark(page.getByRole('button', { name: 'Удалить' }).last(), 'Удаление теперь здесь', 'top')
  await shot(page, '03-admin-tariff-editor.png')
  console.log('3', errors)
  await ctx.close()
}
{
  // 4. Статус серверов: один столбик — один замер.
  const fx = scene('release-next-status.json')
  const { ctx, page, errors } = await open(1440, 1000, fx, '/status', '')
  await page.mouse.move(2, 900)
  await scrollToBlock(page, page.getByText('Локация', { exact: true }), 140)
  await page.waitForTimeout(1200)
  await mark(page.locator('li').filter({ hasText: 'Нидерланды' }).first().locator('.landing-status-bar').first(), 'Столбик = замер', 'bottom')
  await shot(page, '04-status-bars.png')
  console.log('4', errors)
  await ctx.close()
}
{
  // 5. Главная: кольца со скруглёнными концами.
  const fx = scene('split-ok.json')
  const { ctx, page, errors } = await open(1440, 900, fx, '/dashboard')
  await page.mouse.move(2, 880)
  await caption(page, 'Кольца: обычный трафик и обход, скруглённые концы', 150)
  await shot(page, '05-home-rings.png')
  console.log('5', errors)
  await ctx.close()
}
{
  // 5б. Шторка расхода трафика: обычный трафик и обход.
  const fx = scene('split-both.json')
  const { ctx, page, errors } = await open(1440, 900, fx, '/dashboard')
  await page.mouse.move(2, 880)
  await page.getByText(/ГБ/).filter({ hasText: '/' }).first().click().catch((e) => console.log('usage', String(e).slice(0, 90)))
  await page.waitForTimeout(1200)
  await shot(page, '05b-traffic-usage.png')
  console.log('5b', errors)
  await ctx.close()
}
{
  // 6. Докупка трафика: цена пакета со скидкой.
  const fx = scene('split-pack-discount.json')
  const { ctx, page, errors } = await open(1440, 900, fx, '/dashboard')
  const buy = page.getByRole('button', { name: /Докупить|Купить трафик|трафик/i }).first()
  console.log('buy-count', await page.getByRole('button', { name: /Докупить/i }).count())
  await buy.click().catch((e) => console.log('click', String(e).slice(0, 80)))
  await page.waitForTimeout(900)
  await caption(page, 'Цена за ГБ и скидка за крупный пакет', 170)
  await shot(page, '06-traffic-pack.png')
  console.log('6', errors)
  await ctx.close()
}
{
  // 7. Пользователи в админке: фильтр статуса в шапке таблицы.
  const fx = scene('admin-users-list.json')
  const { ctx, page, errors } = await open(1440, 900, fx, '/admin/users')
  await page.locator('th').filter({ hasText: 'Статус' }).locator('button').last().click().catch((e) => console.log('filter', String(e).slice(0, 80)))
  await page.waitForTimeout(600)
  await caption(page, 'Фильтр статуса — выпадающим списком в шапке', 800)
  await shot(page, '07-admin-users.png')
  console.log('7', errors)
  await ctx.close()
}
await b.close()
server.close()
