package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"capsule_bot/pkg/capsule"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
)

func main() {
	token := os.Getenv("BOT_TOKEN")
	if token == "" {
		log.Fatal("Установи BOT_TOKEN в переменных окружения")
	}

	// 1. Инициализация MongoDB-хранилища
	mongoURI := os.Getenv("MONGO_URI")
	if mongoURI == "" {
		log.Fatal("Установи MONGO_URI в переменных окружения")
	}
	store, err := capsule.NewMongoStore(mongoURI, "capsule_app", "capsules")
	if err != nil {
		log.Fatal(err)
	}

	// Создаём индексы
	if err := store.CreateIndexes(); err != nil {
		log.Println("⚠️ Ошибка создания индексов:", err)
	}

	// 2. Инициализация Telegram бота
	bot, err := gotgbot.NewBot(token, nil)
	if err != nil {
		log.Fatal("Ошибка запуска бота:", err)
	}

	dispatcher := ext.NewDispatcher(&ext.DispatcherOpts{})
	updater := ext.NewUpdater(dispatcher, &ext.UpdaterOpts{})

	// Регистрируем платёжные хендлеры
	dispatcher.AddHandler(handlers.NewPreCheckoutQuery(nil, capsule.MakePreCheckoutHandler()))
	dispatcher.AddHandler(handlers.NewMessage(
		func(msg *gotgbot.Message) bool { return msg.SuccessfulPayment != nil },
		capsule.MakeSuccessfulPaymentHandler(store, bot),
	))

	// Long Polling
	err = updater.StartPolling(bot, &ext.PollingOpts{DropPendingUpdates: true})
	if err != nil {
		log.Fatal("Ошибка Polling:", err)
	}
	log.Printf("%s начал работу!", bot.User.Username)

	// 3. Фоновая горутина для напоминаний
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			capsules, err := store.FindPendingReminders()
			if err != nil {
				log.Println("Reminder check error:", err)
				continue
			}
			for _, c := range capsules {
				targetID := c.ViewerID
				if targetID == 0 {
					targetID = c.SenderID
				}
				msg := "🔔 Сейф откроется через час! Ты готов узнать секрет?"
				_, err := bot.SendMessage(targetID, msg, nil)
				if err != nil {
					log.Println("Reminder send error:", err)
					continue
				}
				if err := store.MarkReminderSent(c.ID); err != nil {
					log.Println("Reminder mark error:", err)
				}
			}
		}
	}()

	// 4. Web API роутинг
	// Статика: безопасно отдаём файлы с диска
	fs := http.FileServer(http.Dir("."))
	http.Handle("/safe.glb", fs)
	http.Handle("/safe-closed.glb", fs)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, "index.html")
	})
	http.HandleFunc("/api/create", capsule.MakeCreateHandler(store))
	http.HandleFunc("/api/get", capsule.MakeGetHandler(store))
	http.HandleFunc("/api/invoice", capsule.MakeInvoiceHandler(bot))
	http.HandleFunc("/api/reaction", capsule.MakeReactionHandler(store))
	http.HandleFunc("/api/passcode", capsule.MakePasscodeHandler(store))
	http.HandleFunc("/api/cron/reminders", capsule.MakeReminderHandler(store, bot))

	log.Println("Web-сервер запущен на порту :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}