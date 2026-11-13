// Снимок одного элемента html-макета: node shot-el.mjs <файл.html> <селектор> <выход.png> [масштаб]
import { chromium } from 'playwright'
import { pathToFileURL } from 'node:url'
import path from 'node:path'

const [file, selector, out, scale = '3'] = process.argv.slice(2)
const browser = await chromium.launch()
const page = await browser.newPage({ viewport: { width: 1300, height: 900 }, deviceScaleFactor: Number(scale) })
await page.goto(pathToFileURL(path.resolve(file)).href)
await page.waitForTimeout(300)
await page.locator(selector).first().screenshot({ path: out })
await browser.close()
console.log(out)
