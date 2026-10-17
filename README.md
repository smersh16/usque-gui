# usque-gui

Лёгкий веб-интерфейс для [usque-keenetic](https://github.com/side-effect-tm/usque-keenetic) —
неофициального клиента Cloudflare WARP (MASQUE) для роутеров Keenetic/Netcraze.

Один статический бинарник на Go (~6 МБ, без зависимостей), интерфейс встроен в него.
usque-gui **не заменяет** usque-keenetic и ничего в нём не меняет: он вызывает его
init-скрипт и правит его `usque.conf`.

## Возможности

- Статус: процесс, интерфейс в ядре, состояние интерфейса в NDMS (`state`/`link`/`connected`), адрес.
- Запуск, остановка, перезапуск (`/opt/etc/init.d/S51usque`).
- Редактор `usque.conf`: интерфейс, IP, маска, SNI, HTTP/2. Комментарии и `CONFIG_VERSION` сохраняются,
  значения строго валидируются (файл выполняется как shell), перед записью делается копия `usque.conf.gui-bak`.
- Аккаунт WARP: регистрация, перерегистрация (сброс + новая регистрация + перезапуск сервиса) и сброс сессии;
  запущенный сервис при этом останавливается автоматически. Содержимое `session.conf` в браузер не передаётся.
- Редактор сессии (`session.conf`): endpoint'ы (в т.ч. для HTTP/2), адреса аккаунта, ключ сервера (PEM).
  Секреты (`private_key`, `access_token`) в браузер не отдаются — показываются маской и меняются только если
  ввести новое значение; значения валидируются, перед записью делается копия `session.conf.gui-bak`.
- Подбор endpoint по схеме [warpscout](https://github.com/vernette/warpscout/blob/master/docs/ru/masque.md):
  режим QUIC (UDP; на адрес уходит QUIC-пакет с неизвестной версией, ответ Version Negotiation означает,
  что QUIC-сервер отвечает; адреса 162.159.198.1/.2) и режим HTTP/2 (TCP + TLS на 443, пулы
  162.159.198.0/24 и 162.159.199.0/24, кнопка «случайные из пулов»). Каждый адрес проверяется 3/5/10 раз
  (endpoint'ы нестабильны), результат — число ответов и задержка; выбранный адрес подставляется в
  `endpoint_v4` или `endpoint_h2_v4` формы сессии. Замер подтверждает достижимость, но не работу
  MASQUE-аккаунта. Зондируются только публичные IPv4-адреса (до 32), порт 443.
- Резервная копия: скачивание и восстановление настроек usque и (по желанию) сессии WARP одним JSON-файлом.
  Копия с сессией содержит секретные ключи, поэтому и скачивание, и восстановление сессии требуют пароль
  интерфейса. При восстановлении все значения проходят ту же валидацию, что и при ручном вводе.
- Трафик интерфейса (всего и текущая скорость), адрес и порт веб-интерфейса из GUI (с самоперезапуском),
  русский и английский язык (переключатель в шапке, выбор запоминается в браузере).
- Версии и обновления: установленные версии `usque-keenetic` (и бинарника usque) и `usque-gui`, проверка
  обновлений через `opkg update` + `opkg list-upgradable`, обновление `usque-keenetic` кнопкой.
- «Проверить выход» — запрос к Cloudflare через интерфейс usque (внешний IP, `warp=on/plus`). Экспериментально.
- Журнал действий, выполненных из GUI (у самого usque журнала нет).

## Требования

- Keenetic с Entware, установленный и настроенный [usque-keenetic](https://github.com/side-effect-tm/usque-keenetic).
- Архитектуры пакетов: `aarch64-3.10`, `mipsel-3.4`, `mips-3.4`.

## Установка

Через opkg (SSH-сессия в Entware, не во встроенном CLI роутера):

```sh
# замените aarch64-3.10 на вашу архитектуру: opkg print-architecture
echo "src/gz usque-gui https://smersh16.github.io/usque-gui/aarch64-3.10" > /opt/etc/opkg/usque-gui.conf
opkg update
opkg install usque-gui
```

Либо скачайте `.ipk` со страницы [Releases](https://github.com/smersh16/usque-gui/releases)
и установите: `opkg install usque-gui_<версия>_<arch>.ipk`.

Откройте `http://<адрес-роутера>:2223` — при первом входе нужно задать пароль.
Пароль можно задать и из консоли: `usque-gui passwd`.

Обновление: `opkg update && opkg upgrade usque-gui`. Удаление: `opkg remove usque-gui`
(настройки в `/opt/etc/usque-gui/` остаются).

## Управление службой

```sh
/opt/etc/init.d/S52usque-gui start|stop|restart|status
```

Журнал самого GUI: `/opt/var/log/usque-gui.log`.

## Безопасность

- Доступ только из локальных сетей (loopback, частные адреса, link-local). Запросы с публичных IP
  отклоняются. Это можно отключить (`"allowWan": true` в `/opt/etc/usque-gui/gui.json`), но делать это
  не рекомендуется.
- Пароль хранится как PBKDF2-SHA256; вход ограничен 5 попытками на IP в минуту.
- Сессия в `HttpOnly`/`SameSite=Strict` cookie, изменяющие запросы требуют служебный заголовок (защита от CSRF).
- usque-gui работает от root (так же, как usque) — вызывает init-скрипт и читает состояние интерфейсов.
  Не пробрасывайте порт 2223 наружу.

Порт и адрес меняются в интерфейсе (раздел «Настройки интерфейса»), в `/opt/etc/usque-gui/gui.json`
(`"listen": "0.0.0.0:2223"`) или флагом `-listen` (флаг действует только на текущий запуск).

## Сборка

Нужен Go 1.24+.

```sh
make test        # go vet + тесты
make build       # локальный бинарник: out/usque-gui
make pkg-aarch64 # out/pkg/usque-gui_<версия>_aarch64-3.10.ipk
make feed        # все пакеты + opkg-репозиторий в out/_pages
```

Для отладки без роутера можно указать подставные пути:
`usque-gui -listen 127.0.0.1:2223 -config /tmp/gui.json -init-script … -usque-conf … -usque-pid …`.

## Релизы

Релиз создаётся GitHub Actions при изменении файла `VERSION` в ветке `main`: собираются `.ipk`,
публикуется GitHub Release и opkg-репозиторий на GitHub Pages
(Settings → Pages → Source: **GitHub Actions**).

## Лицензия

MIT
