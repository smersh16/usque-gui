"use strict";
// Двуязычный интерфейс. Ключ — русский текст (он же язык по умолчанию),
// значение — английский. Нет перевода — показывается русский оригинал.

const EN = {
  // шапка, вход
  "Выйти": "Log out",
  "Вход": "Sign in",
  "Войти": "Sign in",
  "Задайте пароль": "Set a password",
  "Первый запуск: придумайте пароль для этого интерфейса (от 6 символов).": "First run: choose a password for this interface (6+ characters).",
  "Пароль": "Password",
  "Повторите пароль": "Repeat password",
  "Пароли не совпадают": "Passwords do not match",
  // состояние
  "Состояние": "Status",
  "Работает": "Running",
  "Проблема": "Problem",
  "Остановлен": "Stopped",
  "Настройки изменены — перезапустите сервис, чтобы они вступили в силу.": "Settings changed — restart the service to apply them.",
  "Не найден init-скрипт usque-keenetic (S51usque). Установите пакет usque-keenetic.": "usque-keenetic init script (S51usque) not found. Install the usque-keenetic package.",
  "Не найден бинарник usque.": "usque binary not found.",
  "Процесс": "Process",
  "запущен (PID {pid})": "running (PID {pid})",
  "не запущен": "not running",
  "Интерфейс {name}": "Interface {name}",
  "отсутствует в системе": "missing in the system",
  "поднят": "up",
  "опущен": "down",
  "есть": "present",
  "NDMS {name}": "NDMS {name}",
  "интерфейс не найден": "interface not found",
  "NDMS недоступен": "NDMS unavailable",
  "Адрес": "Address",
  "Трафик": "Traffic",
  "Аккаунт WARP": "WARP account",
  "зарегистрирован": "registered",
  "не зарегистрирован": "not registered",
  "Запустить": "Start",
  "Остановить": "Stop",
  "Перезапустить": "Restart",
  "Проверить выход": "Check exit",
  "Запрос к Cloudflare через интерфейс usque (экспериментально)": "Request to Cloudflare through the usque interface (experimental)",
  "Проверяю…": "Checking…",
  "Не удалось: {err}": "Failed: {err}",
  "Внешний IP: {ip}": "External IP: {ip}",
  // единицы
  "Б": "B", "КБ": "KB", "МБ": "MB", "ГБ": "GB", "ТБ": "TB",
  "/с": "/s",
  // настройки usque
  "Настройки usque": "usque settings",
  "Интерфейс": "Interface",
  "должен начинаться с opkgtun": "must start with opkgtun",
  "домен для маскировки трафика, можно пусто": "domain used to disguise traffic, may be empty",
  "IP интерфейса": "Interface IP",
  "авто (172.16.1.100–200)": "auto (172.16.1.100–200)",
  "пусто — выбрать свободный автоматически": "empty — pick a free one automatically",
  "Маска": "Netmask",
  "пусто — 255.255.255.255": "empty — 255.255.255.255",
  "Использовать HTTP/2": "Use HTTP/2",
  "Сохранить": "Save",
  "Настройки сохранены": "Settings saved",
  // аккаунт
  "Зарегистрировать": "Register",
  "Перерегистрировать": "Re-register",
  "Сбросить сессию": "Reset session",
  "Аккаунт WARP зарегистрирован, сессия сохранена.": "WARP account is registered, session is saved.",
  "Сессии нет. Она создаётся автоматически при первом запуске или кнопкой ниже.": "No session yet. It is created automatically on first start or with the button below.",
  "«Перерегистрировать» и «Сбросить» заменяют ключи устройства; запущенный сервис будет остановлен (после перерегистрации — запущен снова).": "“Re-register” and “Reset” replace the device keys; a running service will be stopped (and started again after re-registration).",
  "Заменить аккаунт WARP новым?": "Replace the WARP account with a new one?",
  "Сервис будет перезапущен.": "The service will be restarted.",
  "Сервис будет остановлен.": "The service will be stopped.",
  "Удалить сессию WARP?": "Delete the WARP session?",
  "При следующем запуске usque создаст новый аккаунт.": "usque will create a new account on the next start.",
  "Регистрация": "Registration",
  "Перерегистрация": "Re-registration",
  "Сброс сессии": "Session reset",
  "Редактировать конфигурацию сессии": "Edit session configuration",
  "Секретные поля (private_key, access_token) скрыты: оставьте пустыми, чтобы не менять, или введите новое значение. Перед записью делается копия": "Secret fields (private_key, access_token) are hidden: leave them empty to keep, or enter a new value. A backup copy is made before writing:",
  "для HTTP/2": "for HTTP/2",
  "адрес, выданный аккаунту": "address assigned to the account",
  "идентификатор устройства": "device identifier",
  "Сохранить сессию": "Save session",
  "Сессия сохранена": "Session saved",
  "•••••••• (задан, не менять)": "•••••••• (set, unchanged)",
  "не задан": "not set",
  "Файл сессии содержит ключи устройства; секретные значения никогда не передаются в браузер.": "The session file holds device keys; secret values are never sent to the browser.",
  // подбор endpoint
  "Подбор endpoint": "Endpoint finder",
  "Проверка доступности адресов Cloudflare с роутера (TCP + TLS на порту 443). Адреса ниже — предположительные из диапазонов Cloudflare WARP, не гарантированные. usque работает по QUIC (UDP), поэтому замер ориентировочный: он показывает достижимость и задержку, но не гарантирует работу MASQUE. Замер идёт по обычному маршруту роутера.": "Checks Cloudflare addresses from the router (TCP + TLS on port 443). The addresses below are guesses from Cloudflare WARP ranges, not guaranteed. usque uses QUIC (UDP), so the result is indicative: it shows reachability and latency but does not guarantee MASQUE works. Probing uses the router’s normal route.",
  "Адреса (по одному в строке, до 32)": "Addresses (one per line, up to 32)",
  "Проверить": "Probe",
  "Проверяю адреса…": "Probing addresses…",
  "Задержка": "Latency",
  "TLS": "TLS",
  "да": "yes",
  "нет": "no",
  "Применить": "Apply",
  "также для HTTP/2": "also for HTTP/2",
  "Значение подставлено в форму сессии — проверьте и нажмите «Сохранить сессию».": "Value placed into the session form — review it and press “Save session”.",
  "Нет ответивших адресов": "No address responded",
  // версии
  "Версии и обновления": "Versions and updates",
  "Проверить обновления": "Check for updates",
  "установлена: {v}": "installed: {v}",
  "неизвестно": "unknown",
  "бинарник usque {v}": "usque binary {v}",
  "доступна: {v}": "available: {v}",
  "есть обновление": "update available",
  "Обновить": "Update",
  "актуальная": "up to date",
  "Обновить {name} до {v}? Сервис будет перезапущен.": "Update {name} to {v}? The service will be restarted.",
  "Обновление {name}": "Updating {name}",
  "Проверено: {d}.": "Checked: {d}.",
  "Ещё не проверялось.": "Not checked yet.",
  "Проверяю (opkg update)…": "Checking (opkg update)…",
  // резервная копия
  "Резервная копия": "Backup",
  "Копия содержит настройки usque и, по желанию, сессию WARP. Файл с сессией содержит секретные ключи — храните его как пароль.": "The backup contains usque settings and, optionally, the WARP session. A file with the session contains secret keys — store it like a password.",
  "Включить сессию WARP (секретные ключи)": "Include the WARP session (secret keys)",
  "Пароль интерфейса (нужен для копии и восстановления сессии)": "Interface password (required for backing up or restoring the session)",
  "Скачать копию": "Download backup",
  "Восстановление": "Restore",
  "Файл копии": "Backup file",
  "Восстановить настройки usque": "Restore usque settings",
  "Восстановить сессию WARP": "Restore the WARP session",
  "Восстановить": "Restore",
  "Выберите файл копии": "Choose a backup file",
  "Файл не является корректным JSON": "The file is not valid JSON",
  "Восстановить из копии? Текущие настройки будут заменены (резервные копии файлов сохраняются рядом).": "Restore from the backup? Current settings will be replaced (backup copies of the files are kept next to them).",
  "Копия скачана": "Backup downloaded",
  "Восстановлено": "Restored",
  "Скачивание": "Downloading",
  "Восстановление из копии": "Restoring from backup",
  // журнал
  "Журнал действий": "Action log",
  "Обновить журнал": "Refresh log",
  "У самого usque журнала нет (init-скрипт не сохраняет вывод), здесь — результаты действий из этого интерфейса.": "usque itself has no log (the init script does not keep its output); this shows results of actions taken from this interface.",
  "Пока пусто.": "Empty so far.",
  "ок": "ok",
  "ошибка": "error",
  // интерфейс
  "Настройки интерфейса": "Interface settings",
  "Язык": "Language",
  "Адрес и порт веб-интерфейса": "Web interface address and port",
  "0.0.0.0 — все интерфейсы; IP роутера — только эта сеть. Применяется после перезапуска интерфейса.": "0.0.0.0 — all interfaces; the router’s IP — that network only. Applied after restarting the interface.",
  "Адрес сохранён. Перезапустите интерфейс, чтобы применить.": "Address saved. Restart the interface to apply it.",
  "Перезапустить интерфейс": "Restart interface",
  "Перезапуск интерфейса… страница откроется по новому адресу.": "Restarting the interface… the page will reopen at the new address.",
  "Пароль интерфейса": "Interface password",
  "Текущий": "Current",
  "Новый (от 6 символов)": "New (6+ characters)",
  "Сменить": "Change",
  "Пароль изменён": "Password changed",
  // общие
  "Готово": "Done",
  "{label}…": "{label}…",
  "Ошибка {code}": "Error {code}",
  "Запуск": "Starting",
  "Остановка": "Stopping",
  "Перезапуск": "Restarting",
  // серверные сообщения
  "требуется вход": "sign-in required",
  "доступ разрешён только из локальной сети": "access is allowed from the local network only",
  "некорректный запрос": "invalid request",
  "пароль уже задан": "password is already set",
  "пароль слишком короткий (минимум 6 символов)": "password is too short (minimum 6 characters)",
  "неверный пароль": "wrong password",
  "слишком много попыток, подождите минуту": "too many attempts, wait a minute",
  "текущий пароль неверен": "current password is wrong",
  "другая операция ещё выполняется": "another operation is still running",
  "неизвестное действие": "unknown action",
  "init-скрипт usque-keenetic не найден": "usque-keenetic init script not found",
  "бинарник usque не найден": "usque binary not found",
  "сессия уже существует; воспользуйтесь перерегистрацией": "a session already exists; use re-registration",
  "регистрация завершилась, но файл сессии не создан": "registration finished but the session file was not created",
  "не удалось остановить сервис": "failed to stop the service",
  "аккаунт создан, но сервис не запустился": "account created but the service did not start",
  "сессии нет: сначала зарегистрируйте аккаунт": "no session: register an account first",
  "private_key не может быть пустым": "private_key must not be empty",
  "интерфейс должен иметь вид opkgtunN (например, opkgtun0)": "interface must look like opkgtunN (e.g. opkgtun0)",
  "IP-адрес интерфейса: ожидается IPv4, например 172.16.1.100": "interface IP: an IPv4 address is expected, e.g. 172.16.1.100",
  "маска: ожидается IPv4-маска, например 255.255.255.255": "netmask: an IPv4 mask is expected, e.g. 255.255.255.255",
  "SNI: допустимы только латиница, цифры, точка и дефис": "SNI: only Latin letters, digits, dot and hyphen are allowed",
  "не IP-адрес": "not an IP address",
  "адрес не публичный": "address is not public",
  "список адресов пуст": "address list is empty",
  "слишком много адресов (максимум 32)": "too many addresses (maximum 32)",
  "это не резервная копия usque-gui": "this is not a usque-gui backup",
  "в копии нет настроек usque": "the backup has no usque settings",
  "в копии нет сессии WARP (копия сделана без неё)": "the backup has no WARP session (it was made without it)",
  "в копии нет private_key": "the backup has no private_key",
  "в копии маска вместо секрета": "the backup has a mask instead of a secret",
  "ничего не выбрано для восстановления": "nothing selected to restore",
  "для копии с сессией нужен верный пароль": "the correct password is required for a backup with the session",
  "для восстановления сессии нужен верный пароль": "the correct password is required to restore the session",
  "usque.conf не найден: установите usque-keenetic": "usque.conf not found: install usque-keenetic",
  "адрес должен быть вида 0.0.0.0:2223 или 192.168.1.1:2223": "address must look like 0.0.0.0:2223 or 192.168.1.1:2223",
  "адрес должен быть IP-адресом (или пустым — все интерфейсы)": "host must be an IP address (or empty for all interfaces)",
  "порт должен быть от 1 до 65535": "port must be between 1 and 65535",
  "перезапуск недоступен": "restart is unavailable",
  "из интерфейса можно обновить только usque-keenetic": "only usque-keenetic can be updated from the interface",
  "opkg не найден (нужен Entware)": "opkg not found (Entware is required)",
  "остановите сервис перед сбросом сессии": "stop the service before resetting the session",
  "таймаут": "timeout",
  "отказ в соединении": "connection refused",
  "сеть недоступна": "network unreachable",
  "неожиданный ответ Cloudflare": "unexpected Cloudflare response",
  "сервис не запущен или интерфейс отсутствует": "the service is not running or the interface is missing",
  // журнал (сообщения сервера)
  "usque.conf сохранён": "usque.conf saved",
  "session.conf удалён": "session.conf removed",
  "проверка обновлений выполнена": "update check completed",
  "перезапуск интерфейса": "interface restart",
};

