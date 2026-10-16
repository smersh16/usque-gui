"use strict";

const $ = (id) => document.getElementById(id);
let busy = false;
let pollTimer = null;
let cfgDirty = false;
let hasSession = false;
let wasRunning = false;

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
  loadVersions();
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
  hasSession = st.hasSession;
  $("sess-edit").hidden = !st.hasSession;
  wasRunning = st.running;
  $("btn-reset").disabled = busy || !st.hasSession;
  $("btn-register").disabled = busy || !st.initFound;
  $("btn-register").textContent = st.hasSession ? "Перерегистрировать" : "Зарегистрировать";
  $("sess-text").textContent = st.hasSession
    ? "Аккаунт WARP зарегистрирован, сессия сохранена."
    : "Сессии нет. Она создаётся автоматически при первом запуске или кнопкой ниже.";
  $("sess-hint").textContent = st.hasSession
    ? "«Перерегистрировать» и «Сбросить» заменяют ключи устройства; запущенный сервис будет остановлен" +
      " (после перерегистрации — запущен снова)."
    : "";
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
$("btn-register").onclick = () => {
  if (!hasSession) return act("Регистрация", () => api("POST", "/api/session/register", {}));
  if (!confirm("Заменить аккаунт WARP новым?" + (wasRunning ? " Сервис будет перезапущен." : ""))) return;
  act("Перерегистрация", () => api("POST", "/api/session/renew", {}));
};
$("btn-reset").onclick = () => {
  if (!confirm("Удалить сессию WARP?" + (wasRunning ? " Сервис будет остановлен." : "") +
    " При следующем запуске usque создаст новый аккаунт.")) return;
  act("Сброс сессии", () => api("POST", "/api/session/reset", {}));
};

// ---------- конфигурация сессии ----------
const SESS_PLAIN = ["endpoint_v4", "endpoint_v6", "endpoint_h2_v4", "endpoint_h2_v6", "ipv4", "ipv6", "id", "endpoint_pub_key"];
const SESS_SECRET = ["private_key", "access_token"];

async function loadSession() {
  try {
    const v = await api("GET", "/api/session");
    $("sess-edit").hidden = !v.exists;
    if (!v.exists) return;
    SESS_PLAIN.forEach((k) => { $("s-" + k).value = (v.config && v.config[k]) || ""; });
    SESS_SECRET.forEach((k) => {
      const set = v.config && v.config[k];
      $("s-" + k).value = "";
      $("s-" + k).placeholder = set ? "•••••••• (задан, не менять)" : "не задан";
    });
    $("sess-msg").textContent = "";
  } catch (e) { $("sess-msg").textContent = e.message; }
}

$("sess-edit").addEventListener("toggle", () => { if ($("sess-edit").open) loadSession(); });
$("sess-form").addEventListener("submit", async (ev) => {
  ev.preventDefault();
  const body = {};
  SESS_PLAIN.forEach((k) => { body[k] = $("s-" + k).value.trim(); });
  SESS_SECRET.forEach((k) => { body[k] = $("s-" + k).value.trim(); });
  try {
    await api("PUT", "/api/session", body);
    toast("Сессия сохранена");
    await loadSession();
    refresh(); loadLog();
  } catch (e) { $("sess-msg").textContent = e.message; toast(e.message, true); }
});

// ---------- версии и обновления ----------
function renderVersions(v) {
  const box = $("versions");
  box.textContent = "";
  (v.packages || []).forEach((p) => {
    const row = document.createElement("div");
    row.className = "ver";
    const left = document.createElement("div");
    const name = document.createElement("div");
    name.className = "name";
    name.textContent = p.name;
    const info = document.createElement("div");
    info.className = "info";
    let t = "установлена: " + (p.installed || "неизвестно");
    if (p.binaryVersion) t += " · бинарник usque " + p.binaryVersion;
    if (p.upgradable) t += " · доступна: " + p.available;
    info.textContent = t;
    left.append(name, info);

    const right = document.createElement("div");
    right.className = "row";
    if (p.upgradable) {
      const tag = document.createElement("span");
      tag.className = "tag new";
      tag.textContent = "есть обновление";
      right.appendChild(tag);
      if (p.canUpgrade) {
        const b = document.createElement("button");
        b.textContent = "Обновить";
        b.onclick = () => {
          if (!confirm("Обновить " + p.name + " до " + p.available + "? Сервис будет перезапущен.")) return;
          act("Обновление " + p.name, async () => {
            await api("POST", "/api/updates/upgrade", { package: p.name });
            await loadVersions();
          });
        };
        right.appendChild(b);
      } else if (p.upgradeHint) {
        const c = document.createElement("code");
        c.textContent = p.upgradeHint;
        right.appendChild(c);
      }
    } else if (v.checkedAt) {
      const tag = document.createElement("span");
      tag.className = "tag ok";
      tag.textContent = "актуальная";
      right.appendChild(tag);
    }
    row.append(left, right);
    box.appendChild(row);
  });
  $("upd-msg").textContent = (v.checkedAt ? "Проверено: " + new Date(v.checkedAt).toLocaleString() + ". " : "Ещё не проверялось. ") +
    (v.warning || "");
}

async function loadVersions() {
  try { renderVersions(await api("GET", "/api/versions")); } catch (_) { /* ignore */ }
}

$("btn-upd-check").onclick = async () => {
  const b = $("btn-upd-check");
  b.disabled = true;
  $("upd-msg").textContent = "Проверяю (opkg update)…";
  try { renderVersions(await api("POST", "/api/updates/check", {})); }
  catch (e) { $("upd-msg").textContent = e.message; }
  finally { b.disabled = false; loadLog(); }
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
