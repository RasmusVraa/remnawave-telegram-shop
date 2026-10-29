---
name: cabinet-dev-server
description: >-
  Собирает React-кабинет, поднимает локальный dev-сервер с mock-API,
  открывает в браузере. Даёт быструю итерацию при разработке фронтенда.
  Альтернатива ручному npm run dev.
---

# Dev-сервер кабинета

При правке фронтенда нужно видеть результат в браузере в реальном времени.
Этот скилл автоматизирует подъём окружения.

## Когда применять

- Разработка компонентов кабинета (`web/cabinet/src/components`).
- Правка страниц, навигации, локализации.
- Тестирование новых фич перед git push.

## Порядок

### 1. Проверить зависимости

```bash
cd web/cabinet
npm install --legacy-peer-deps  # если нужно
npm list react react-dom react-router-dom  # проверить основные
```

### 2. Запустить dev-сервер

```bash
cd web/cabinet

# Стандартный запуск
npm run dev

# С конкретным портом (если 5173 занят)
VITE_PORT=5174 npm run dev

# Только сборка без сервера (для CI/CD)
npm run build

# Проверка типов
npm run typecheck
```

Вывод должен быть вроде:

```
  VITE v... dev server running at:

  > Local:    http://localhost:5173/
  > press h + enter to show help
```

### 3. Открыть в браузере

Перейти на http://localhost:5173/cabinet/subscription

Проверить:
- Компоненты рендерятся без ошибок в консоли
- Стили загружены (не видно белого текста на белом фоне)
- Интерактивность работает (кнопки, инпуты, навигация)

### 4. Горячая перезагрузка

Vite поддерживает HMR (Hot Module Replacement):
- Правка TypeScript/React файла → автоматически обновляется в браузере
- Стили (CSS) обновляются без перезагрузки страницы
- Иногда нужна ручная перезагрузка (`Ctrl+R`)

### 5. Отладка

**DevTools консоль:**
- Смотреть console.log() и ошибки
- Смотреть Network tab, какие запросы идут
- Смотреть React DevTools (если установлено расширение)

**Типичные ошибки:**
- `Module not found: Can't resolve '@/...'` → алиас `@/` не работает, проверить `vite.config.ts`
- `[vue-tsc] Type error...` → ошибка типов TypeScript, нужно чинить
- `CORS error` → если обращаетесь к реальному API вместо мока
- `Failed to parse sourcemap` → некритично, только при отладке

### 6. Контрольный список

- [ ] `npm install` прошёл без ошибок
- [ ] `npm run dev` запустился без паник
- [ ] Dev-сервер доступен на http://localhost:5173
- [ ] Компоненты рендеринги без console.error
- [ ] Стили загружены правильно
- [ ] HMR работает (правка файла обновляет браузер)
- [ ] TypeScript-ошибок нет (`npm run typecheck`)

### 7. Типичный workflow

```bash
# 1. Поднять сервер
npm run dev

# 2. В другом терминале: правка кода
# (осуществляется в IDE, hot-reload срабатывает автоматически)

# 3. Проверить типы перед коммитом
npm run typecheck

# 4. Собрать продакшн-бундл
npm run build

# 5. Выключить сервер
# Ctrl+C
```

### 8. Если что-то не работает

```bash
# Полная переустановка
rm -rf node_modules package-lock.json
npm install --legacy-peer-deps

# Очистка кэша Vite
rm -rf .vite

# Проверить версии
npm list react react-router-dom react-i18next

# Перезагрузить браузер
# Hard refresh: Ctrl+Shift+R (или Cmd+Shift+R на Mac)
```

## Notes

- Dev-сервер использует эмулированный API из `tools/cabinet-visual-check/fixtures/`
  (если запускать через capture.mjs). При `npm run dev` напрямую может ловить CORS.
- Порт 5173 — стандартный для Vite. Если занят, используйте `VITE_PORT=...`
- На продакшене кабинет собирается в `dist/` и раздаётся Go-сервером как статика.
