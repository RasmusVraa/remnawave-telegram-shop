// Снимок локального html-макета: node shot-html.mjs <файл.html> <выход.png> [ширина] [масштаб]
import { chromium } from 'playwright'
import { pathToFileURL } from 'node:url'
import path from 'node:path'

const [file, out, width = '1300', scale = '1'] = process.argv.slice(2)
const browser = await chromium.launch()
const page = await browser.newPage({ viewport: { width: Number(width), height: 900 }, deviceScaleFactor: Number(scale) })
await page.goto(pathToFileURL(path.resolve(file)).href)
await page.waitForTimeout(300)
await page.screenshot({ path: out, fullPage: true })
await browser.close()
console.log(out)
