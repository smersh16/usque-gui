"use strict";

const $ = (id) => document.getElementById(id);
let busy = false;
let pollTimer = null;
let hasSession = false;
let wasRunning = false;
let setupMode = false;

// последние данные — чтобы перерисовать при смене языка
let lastStatus = null;
let lastVersions = null;
let lastLog = null;
let lastProbe = null;
let lastTraffic = null; // для расчёта скорости

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
    const e = new Error(data.error ? tr(data.error) : t("Ошибка {code}", { code: r.status }));
    e.output = data.output;
    throw e;
  }
  return data;
}

let toastTimer;
function toast(msg, bad) {
  const el = $("toast");
  el.textContent = msg;
  el.className = "toast" + (bad ? " bad" : "");
  el.hidden = false;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => { el.hidden = true; }, bad ? 6000 : 3000);
}

function fmtBytes(n) {
  const units = ["Б", "КБ", "МБ", "ГБ", "ТБ"];
  let i = 0;
  let v = n;
  while (v >= 1024 && i < units.length - 1) { v /= 1024; i++; }
  return (i === 0 ? String(Math.round(v)) : v.toFixed(v < 10 ? 2 : 1)) + " " + t(units[i]);
}

// ---------- язык ----------
function refreshLangButton() { $("lang").textContent = LANG === "ru" ? "EN" : "RU"; }

function rerender() {
  applyI18n();
  refreshLangButton();
  updateAuthTexts();
  if (lastStatus) renderStatus(lastStatus);
  if (lastVersions) renderVersions(lastVersions);
  if (lastLog) renderLog(lastLog);
  if (lastProbe) renderProbe(lastProbe);
  if ($("sess-edit").open) loadSession();
}

$("lang").addEventListener("click", () => {
  setLang(LANG === "ru" ? "en" : "ru");
  rerender();
});

// ---------- авторизация ----------
function updateAuthTexts() {
  $("auth-title").textContent = setupMode ? t("Задайте пароль") : t("Вход");
  $("auth-submit").textContent = setupMode ? t("Сохранить") : t("Войти");
  $("auth-hint").textContent = setupMode ? t("Первый запуск: придумайте пароль для этого интерфейса (от 6 символов).") : "";
}

async function boot() {
  setLang(LANG);
  rerender();
  try {
    const a = await api("GET", "/api/auth");
    $("ver").textContent = a.version || "";
    if (a.authenticated) showApp(); else showAuth(a.needSetup);
  } catch (e) {
    showAuth();
    $("auth-err").textContent = e.message;
  }
}

async function showAuth(needSetup) {
  clearInterval(pollTimer);
  if (needSetup === undefined) {
    try { needSetup = (await api("GET", "/api/auth")).needSetup; } catch (_) { needSetup = false; }
  }
  setupMode = !!needSetup;
  $("view-app").hidden = true;
  $("logout").hidden = true;
  $("view-auth").hidden = false;
  $("auth-pass2-wrap").hidden = !setupMode;
  $("auth-pass").autocomplete = setupMode ? "new-password" : "current-password";
  updateAuthTexts();
}

