import { createLucideIcon } from 'lucide-react'

/*
 * Иконка раздела «Колесо фортуны»: обод во всю сетку, 6 спиц, через сектор
 * полупрозрачная заливка тем же цветом и маленькая стрелка внутри верхнего
 * (незалитого) сектора. Рисуется как обычная иконка lucide — цвет, размер и
 * толщина линии берутся снаружи. Стрелка внутри, а не над ободом, и без
 * кружка в центре: иначе на 24 px колесо мельче соседних иконок и сливается
 * в сплошной диск.
 */
export const FortuneWheelIcon = createLucideIcon('FortuneWheel', [
  ['path', { d: 'M12 12L17 20.66A10 10 0 0 1 7 20.66Z', fill: 'currentColor', fillOpacity: '0.28', stroke: 'none', key: 's1' }],
  ['path', { d: 'M12 12L2 12A10 10 0 0 1 7 3.34Z', fill: 'currentColor', fillOpacity: '0.28', stroke: 'none', key: 's2' }],
  ['path', { d: 'M12 12L17 3.34A10 10 0 0 1 22 12Z', fill: 'currentColor', fillOpacity: '0.28', stroke: 'none', key: 's3' }],
  ['circle', { cx: '12', cy: '12', r: '10', key: 'rim' }],
  ['path', { d: 'M2 12h20M7 3.34l10 17.32M17 3.34l-10 17.32', key: 'spokes' }],
  ['path', { d: 'M10.5 4.6h3L12 7.2z', fill: 'currentColor', key: 'pointer' }],
])
