package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

// version подставляется при сборке: -ldflags "-X main.version=..."
var version = "dev"

const defaultGUIConfig = "/opt/etc/usque-gui/gui.json"

func main() {
	p := defaultPaths()
	fs := flag.NewFlagSet("usque-gui", flag.ExitOnError)
	cfgPath := fs.String("config", defaultGUIConfig, "файл настроек GUI")
	listen := fs.String("listen", "", "адрес прослушивания (по умолчанию из настроек, иначе "+defaultListen+")")
	showVer := fs.Bool("version", false, "показать версию")
	fs.StringVar(&p.InitScript, "init-script", p.InitScript, "init-скрипт usque-keenetic")
	fs.StringVar(&p.UsqueBin, "usque-bin", p.UsqueBin, "бинарник usque")
	fs.StringVar(&p.Conf, "usque-conf", p.Conf, "usque.conf")
	fs.StringVar(&p.Session, "usque-session", p.Session, "session.conf")
	fs.StringVar(&p.PidFile, "usque-pid", p.PidFile, "pid-файл usque")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: usque-gui [flags] [passwd]\n\n  passwd   задать или сменить пароль веб-интерфейса\n\n")
		fs.PrintDefaults()
	}
	_ = fs.Parse(os.Args[1:])

	if *showVer {
		fmt.Println("usque-gui", version)
		return
	}

	cfg, err := loadGUIConfig(*cfgPath)
	if err != nil {
		log.Fatalf("настройки %s: %v", *cfgPath, err)
	}
	if *listen != "" {
		cfg.Listen = *listen
	}
	auth := NewAuth(*cfgPath, cfg)

	if fs.Arg(0) == "passwd" {
		if err := cmdPasswd(auth); err != nil {
			log.Fatal(err)
		}
		return
	}

	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           NewApp(p, auth).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      6 * time.Minute, // register/restart/opkg upgrade могут быть долгими
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(sctx)
	}()

	log.Printf("usque-gui %s слушает %s", version, cfg.Listen)
	if auth.NeedSetup() {
		log.Printf("пароль не задан: откройте веб-интерфейс и задайте его (или: usque-gui passwd)")
	}
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func cmdPasswd(a *Auth) error {
	fmt.Print("Новый пароль: ")
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		return err
	}
	if err := a.SetPassword(strings.TrimRight(line, "\r\n")); err != nil {
		return err
	}
	fmt.Println("Пароль сохранён.")
	return nil
}