$("auth-form").addEventListener("submit", async (ev) => {
  ev.preventDefault();
  $("auth-err").textContent = "";
  const pw = $("auth-pass").value;
  if (setupMode && pw !== $("auth-pass2").value) {
    $("auth-err").textContent = t("Пароли не совпадают");
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
  loadConfig();
  refresh();
  loadLog();
  loadVersions();
  loadGUI();
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

function trafficText(link, iface) {
  const now = Date.now();
  let text = "↓ " + fmtBytes(link.rxBytes) + "  ↑ " + fmtBytes(link.txBytes);
  const p = lastTraffic;
  if (p && p.iface === iface && now > p.t && link.rxBytes >= p.rx && link.txBytes >= p.tx) {
    const dt = (now - p.t) / 1000;
    if (dt >= 2) {
      const rx = (link.rxBytes - p.rx) / dt;
      const tx = (link.txBytes - p.tx) / dt;
      text += "  (↓ " + fmtBytes(rx) + t("/с") + "  ↑ " + fmtBytes(tx) + t("/с") + ")";
      lastTraffic = { iface, t: now, rx: link.rxBytes, tx: link.txBytes };
      lastRate = text;
      return text;
    }
    return lastRate || text;
  }
  lastTraffic = { iface, t: now, rx: link.rxBytes, tx: link.txBytes };
  lastRate = null;
  return text;
}
let lastRate = null;

function renderStatus(res) {
  lastStatus = res;
  const st = res.status;
  const b = $("badge");
  b.className = "badge " + st.state;
  b.textContent = t(STATE_TEXT[st.state] || st.state);

  $("warn-restart").hidden = !res.restartPending;

  const missing = [];
  if (!st.initFound) missing.push(t("Не найден init-скрипт usque-keenetic (S51usque). Установите пакет usque-keenetic."));
  else if (!st.binaryFound) missing.push(t("Не найден бинарник usque."));
  $("warn-missing").hidden = missing.length === 0;
  $("warn-missing").textContent = missing.join(" ");

  const dl = $("facts");
  dl.textContent = "";
  fact(dl, t("Процесс"), st.running ? t("запущен (PID {pid})", { pid: st.pid }) : t("не запущен"), st.running ? "ok" : "bad");
  const l = st.link;
  fact(dl, t("Интерфейс {name}", { name: st.iface }),
    !l.exists ? t("отсутствует в системе") : (l.up ? t("поднят") : t("опущен")) + (l.oper ? " (" + l.oper + ")" : ""),
    !l.exists ? "bad" : l.up ? "ok" : "warn");
  const n = st.ndms;
  if (n.available) {
    const parts = [];
    if (n.state) parts.push("state: " + n.state);
    if (n.link) parts.push("link: " + n.link);
    if (n.connected) parts.push("connected: " + n.connected);
    fact(dl, t("NDMS {name}", { name: n.name }), parts.join(", ") || t("есть"), n.link === "up" || n.state === "up" ? "ok" : "warn");
    if (n.address) fact(dl, t("Адрес"), n.address + (n.mtu ? " (MTU " + n.mtu + ")" : ""));
  } else {
    fact(dl, t("NDMS {name}", { name: n.name }), n.error ? tr(n.error) : t("интерфейс не найден"), "warn");
  }
  if (l.exists) fact(dl, t("Трафик"), trafficText(l, st.iface));
  fact(dl, t("Аккаунт WARP"), st.hasSession ? t("зарегистрирован") : t("не зарегистрирован"), st.hasSession ? "ok" : "warn");

  $("btn-start").disabled = busy || st.running || !st.initFound;
  $("btn-stop").disabled = busy || !st.running;
  $("btn-restart").disabled = busy || !st.initFound;
  $("btn-check").disabled = busy || !st.running;
  hasSession = st.hasSession;
  $("sess-edit").hidden = !st.hasSession;
  $("card-probe").hidden = !st.hasSession;
  if (st.hasSession) initProbeList();
  wasRunning = st.running;
  $("btn-reset").disabled = busy || !st.hasSession;
  $("btn-register").disabled = busy || !st.initFound;
  $("btn-register").textContent = st.hasSession ? t("Перерегистрировать") : t("Зарегистрировать");
  $("sess-text").textContent = st.hasSession
    ? t("Аккаунт WARP зарегистрирован, сессия сохранена.")
    : t("Сессии нет. Она создаётся автоматически при первом запуске или кнопкой ниже.");
  $("sess-hint").textContent = st.hasSession
    ? t("«Перерегистрировать» и «Сбросить» заменяют ключи устройства; запущенный сервис будет остановлен (после перерегистрации — запущен снова).")
    : "";
}

async function refresh() {
  try { renderStatus(await api("GET", "/api/status")); } catch (_) { /* 401 обработан */ }
}

async function loadConfig() {
  try {
    const c = await api("GET", "/api/config");
    $("cfg-iface").value = c.iface || "";
    $("cfg-sni").value = c.sni || "";
    $("cfg-ip").value = c.ifaceIp || "";
    $("cfg-mask").value = c.ifaceMask || "";
    $("cfg-http2").checked = !!c.http2;
  } catch (e) { $("cfg-msg").textContent = e.message; }
}

$("cfg-form").addEventListener("input", () => { $("cfg-msg").textContent = ""; });
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
    toast(t("Настройки сохранены"));
    refresh(); loadLog();
  } catch (e) { toast(e.message, true); }
});

