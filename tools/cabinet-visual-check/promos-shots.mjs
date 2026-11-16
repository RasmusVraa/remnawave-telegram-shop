// Снимки страницы «Промокоды» в админке: список, фильтры, меню, все окна — на ПК,
// узком ПК и телефоне, в тёмной/светлой теме и цветных декор-темах.
//
//   cd web/cabinet && npm run build && cd ../..
//   node tools/cabinet-visual-check/make-promos-fixture.cjs
//   node tools/cabinet-visual-check/promos-shots.mjs [out] [themes=dark,light] [decor=off]
//
// decor — id декор-темы кабинета (pink, green, orange…): так проверяется, что акцент
// страницы берётся из темы. Данные — fixtures/admin-promos.json (выдуманные).
import http from 'node:http'
import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { chromium } from 'playwright'

const here = path.dirname(fileURLToPath(import.meta.url))
const root = path.resolve(here, '../..')
const distDir = path.join(root, 'internal/cabinet/web/dist')
const outDir = path.resolve(process.argv[2] ?? path.join(root, '.tmp/promos-shots'))
const themes = (process.argv[3] ?? 'dark,light').split(',')
const decor = process.argv[4] ?? 'off'
fs.mkdirSync(outDir, { recursive: true })

const MIME = { '.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css', '.svg': 'image/svg+xml', '.woff2': 'font/woff2', '.png': 'image/png', '.webp': 'image/webp' }
const server = http.createServer((req, res) => {
  const url = (req.url ?? '/').split('?')[0]
  let file = path.join(distDir, decodeURIComponent(url).replace(/^\/cabinet/, ''))
  if (!fs.existsSync(file) || fs.statSync(file).isDirectory()) file = path.join(distDir, 'index.html')
  res.writeHead(200, { 'Content-Type': MIME[path.extname(file)] ?? 'application/octet-stream' })
  res.end(fs.readFileSync(file))
})
const PORT = 5611
await new Promise((r) => server.listen(PORT, r))

const fixtures = {
  '/auth/refresh': { access_token: 'stub', token_type: 'Bearer', expires_in: 3600 },
  '/auth/bootstrap': { decor_theme: decor },
  '/me': { id: 1, email: 'admin@example.com', email_verified: true, language: 'ru', providers: ['password'], has_telegram_link: true, has_password: true, google_oauth_enabled: false, is_admin: true, customer_id: 1 },
  ...JSON.parse(fs.readFileSync(path.join(here, 'fixtures/admin-promos.json'), 'utf8')),
}

const browser = await chromium.launch()
const errors = []

async function open(theme, width, height) {
  const ctx = await browser.newContext({ viewport: { width, height }, deviceScaleFactor: 2, colorScheme: theme, locale: 'ru-RU', timezoneId: 'Europe/Moscow' })
  await ctx.addInitScript((mode) => {
    try {
      localStorage.setItem('onboarding_completed', 'true')
      localStorage.setItem('cab_theme', mode)
    } catch {
      /* ignore */
    }
  }, theme)
  await ctx.route('**/cabinet/api/**', (route) => {
    const p = new URL(route.request().url()).pathname.replace('/cabinet/api', '')
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(fixtures[p] ?? {}) })
  })
  const page = await ctx.newPage()
  page.on('pageerror', (e) => errors.push(String(e)))
  page.on('console', (m) => m.type() === 'error' && errors.push(m.text()))
  await page.goto(`http://localhost:${PORT}/cabinet/admin/promos`, { waitUntil: 'networkidle' })
  await page.waitForTimeout(1200)
  return { ctx, page }
}

async function shot(page, name, full = false) {
  await page.mouse.move(2, 2)
  await page.waitForTimeout(350)
  const file = path.join(outDir, `${name}.png`)
  await page.screenshot({ path: file, fullPage: full })
  console.log(file)
}

const esc = async (page) => {
  await page.keyboard.press('Escape')
  await page.waitForTimeout(350)
}