// Сообщения с переменной частью.
const EN_RX = [
  [/^превышено время ожидания \((.+)\)$/, "timed out ({1})"],
  [/^не удаётся занять (.+?): (.+)$/, "cannot bind {1}: {2}"],
  [/^не удалось прочитать (.+?): (.+)$/, "failed to read {1}: {2}"],
  [/^неподдерживаемая версия копии: (.+)$/, "unsupported backup version: {1}"],
  [/^(.+): ожидается IPv4-адрес$/, "{1}: an IPv4 address is expected"],
  [/^(.+): ожидается IPv6-адрес$/, "{1}: an IPv6 address is expected"],
  [/^(.+): ожидается base64$/, "{1}: base64 is expected"],
  [/^(.+): ожидается PEM-блок.*$/, "{1}: a PEM block is expected"],
  [/^(.+): недопустимое значение$/, "{1}: invalid value"],
  [/^session\.conf обновлён \(копия: (.+)\)$/, "session.conf updated (backup: {1})"],
  [/^адрес интерфейса: (.+) \(применится после перезапуска GUI\)$/, "interface address: {1} (applies after the GUI restarts)"],
  [/^восстановлено: настройки=(\w+), сессия=(\w+) \(копия от (.+)\)$/, "restored: settings={1}, session={2} (backup from {3})"],
  [/^создана копия \(с сессией: (\w+)\)$/, "backup created (with session: {1})"],
];