async function act(label, fn) {
  if (busy) return;
  busy = true;
  ["btn-start", "btn-stop", "btn-restart", "btn-check", "btn-register", "btn-reset"].forEach((id) => { $(id).disabled = true; });
  toast(label + "…");
  try {
    await fn();
    toast(t("Готово"));
  } catch (e) {
    toast(e.message + (e.output ? ": " + e.output.split("\n").slice(-1)[0] : ""), true);
  } finally {
    busy = false;
    await refresh();
    loadLog();
  }
}

const svc = (action) => () => api("POST", "/api/service", { action });
$("btn-start").onclick = () => act(t("Запуск"), svc("start"));
$("btn-stop").onclick = () => act(t("Остановка"), svc("stop"));
$("btn-restart").onclick = () => act(t("Перезапуск"), svc("restart"));
$("btn-register").onclick = () => {
  if (!hasSession) return act(t("Регистрация"), () => api("POST", "/api/session/register", {}));
  if (!confirm(t("Заменить аккаунт WARP новым?") + (wasRunning ? " " + t("Сервис будет перезапущен.") : ""))) return;
  act(t("Перерегистрация"), () => api("POST", "/api/session/renew", {}));
};
$("btn-reset").onclick = () => {
  if (!confirm(t("Удалить сессию WARP?") + (wasRunning ? " " + t("Сервис будет остановлен.") : "") +
    " " + t("При следующем запуске usque создаст новый аккаунт."))) return;
  act(t("Сброс сессии"), () => api("POST", "/api/session/reset", {}));
};

// ---------- конфигурация сессии ----------
const SESS_PLAIN = ["endpoint_v4", "endpoint_v6", "endpoint_h2_v4", "endpoint_h2_v6", "ipv4", "ipv6", "id", "endpoint_pub_key"];
const SESS_SECRET = ["private_key", "access_token"];
let skipNextToggleLoad = false;

async function loadSession() {
  try {
    const v = await api("GET", "/api/session");
    $("sess-edit").hidden = !v.exists;
    if (!v.exists) return v;
    SESS_PLAIN.forEach((k) => { $("s-" + k).value = (v.config && v.config[k]) || ""; });
    SESS_SECRET.forEach((k) => {
      const set = v.config && v.config[k];
      $("s-" + k).value = "";
      $("s-" + k).placeholder = set ? t("•••••••• (задан, не менять)") : t("не задан");
    });
    $("sess-msg").textContent = "";
    return v;
  } catch (e) { $("sess-msg").textContent = e.message; return null; }
}

$("sess-edit").addEventListener("toggle", () => {
  if (skipNextToggleLoad) { skipNextToggleLoad = false; return; }
  if ($("sess-edit").open) loadSession();
});
$("sess-form").addEventListener("submit", async (ev) => {
  ev.preventDefault();
  const body = {};
  SESS_PLAIN.concat(SESS_SECRET).forEach((k) => { body[k] = $("s-" + k).value.trim(); });
  try {
    await api("PUT", "/api/session", body);
    toast(t("Сессия сохранена"));
    await loadSession();
    refresh(); loadLog();
  } catch (e) { $("sess-msg").textContent = e.message; toast(e.message, true); }
});

// ---------- подбор endpoint ----------
// Настоящие MASQUE-endpoint'ы (см. docs warpscout):
//   QUIC (UDP):  162.159.198.1, 162.159.198.2
//   HTTP/2 (TCP): пулы 162.159.198.0/24 и 162.159.199.0/24
const PROBE_QUIC = ["162.159.198.1", "162.159.198.2"];
const PROBE_H2_POOLS = ["162.159.198", "162.159.199"];
let probeInit = false;
let lastProbeMode = "quic";
let sessionEndpoints = {};

