"use strict";

const $ = (id) => document.getElementById(id);
let busy = false;
let pollTimer = null;
let cfgDirty = false;

async function api(method, path, body) {
  const opts = { method, headers: { "X-Requested-With": "usque-gui" } };
  if (body !== undefined) {
    opts.headers["Content-Type"] = "application/json";
    opts.body = JSON.stringify(body);
  }
  const r = await fetch(path, opts);
  let data = {};
  try { data = await r.json(); } catch (_) { /* пустой ответ */ }
  if (r.status === 401 && path !== "/api/login") { showAuth(); }
  if (!r.ok) {
    const e = new Error(data.error || ("Ошибка " + r.status));
    e.output = data.output;
    throw e;
  }
  return data;
}

let toastTimer;
function toast(msg, bad) {
  const t = $("toast");
  t.textContent = msg;
  t.className = "toast" + (bad ? " bad" : "");
  t.hidden = false;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => { t.hidden = true; }, bad ? 6000 : 3000);
}

// ---------- авторизация ----------
async function boot() {
  try {
    const a = await api("GET", "/api/auth");
    $("ver").textContent = a.version || "";
    if (a.authenticated) showApp(); else showAuth(a.needSetup);
  } catch (e) {
    showAuth();
    $("auth-err").textContent = e.message;
  }
}

let setupMode = false;
async function showAuth(needSetup) {
  clearInterval(pollTimer);
  if (needSetup === undefined) {
    try { needSetup = (await api("GET", "/api/auth")).needSetup; } catch (_) { needSetup = false; }
  }
  setupMode = !!needSetup;
  $("view-app").hidden = true;
  $("logout").hidden = true;
  $("view-auth").hidden = false;
  $("auth-title").textContent = setupMode ? "Задайте пароль" : "Вход";
  $("auth-hint").textContent = setupMode ? "Первый запуск: придумайте пароль для этого интерфейса (от 6 символов)." : "";
  $("auth-pass2-wrap").hidden = !setupMode;
  $("auth-pass").autocomplete = setupMode ? "new-password" : "current-password";
}

$("auth-form").addEventListener("submit", async (ev) => {
  ev.preventDefault();
  $("auth-err").textContent = "";
  const pw = $("auth-pass").value;
  if (setupMode && pw !== $("auth-pass2").value) {
    $("auth-err").textContent = "Пароли не совпадают";
    return;
  }
  try {
    await api("POST", setupMode ? "/api/setup" : "/api/login", { password: pw });
    $("auth-pass").value = $("auth-pass2").value = "";
    showApp();
  } catch (e) {
    $("auth-err").textContent = e.message;
  }
});

$("logout").addEventListener("click", async () => {
  try { await api("POST", "/api/logout", {}); } catch (_) { /* ignore */ }
  showAuth(false);
});

// ---------- панель ----------
function showApp() {
  $("view-auth").hidden = true;
  $("view-app").hidden = false;
  $("logout").hidden = false;
  cfgDirty = false;
  loadConfig();
  refresh();
  loadLog();
  clearInterval(pollTimer);
  pollTimer = setInterval(() => { if (!busy && !document.hidden) refresh(); }, 5000);
}

const STATE_TEXT = { running: "Работает", degraded: "Проблема", stopped: "Остановлен" };

function fact(dl, label, value, dot) {
  const dt = document.createElement("dt");
  dt.textContent = label;
  const dd = document.createElement("dd");
  if (dot) {
    const s = document.createElement("span");
    s.className = "dot " + dot;
    dd.appendChild(s);
  }
  dd.appendChild(document.createTextNode(value));
  dl.append(dt, dd);
}

function renderStatus(res) {
  const st = res.status;
  const b = $("badge");
  b.className = "badge " + st.state;
  b.textContent = STATE_TEXT[st.state] || st.state;

  $("warn-restart").hidden = !res.restartPending;

  const missing = [];
  if (!st.initFound) missing.push("Не найден init-скрипт usque-keenetic (S51usque). Установите пакет usque-keenetic.");
  else if (!st.binaryFound) missing.push("Не найден бинарник usque.");
  $("warn-missing").hidden = missing.length === 0;
  $("warn-missing").textContent = missing.join(" ");

  const dl = $("facts");
  dl.textContent = "";
  fact(dl, "Процесс", st.running ? "запущен (PID " + st.pid + ")" : "не запущен", st.running ? "ok" : "bad");
  const l = st.link;
  fact(dl, "Интерфейс " + st.iface,
    !l.exists ? "отсутствует в системе" : (l.up ? "поднят" : "опущен") + (l.oper ? " (" + l.oper + ")" : ""),
    !l.exists ? "bad" : l.up ? "ok" : "warn");
  const n = st.ndms;
  if (n.available) {
    const parts = [];
    if (n.state) parts.push("state: " + n.state);
    if (n.link) parts.push("link: " + n.link);
    if (n.connected) parts.push("connected: " + n.connected);
    fact(dl, "NDMS " + n.name, parts.join(", ") || "есть", n.link === "up" || n.state === "up" ? "ok" : "warn");
    if (n.address) fact(dl, "Адрес", n.address + (n.mtu ? " (MTU " + n.mtu + ")" : ""));
  } else {
    fact(dl, "NDMS " + n.name, n.error || "интерфейс не найден", "warn");
  }
  fact(dl, "Аккаунт WARP", st.hasSession ? "зарегистрирован" : "не зарегистрирован", st.hasSession ? "ok" : "warn");

  $("btn-start").disabled = busy || st.running || !st.initFound;
  $("btn-stop").disabled = busy || !st.running;
  $("btn-restart").disabled = busy || !st.initFound;
  $("btn-check").disabled = busy || !st.running;
  $("btn-reset").disabled = busy || st.running || !st.hasSession;
  $("btn-register").disabled = busy || st.hasSession;
  $("sess-text").textContent = st.hasSession
    ? "Сессия есть. Сброс удалит ключи устройства: при следующем запуске usque зарегистрирует новый аккаунт."
    : "Сессии нет. Она создаётся автоматически при первом запуске или кнопкой ниже.";
}