let LANG = "ru";
try {
  const saved = localStorage.getItem("usque_lang");
  if (saved === "en" || saved === "ru") LANG = saved;
} catch (_) { /* хранилище недоступно */ }

function setLang(l) {
  LANG = l === "en" ? "en" : "ru";
  try { localStorage.setItem("usque_lang", LANG); } catch (_) { /* ignore */ }
  document.documentElement.lang = LANG;
}

function fill(s, vars) {
  if (!vars) return s;
  return s.replace(/\{(\w+)\}/g, (_, k) => (k in vars ? vars[k] : "{" + k + "}"));
}

// t — перевод интерфейсной строки (ключ — русский текст, {имя} — подстановки).
function t(key, vars) {
  const s = LANG === "en" && EN[key] !== undefined ? EN[key] : key;
  return fill(s, vars);
}

// tr — перевод сообщения, пришедшего с сервера (в т.ч. «заголовок: подробности»).
function tr(msg) {
  if (LANG !== "en" || !msg) return msg;
  if (EN[msg] !== undefined) return EN[msg];
  for (const [re, tpl] of EN_RX) {
    const m = msg.match(re);
    if (m) return tpl.replace(/\{(\d)\}/g, (_, i) => m[+i] ?? "");
  }
  const i = msg.indexOf(": ");
  if (i > 0 && EN[msg.slice(0, i)] !== undefined) return EN[msg.slice(0, i)] + ": " + tr(msg.slice(i + 2));
  return msg;
}

// applyI18n переводит статические элементы разметки.
//   data-i18n="" — текст элемента; data-i18n-ph — placeholder; data-i18n-title — title.
function applyI18n() {
  document.querySelectorAll("[data-i18n]").forEach((el) => {
    if (el.dataset.k === undefined) el.dataset.k = el.textContent.replace(/\s+/g, " ").trim();
    el.textContent = t(el.dataset.k);
  });
  document.querySelectorAll("[data-i18n-ph]").forEach((el) => {
    if (el.dataset.kph === undefined) el.dataset.kph = el.getAttribute("placeholder") || "";
    el.setAttribute("placeholder", t(el.dataset.kph));
  });
  document.querySelectorAll("[data-i18n-title]").forEach((el) => {
    if (el.dataset.kt === undefined) el.dataset.kt = el.getAttribute("title") || "";
    el.setAttribute("title", t(el.dataset.kt));
  });
}