function randomH2(n, exclude) {
  const out = new Set(exclude);
  const res = [];
  let guard = 0;
  while (res.length < n && guard++ < 1000) {
    const ip = PROBE_H2_POOLS[Math.floor(Math.random() * PROBE_H2_POOLS.length)] + "." + (1 + Math.floor(Math.random() * 254));
    if (!out.has(ip)) { out.add(ip); res.push(ip); }
  }
  return res;
}

function fillProbeList(forceRandom) {
  const mode = $("probe-mode").value;
  const curKey = mode === "h2" ? "endpoint_h2_v4" : "endpoint_v4";
  const cur = sessionEndpoints[curKey] ? [sessionEndpoints[curKey]] : [];
  let list;
  if (mode === "quic") {
    list = cur.concat(PROBE_QUIC.filter((x) => !cur.includes(x)));
  } else {
    const base = cur.concat(["162.159.198.2"].filter((x) => !cur.includes(x)));
    list = base.concat(randomH2(forceRandom ? 15 : 12, base));
  }
  $("probe-list").value = list.join("\n");
}

async function initProbeList() {
  if (probeInit) return;
  probeInit = true;
  const v = await api("GET", "/api/session").catch(() => null);
  sessionEndpoints = (v && v.config) || {};
  const c = await api("GET", "/api/config").catch(() => null);
  $("probe-mode").value = c && c.http2 ? "h2" : "quic";
  fillProbeList(false);
}

$("probe-mode").addEventListener("change", () => fillProbeList(false));
$("btn-probe-random").onclick = () => { $("probe-mode").value = "h2"; fillProbeList(true); };

function renderProbe(results) {
  lastProbe = results;
  const box = $("probe-result");
  box.textContent = "";
  if (!results.length) return;
  const isH2 = lastProbeMode === "h2";
  const table = document.createElement("table");
  const head = document.createElement("tr");
  ["IP", t("Ответы"), t("Задержка")].concat(isH2 ? [t("TLS")] : []).concat([""]).forEach((h) => {
    const th = document.createElement("th");
    th.textContent = h;
    head.appendChild(th);
  });
  table.appendChild(head);
  results.forEach((r) => {
    const row = document.createElement("tr");
    const cells = [r.ip, r.success + "/" + r.attempts, r.ok ? r.rttMs.toFixed(1) + " ms" : tr(r.error || "")];
    if (isH2) cells.push(r.ok ? (r.tls ? t("да") : t("нет")) : "—");
    cells.forEach((c, i) => {
      const td = document.createElement("td");
      td.textContent = c;
      if (!r.ok && i === 2) td.className = "bad-text";
      row.appendChild(td);
    });
    const act_ = document.createElement("td");
    if (r.ok) {
      const b = document.createElement("button");
      b.textContent = t("Применить");
      b.onclick = () => applyEndpoint(r.ip);
      act_.appendChild(b);
    }
    row.appendChild(act_);
    table.appendChild(row);
  });
  box.appendChild(table);
}

async function applyEndpoint(ip) {
  const v6 = ip.includes(":");
  const field = (lastProbeMode === "h2" ? "endpoint_h2_" : "endpoint_") + (v6 ? "v6" : "v4");
  const det = $("sess-edit");
  if (!det.open) { skipNextToggleLoad = true; det.open = true; }
  await loadSession();
  $("s-" + field).value = ip;
  toast(t("Значение подставлено в поле {field} формы сессии — проверьте и нажмите «Сохранить сессию».", { field }));
  det.scrollIntoView({ behavior: "smooth", block: "center" });
}

$("btn-probe").onclick = async () => {
  const ips = $("probe-list").value.split(/[\s,;]+/).filter(Boolean);
  const b = $("btn-probe");
  b.disabled = true;
  $("probe-msg").textContent = t("Проверяю адреса…");
  const mode = $("probe-mode").value;
  try {
    const res = await api("POST", "/api/probe", { ips, mode, attempts: +$("probe-tries").value });
    lastProbeMode = mode;
    renderProbe(res);
    $("probe-msg").textContent = res.some((r) => r.ok) ? "" : t("Нет ответивших адресов");
  } catch (e) { $("probe-msg").textContent = e.message; }
  finally { b.disabled = false; }
};