async function refresh() {
  try { renderStatus(await api("GET", "/api/status")); } catch (e) { if (e.message) { /* 401 обработан */ } }
}

async function loadConfig() {
  try {
    const c = await api("GET", "/api/config");
    $("cfg-iface").value = c.iface || "";
    $("cfg-sni").value = c.sni || "";
    $("cfg-ip").value = c.ifaceIp || "";
    $("cfg-mask").value = c.ifaceMask || "";
    $("cfg-http2").checked = !!c.http2;
    cfgDirty = false;
  } catch (e) { $("cfg-msg").textContent = e.message; }
}

$("cfg-form").addEventListener("input", () => { cfgDirty = true; $("cfg-msg").textContent = ""; });
$("cfg-form").addEventListener("submit", async (ev) => {
  ev.preventDefault();
  try {
    await api("PUT", "/api/config", {
      iface: $("cfg-iface").value.trim(),
      sni: $("cfg-sni").value.trim(),
      ifaceIp: $("cfg-ip").value.trim(),
      ifaceMask: $("cfg-mask").value.trim(),
      http2: $("cfg-http2").checked,
    });
    cfgDirty = false;
    toast("Настройки сохранены");
    refresh(); loadLog();
  } catch (e) { toast(e.message, true); }
});

async function act(label, fn) {
  if (busy) return;
  busy = true;
  document.querySelectorAll(".actions button").forEach((x) => { x.disabled = true; });
  toast(label + "…");
  try {
    await fn();
    toast("Готово");
  } catch (e) {
    toast(e.message + (e.output ? ": " + e.output.split("\n").slice(-1)[0] : ""), true);
  } finally {
    busy = false;
    await refresh();
    loadLog();
  }
}

const svc = (action) => () => api("POST", "/api/service", { action });
$("btn-start").onclick = () => act("Запуск", svc("start"));
$("btn-stop").onclick = () => act("Остановка", svc("stop"));
$("btn-restart").onclick = () => act("Перезапуск", svc("restart"));
$("btn-register").onclick = () => act("Регистрация", () => api("POST", "/api/session/register", {}));
$("btn-reset").onclick = () => {
  if (!confirm("Удалить сессию WARP? При следующем запуске будет создан новый аккаунт.")) return;
  act("Сброс сессии", () => api("POST", "/api/session/reset", {}));
};

$("btn-check").onclick = async () => {
  const out = $("check-result");
  out.textContent = "Проверяю…";
  try {
    const r = await api("POST", "/api/check", {});
    if (r.error) out.textContent = "Не удалось: " + r.error;
    else out.textContent = "Внешний IP: " + r.ip + (r.loc ? " (" + r.loc + (r.colo ? ", " + r.colo : "") + ")" : "") +
      " · WARP: " + (r.warp || "?");
  } catch (e) { out.textContent = e.message; }
};

async function loadLog() {
  try {
    const entries = await api("GET", "/api/log");
    const box = $("log");
    box.textContent = "";
    if (!entries.length) {
      const p = document.createElement("div");
      p.className = "muted";
      p.textContent = "Пока пусто.";
      box.appendChild(p);
      return;
    }
    entries.slice().reverse().forEach((e) => {
      const row = document.createElement("div");
      row.className = "e";
      const h = document.createElement("div");
      h.className = "h";
      const t = new Date(e.time).toLocaleTimeString();
      const ok = document.createElement("span");
      ok.className = e.ok ? "ok" : "fail";
      ok.textContent = e.ok ? "ок" : "ошибка";
      const b = document.createElement("b");
      b.textContent = e.action;
      h.append(t + " ", b, " — ", ok);
      row.appendChild(h);
      if (e.output) {
        const pre = document.createElement("pre");
        pre.textContent = e.output;
        row.appendChild(pre);
      }
      box.appendChild(row);
    });
  } catch (_) { /* ignore */ }
}
$("log-refresh").onclick = loadLog;

$("pw-form").addEventListener("submit", async (ev) => {
  ev.preventDefault();
  try {
    await api("POST", "/api/password", { current: $("pw-cur").value, new: $("pw-new").value });
    $("pw-cur").value = $("pw-new").value = "";
    $("pw-msg").textContent = "";
    toast("Пароль изменён");
  } catch (e) { $("pw-msg").textContent = e.message; }
});

boot();
