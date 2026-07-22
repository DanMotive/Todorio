// Todorio — your private workspace for tasks and teams.
// https://github.com/DanMotive/Todorio · Apache 2.0 · Developed by Vlad
package main

import (
	"fmt"
	"os"

	"github.com/DanMotive/Todorio/internal/config"
	"github.com/DanMotive/Todorio/internal/ops"
	"github.com/DanMotive/Todorio/internal/server"
	"github.com/DanMotive/Todorio/internal/setup"
)

var version = "1.0.0-dev"

func usage() {
	fmt.Println(`Todorio ` + version + `

Команды:
  todorio setup                       Интерактивная первичная настройка
  todorio serve [--dev]               Запуск сервера
  todorio doctor                      Диагностика (сервис, БД, диск, SSL, бэкапы)
  todorio backup create               Создать бэкап
  todorio update                      Обновиться до последнего релиза
  todorio server config set K V       Настройки (default_locale, detect_browser_locale, ...)
  todorio server policy set K V       Политики (registration.mode, users.can_create_spaces, ...)
  todorio server limits set K V       Лимиты (uploads.max_file_size_mb, ...)
  todorio server branding set K V     Брендинг (site_name, browser_title, developer_name, ...)
  todorio server locales enable L     Включить локаль (например tr-TR)
  todorio version`)
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "version", "--version", "-v":
		fmt.Println("todorio", version)
	case "setup":
		if err := setup.Run(); err != nil {
			fail(err)
		}
	case "serve":
		cfg, err := config.Load()
		if err != nil {
			fail(fmt.Errorf("конфиг не найден — сначала выполните `todorio setup`: %w", err))
		}
		if err := server.Run(cfg, version); err != nil {
			fail(err)
		}
	case "doctor":
		cfg, _ := config.Load() // doctor работает и без конфига — покажет, чего не хватает
		if err := ops.Doctor(cfg, version); err != nil {
			fail(err)
		}
	case "backup":
		cfg, err := config.Load()
		if err != nil {
			fail(fmt.Errorf("конфиг не найден — сначала выполните `todorio setup`: %w", err))
		}
		if err := ops.Backup(cfg); err != nil {
			fail(err)
		}
	case "update":
		if err := ops.Update(version); err != nil {
			fail(err)
		}
	case "server":
		if err := config.RunCLI(os.Args[2:]); err != nil {
			fail(err)
		}
	default:
		usage()
		os.Exit(1)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "ошибка:", err)
	os.Exit(1)
}