// ---------- версии и обновления ----------
function renderVersions(v) {
  lastVersions = v;
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
    let s = t("установлена: {v}", { v: p.installed || t("неизвестно") });
    if (p.binaryVersion) s += " · " + t("бинарник usque {v}", { v: p.binaryVersion });
    if (p.upgradable) s += " · " + t("доступна: {v}", { v: p.available });
    info.textContent = s;
    left.append(name, info);

    const right = document.createElement("div");
    right.className = "row";
    if (p.upgradable) {
      const tag = document.createElement("span");
      tag.className = "tag new";
      tag.textContent = t("есть обновление");
      right.appendChild(tag);
      if (p.canUpgrade) {
        const b = document.createElement("button");
        b.textContent = t("Обновить");
        b.onclick = () => {
          if (!confirm(t("Обновить {name} до {v}? Сервис будет перезапущен.", { name: p.name, v: p.available }))) return;
          act(t("Обновление {name}", { name: p.name }), async () => {
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
      tag.textContent = t("актуальная");
      right.appendChild(tag);
    }
    row.append(left, right);
    box.appendChild(row);
  });
  $("upd-msg").textContent = (v.checkedAt ? t("Проверено: {d}.", { d: new Date(v.checkedAt).toLocaleString() }) : t("Ещё не проверялось.")) +
    (v.warning ? " " + v.warning : "");
}

async function loadVersions() {
  try { renderVersions(await api("GET", "/api/versions")); } catch (_) { /* ignore */ }
}

$("btn-upd-check").onclick = async () => {
  const b = $("btn-upd-check");
  b.disabled = true;
  $("upd-msg").textContent = t("Проверяю (opkg update)…");
  try { renderVersions(await api("POST", "/api/updates/check", {})); }
  catch (e) { $("upd-msg").textContent = e.message; }
  finally { b.disabled = false; loadLog(); }
};

$("btn-check").onclick = async () => {
  const out = $("check-result");
  out.textContent = t("Проверяю…");
  try {
    const r = await api("POST", "/api/check", {});
    if (r.error) out.textContent = t("Не удалось: {err}", { err: tr(r.error) });
    else out.textContent = t("Внешний IP: {ip}", { ip: r.ip }) + (r.loc ? " (" + r.loc + (r.colo ? ", " + r.colo : "") + ")" : "") +
      " · WARP: " + (r.warp || "?");
  } catch (e) { out.textContent = e.message; }
};

// ---------- резервная копия ----------
$("btn-backup").onclick = async () => {
  const includeSession = $("bk-session").checked;
  $("bk-msg").textContent = "";
  try {
    const r = await fetch("/api/backup", {
      method: "POST",
      headers: { "X-Requested-With": "usque-gui", "Content-Type": "application/json" },
      body: JSON.stringify({ includeSession, password: $("bk-pass").value }),
    });
    if (!r.ok) {
      let d = {};
      try { d = await r.json(); } catch (_) { /* ignore */ }
      throw new Error(d.error ? tr(d.error) : t("Ошибка {code}", { code: r.status }));
    }
    const blob = await r.blob();
    const m = /filename="([^"]+)"/.exec(r.headers.get("Content-Disposition") || "");
    const a = document.createElement("a");
    a.href = URL.createObjectURL(blob);
    a.download = m ? m[1] : "usque-gui-backup.json";
    document.body.appendChild(a);
    a.click();
    a.remove();
    setTimeout(() => URL.revokeObjectURL(a.href), 5000);
    toast(t("Копия скачана"));
    loadLog();
  } catch (e) { toast(e.message, true); }
};