for (const theme of themes) {
  const tag = decor === 'off' ? theme : `${theme}-${decor}`

  // ---------- ПК ----------
  {
    const { ctx, page } = await open(theme, 1440, 1000)
    await shot(page, `${tag}-pc-list`, true)
    await page.getByRole('button', { name: 'Фильтр и сортировка' }).click()
    await shot(page, `${tag}-pc-filter`)
    await esc(page)
    // Фильтр типа в шапке таблицы (как у «Пользователей»)
    await page.getByRole('button', { name: 'Тип', exact: true }).click()
    await shot(page, `${tag}-pc-head-type`)
    await esc(page)
    // Сортировка по заголовку «Использовано»
    await page.getByRole('row').first().getByRole('button', { name: /Использовано/ }).click()
    await page.waitForTimeout(500)
    await shot(page, `${tag}-pc-head-sort`)
    await page.getByRole('button', { name: 'Действия' }).first().click()
    await shot(page, `${tag}-pc-menu`)
    await esc(page)
    // Клик по строке — правка; меняем скидку, чтобы увидеть «было».
    // Клик по строке — подробности (один флоу с телефоном)
    await page.getByRole('row').filter({ hasText: 'CRYPTO' }).getByText('Скидка 15%').click()
    await page.waitForTimeout(600)
    await shot(page, `${tag}-pc-detail`)
    // Тап по плитке — правка сразу на этом поле
    await page.locator('.admin-modal-panel').getByRole('button', { name: 'Срок действия: Редактировать' }).click()
    await page.waitForTimeout(1000)
    await shot(page, `${tag}-pc-edit-focus`)
    await esc(page)
    await esc(page)
    // Правка — из меню «⋯»
    await page.getByRole('row').filter({ hasText: 'CRYPTO' }).getByRole('button', { name: 'Действия' }).click()
    await page.getByRole('menuitem', { name: 'Редактировать' }).click()
    await page.waitForTimeout(500)
    await page.getByRole('button', { name: '20%' }).click()
    await shot(page, `${tag}-pc-edit`)
    await esc(page)
    await shot(page, `${tag}-pc-discard`)
    await page.getByRole('button', { name: 'Закрыть', exact: true }).last().click()
    await page.waitForTimeout(400)
    // Клик по «Использовано» — кто использовал.
    await page.getByRole('row').filter({ hasText: 'HELL26' }).getByRole('button', { name: 'Кто использовал' }).click()
    await page.waitForTimeout(700)
    await shot(page, `${tag}-pc-used`)
    await esc(page)
    await page.getByRole('button', { name: 'Создать' }).first().click()
    await page.waitForTimeout(400)
    await page.getByRole('radio', { name: /Скидка/ }).click()
    await page.getByRole('button', { name: 'Сгенерировать код' }).click()
    await page.locator('input[inputmode="numeric"]').first().fill('15')
    await page.getByPlaceholder('Для кого и зачем — видно только в админке').fill('Весенняя рассылка')
    await shot(page, `${tag}-pc-create`)
    await page.getByRole('button', { name: 'Отмена' }).click()
    await page.waitForTimeout(300)
    await page.getByRole('button', { name: 'Закрыть', exact: true }).last().click()
    await page.waitForTimeout(300)
    await page.getByRole('row').filter({ hasText: 'CRYPTO' }).getByRole('button', { name: 'Действия' }).click()
    await page.getByRole('menuitem', { name: 'Удалить…' }).click()
    await page.waitForTimeout(700)
    await shot(page, `${tag}-pc-delete`)
    await ctx.close()
  }

  // ---------- узкий ПК: карточки в две колонки ----------
  {
    const { ctx, page } = await open(theme, 1100, 900)
    await shot(page, `${tag}-pc1100-list`)
    await ctx.close()
  }

  // ---------- телефон ----------
  for (const w of [390, 375]) {
    const { ctx, page } = await open(theme, w, 844)
    await shot(page, `${tag}-m${w}-list`, true)
    if (w === 375) {
      await ctx.close()
      continue
    }
    await page.getByRole('button', { name: 'Фильтр и сортировка' }).click()
    await page.waitForTimeout(500)
    await shot(page, `${tag}-m-filter`)
    await esc(page)
    await page.getByRole('button').filter({ hasText: 'MORNING' }).first().getByText('+7 дней').click()
    await page.waitForTimeout(700)
    await shot(page, `${tag}-m-detail`)
    await page.locator('.admin-modal-panel').getByRole('button', { name: 'Дней подписки: Редактировать' }).click()
    await page.waitForTimeout(1000)
    await shot(page, `${tag}-m-edit-focus`)
    await esc(page)
    await page.locator('.admin-modal-panel').getByRole('button', { name: 'Редактировать', exact: true }).click()
    await page.waitForTimeout(500)
    await page.locator('input[inputmode="numeric"]').first().fill('10')
    await shot(page, `${tag}-m-edit`)
    await esc(page)
    await page.getByRole('button', { name: 'Закрыть', exact: true }).last().click()
    await page.waitForTimeout(500)
    await page.locator('.admin-modal-panel').getByRole('button', { name: /^Кто использовал \d/ }).click()
    await page.waitForTimeout(700)
    await shot(page, `${tag}-m-used`)
    await esc(page)
    await esc(page)
    await page.getByRole('button', { name: 'Создать' }).first().click()
    await page.waitForTimeout(500)
    await shot(page, `${tag}-m-create`)
    await ctx.close()
  }
}

await browser.close()
server.close()
if (errors.length) {
  console.warn(`ошибок в консоли: ${errors.length}`)
  for (const e of [...new Set(errors)].slice(0, 8)) console.warn('  ' + e)
}
