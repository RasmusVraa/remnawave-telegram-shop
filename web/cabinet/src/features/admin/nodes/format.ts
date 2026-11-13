const BYTE_UNITS = ['B', 'KiB', 'MiB', 'GiB', 'TiB', 'PiB']

/** 652.59 GiB — как в панели: степени 1024, два знака после точки. */
export function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return '0 B'
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), BYTE_UNITS.length - 1)
  const v = bytes / 1024 ** i
  return `${i === 0 ? v.toFixed(0) : v.toFixed(2)} ${BYTE_UNITS[i]}`
}

const RATE_UNITS = ['b/s', 'Kb/s', 'Mb/s', 'Gb/s', 'Tb/s']

/**
 * Скорость интерфейса. Панель отдаёт байты в секунду, а канал принято мерить в битах:
 * умножаем на 8 и считаем степенями 1000 (14.37 Mb/s).
 */
export function formatBitrate(bytesPerSec: number): string {
  const bits = bytesPerSec * 8
  if (!Number.isFinite(bits) || bits <= 0) return '0.00 Kb/s'
  const i = Math.min(Math.max(Math.floor(Math.log10(bits) / 3), 1), RATE_UNITS.length - 1)
  return `${(bits / 1000 ** i).toFixed(2)} ${RATE_UNITS[i]}`
}

/** Разбор скорости на число и единицу — для карточек сводки, где единица рисуется мельче. */
export function splitBitrate(bytesPerSec: number): { value: string; unit: string } {
  const [value, unit] = formatBitrate(bytesPerSec).split(' ')
  return { value, unit }
}

/** 4d / 13h / 12m — самая крупная единица, как в панели. */
export function formatUptime(sec: number): string {
  if (!Number.isFinite(sec) || sec <= 0) return '—'
  if (sec >= 86400) return `${Math.floor(sec / 86400)}d`
  if (sec >= 3600) return `${Math.floor(sec / 3600)}h`
  if (sec >= 60) return `${Math.floor(sec / 60)}m`
  return '<1m'
}
