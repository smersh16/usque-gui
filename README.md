# usque-gui

Лёгкий веб-интерфейс для [usque-keenetic](https://github.com/side-effect-tm/usque-keenetic) —
неофициального клиента Cloudflare WARP (MASQUE) для роутеров Keenetic/Netcraze.

Один статический бинарник на Go (~6 МБ, без зависимостей), интерфейс встроен в него.
usque-gui **не заменяет** usque-keenetic и ничего в нём не меняет: он вызывает его
init-скрипт и правит его `usque.conf`.

## Возможности (v0.1)

- Статус: процесс, интерфейс в ядре, состояние интерфейса в NDMS (`state`/`link`/`connected`), адрес.
- Запуск, остановка, перезапуск (`/opt/etc/init.d/S51usque`).
- Редактор `usque.conf`: интерфейс, IP, маска, SNI, HTTP/2. Комментарии и `CONFIG_VERSION` сохраняются,
  значения строго валидируются (файл выполняется как shell), перед записью делается копия `usque.conf.gui-bak`.
- Аккаунт WARP: регистрация, перерегистрация (сброс + новая регистрация + перезапуск сервиса) и сброс сессии;
  запущенный сервис при этом останавливается автоматически. Содержимое `session.conf` в браузер не передаётся.
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

Порт и адрес меняются в `/opt/etc/usque-gui/gui.json` (`"listen": "0.0.0.0:2223"`) или флагом `-listen`.

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
