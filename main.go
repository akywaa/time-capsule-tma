package main

import (
	"log"
	"net/http"
	"os"

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

	// 1. Инициализация SQLite-хранилища
	store, err := capsule.NewSQLiteStore("capsules.db")
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()

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

	// 3. Web API роутинг
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "index.html")
	})
	http.HandleFunc("/api/create", capsule.MakeCreateHandler(store))
	http.HandleFunc("/api/get", capsule.MakeGetHandler(store))
	http.HandleFunc("/api/invoice", capsule.MakeInvoiceHandler(bot))

	log.Println("Web-сервер запущен на порту :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}