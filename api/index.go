package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"

	"capsule_bot/pkg/capsule"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
)

var (
	initOnce   sync.Once
	store      capsule.Store
	bot        *gotgbot.Bot
	dispatcher *ext.Dispatcher
	initErr    error
)

// initServices — ленивая инициализация MongoDB и Telegram бота (однократно)
func initServices() error {
	initOnce.Do(func() {
		// 1. MongoDB
		mongoURI := os.Getenv("MONGO_URI")
		if mongoURI == "" {
			initErr = fmt.Errorf("переменная MONGO_URI пустая")
			return
		}
		store, initErr = capsule.NewMongoStore(mongoURI, "capsule_app", "capsules")
		if initErr != nil {
			initErr = fmt.Errorf("ошибка подключения к Mongo: %v", initErr)
			return
		}

		// Создаём индексы (не фатально при ошибке)
		if err := store.CreateIndexes(); err != nil {
			log.Println("⚠️ Ошибка создания индексов:", err)
		}

		// 2. Telegram Bot
		botToken := os.Getenv("BOT_TOKEN")
		if botToken == "" {
			initErr = fmt.Errorf("переменная BOT_TOKEN пустая")
			return
		}
		bot, initErr = gotgbot.NewBot(botToken, nil)
		if initErr != nil {
			initErr = fmt.Errorf("ошибка запуска бота: %v", initErr)
			return
		}

		// 3. Диспетчер + платёжные хендлеры
		dispatcher = ext.NewDispatcher(&ext.DispatcherOpts{})
		dispatcher.AddHandler(handlers.NewPreCheckoutQuery(nil, capsule.MakePreCheckoutHandler()))
		dispatcher.AddHandler(handlers.NewMessage(
			func(msg *gotgbot.Message) bool { return msg.SuccessfulPayment != nil },
			capsule.MakeSuccessfulPaymentHandler(store, bot),
		))
	})
	return initErr
}

// Handler — точка входа для Vercel serverless
func Handler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	if err := initServices(); err != nil {
		log.Println("Service Init Error:", err)
		http.Error(w, fmt.Sprintf(`{"error":"Init failed: %s"}`, err.Error()), 500)
		return
	}

	switch {
	case strings.HasPrefix(r.URL.Path, "/api/create"):
		capsule.RequireTWA(bot.Token, capsule.MakeCreateHandler(store))(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/get"):
		capsule.MakeGetHandler(store)(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/invoice"):
		capsule.RequireTWA(bot.Token, capsule.MakeInvoiceHandler(store, bot))(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/reaction"):
		capsule.RequireTWA(bot.Token, capsule.MakeReactionHandler(store))(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/passcode"):
		capsule.RequireTWA(bot.Token, capsule.MakePasscodeHandler(store))(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/cron/reminders"):
		capsule.MakeReminderHandler(store, bot)(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/webhook"):
		apiWebhook(w, r)
	default:
		http.Error(w, `{"error":"Endpoint Not Found"}`, 404)
	}
}

// apiWebhook — приём вебхуков от Telegram
func apiWebhook(w http.ResponseWriter, r *http.Request) {
	var update gotgbot.Update
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		http.Error(w, `{"error":"Bad Request"}`, 400)
		return
	}
	dispatcher.ProcessUpdate(bot, &update, nil)
	w.WriteHeader(http.StatusOK)
}