$("btn-restore").onclick = async () => {
  const f = $("rs-file").files[0];
  if (!f) { $("bk-msg").textContent = t("Выберите файл копии"); return; }
  let backup;
  try { backup = JSON.parse(await f.text()); } catch (_) { $("bk-msg").textContent = t("Файл не является корректным JSON"); return; }
  if (!confirm(t("Восстановить из копии? Текущие настройки будут заменены (резервные копии файлов сохраняются рядом)."))) return;
  $("bk-msg").textContent = "";
  try {
    await api("POST", "/api/restore", {
      password: $("bk-pass").value, backup,
      applyConfig: $("rs-conf").checked, applySession: $("rs-session").checked,
    });
    toast(t("Восстановлено"));
    $("rs-file").value = "";
    loadConfig(); refresh(); loadLog();
    if ($("sess-edit").open) loadSession();
  } catch (e) { toast(e.message, true); }
};

// ---------- журнал ----------
function renderLog(entries) {
  lastLog = entries;
  const box = $("log");
  box.textContent = "";
  if (!entries.length) {
    const p = document.createElement("div");
    p.className = "muted";
    p.textContent = t("Пока пусто.");
    box.appendChild(p);
    return;
  }
  entries.slice().reverse().forEach((e) => {
    const row = document.createElement("div");
    row.className = "e";
    const h = document.createElement("div");
    h.className = "h";
    const ts = new Date(e.time).toLocaleTimeString();
    const ok = document.createElement("span");
    ok.className = e.ok ? "ok" : "fail";
    ok.textContent = e.ok ? t("ок") : t("ошибка");
    const b = document.createElement("b");
    b.textContent = e.action;
    h.append(ts + " ", b, " — ", ok);
    row.appendChild(h);
    if (e.output) {
      const pre = document.createElement("pre");
      pre.textContent = tr(e.output);
      row.appendChild(pre);
    }
    box.appendChild(row);
  });
}

async function loadLog() {
  try { renderLog(await api("GET", "/api/log")); } catch (_) { /* ignore */ }
}
$("log-refresh").onclick = loadLog;

// ---------- настройки интерфейса ----------
let guiListen = "";

async function loadGUI() {
  try {
    const g = await api("GET", "/api/gui");
    guiListen = g.listen || "";
    $("gui-listen").value = guiListen;
    $("btn-gui-restart").dataset.can = g.canRestart ? "1" : "";
  } catch (e) { $("gui-msg").textContent = e.message; }
}

$("gui-form").addEventListener("submit", async (ev) => {
  ev.preventDefault();
  $("gui-msg").textContent = "";
  try {
    const r = await api("PUT", "/api/gui", { listen: $("gui-listen").value.trim() });
    $("gui-listen").value = r.listen;
    const changed = r.listen !== guiListen;
    $("gui-msg").textContent = changed ? t("Адрес сохранён. Перезапустите интерфейс, чтобы применить.") : "";
    $("btn-gui-restart").hidden = !(changed && $("btn-gui-restart").dataset.can);
    toast(t("Настройки сохранены"));
    loadLog();
  } catch (e) { $("gui-msg").textContent = e.message; }
});

$("btn-gui-restart").onclick = async () => {
  try {
    const r = await api("POST", "/api/gui/restart", {});
    $("gui-msg").textContent = t("Перезапуск интерфейса… страница откроется по новому адресу.");
    const m = /^(.*):(\d+)$/.exec(r.listen || "");
    let host = location.hostname;
    if (m && m[1] && m[1] !== "0.0.0.0" && m[1] !== "::" && m[1] !== "[::]") host = m[1].replace(/^\[|\]$/g, "");
    const port = m ? m[2] : location.port;
    const h = host.includes(":") ? "[" + host + "]" : host;
    setTimeout(() => { location.href = location.protocol + "//" + h + ":" + port + "/"; }, 2500);
  } catch (e) { $("gui-msg").textContent = e.message; }
};

$("pw-form").addEventListener("submit", async (ev) => {
  ev.preventDefault();
  try {
    await api("POST", "/api/password", { current: $("pw-cur").value, new: $("pw-new").value });
    $("pw-cur").value = $("pw-new").value = "";
    $("pw-msg").textContent = "";
    toast(t("Пароль изменён"));
  } catch (e) { $("pw-msg").textContent = e.message; }
});

boot();
