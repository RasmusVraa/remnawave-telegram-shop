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
const shot = (page, name) => page.screenshot({ path: path.join(outDir, name) })

// Сцена со своими данными поверх общей фикстуры.
const withSub = (patch) => ({ ...base, '/me/subscription': { ...base['/me/subscription'], ...patch } })

{
  // Публичная страница без входа: карта и таблица, проблемные серверы подсвечены.
  const { ctx, page, errors } = await open(1440, 1000, base, '/status', '')
  await page.mouse.move(2, 900)
  await scrollToBlock(page, page.getByText('Локация', { exact: true }), 140)
  await page.waitForTimeout(1500)
  await mark(page.locator('li').filter({ hasText: 'США' }).first().getByText('Недоступен', { exact: true }).first(), 'Проблемный сервер виден сразу', 'top')
  await shot(page, '01-status-public.png')
  console.log('1', errors)
  await ctx.close()
}
{
  // Страница статуса внутри кабинета (ПК).
  const { ctx, page, errors } = await open(1440, 900, base, '/status')
  await mark(page.locator('.cabinet-status-page').locator('div').filter({ hasText: 'Недоступен' }).filter({ hasText: 'США' }).last(), '', 'top')
  await caption(page, 'Что не работает — подсвечено', 208)
  await shot(page, '02-status-cabinet.png')
  console.log('2', errors)
  await ctx.close()
}
{
  // То же на телефоне.
  const { ctx, page, errors } = await open(390, 844, base, '/status')
  await shot(page, '03-status-phone.png')
  console.log('3', errors)
  await ctx.close()
}
{
  // Вкладка «Статус» на странице поддержки — живая сводка вместо кнопки.
  const { ctx, page, errors } = await open(1440, 1100, base, '/support')
  await page.getByRole('button', { name: 'Статус', exact: true }).click()
  await page.mouse.move(2, 1000)
  await page.waitForTimeout(600)
  await scrollToBlock(page, page.getByText('Статус серверов').first(), 140)
  await mark(page.locator('ul.divide-y').first(), 'Сводка прямо в поддержке', 'left')
  await shot(page, '04-status-support.png')
  console.log('4', errors)
  await ctx.close()
}
{
  // Админка: список серверов, откуда берутся данные, настройки страницы.
  const { ctx, page, errors } = await open(1440, 1200, base, '/admin/status')
  await mark(page.getByRole('button', { name: 'Добавить сервер' }), 'Серверы из панели или вручную', 'top')
  await shot(page, '05-admin-status.png')
  console.log('5', errors)
  await ctx.close()
}
{
  // Окно сервера обхода: хост для проверки, вход и зарубежный выход каскада.
  const { ctx, page, errors } = await open(1440, 1100, base, '/admin/status')
  await page.getByRole('button', { name: 'Изменить' }).nth(1).click()
  await page.waitForTimeout(700)
  await page.mouse.move(2, 1000)
  await mark(page.getByText('Хост для проверки', { exact: true }), 'Сервер обхода: хост и выход каскада', 'left')
  await shot(page, '06-admin-bypass.png')
  console.log('6', errors)
  await ctx.close()
}
await b.close()
server.close()
