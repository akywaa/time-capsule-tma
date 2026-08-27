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

func initServices() error {
	initOnce.Do(func() {
		mongoURI := os.Getenv("MONGO_URI")
		if mongoURI == "" {
			initErr = fmt.Errorf("MONGO_URI not set")
			return
		}

		store, initErr = capsule.NewMongoStore(mongoURI, "capsule_app", "capsules")
		if initErr != nil {
			initErr = fmt.Errorf("failed to connect to mongodb: %w", initErr)
			return
		}

		if err := store.CreateIndexes(); err != nil {
			log.Printf("warning: index creation failed: %v", err)
		}

		botToken := os.Getenv("BOT_TOKEN")
		if botToken == "" {
			initErr = fmt.Errorf("BOT_TOKEN not set")
			return
		}

		bot, initErr = gotgbot.NewBot(botToken, nil)
		if initErr != nil {
			initErr = fmt.Errorf("failed to init tg bot: %w", initErr)
			return
		}

		dispatcher = ext.NewDispatcher(&ext.DispatcherOpts{})
		dispatcher.AddHandler(handlers.NewPreCheckoutQuery(nil, capsule.MakePreCheckoutHandler()))
		dispatcher.AddHandler(handlers.NewMessage(
			func(msg *gotgbot.Message) bool { return msg.SuccessfulPayment != nil },
			capsule.MakeSuccessfulPaymentHandler(store, bot),
		))
	})
	return initErr
}

func Handler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	if err := initServices(); err != nil {
		log.Printf("critical: service initialization failed: %v", err)
		http.Error(w, fmt.Sprintf(`{"error":"Internal Server Error"}`), http.StatusInternalServerError)
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
	case strings.HasPrefix(r.URL.Path, "/api/contribute"):
		capsule.RequireTWA(bot.Token, capsule.MakeContributeHandler(store))(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/geo-check"):
		capsule.RequireTWA(bot.Token, capsule.MakeGeoCheckHandler(store))(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/my"):
		capsule.MakeMyCapsulesHandler(store)(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/cron/reminders"):
		capsule.MakeReminderHandler(store, bot)(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/webhook"):
		apiWebhook(w, r)
	default:
		http.Error(w, `{"error":"Not Found"}`, http.StatusNotFound)
	}
}

func apiWebhook(w http.ResponseWriter, r *http.Request) {
	var update gotgbot.Update
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		log.Printf("error decoding webhook payload: %v", err)
		http.Error(w, `{"error":"Bad Request"}`, http.StatusBadRequest)
		return
	}
	dispatcher.ProcessUpdate(bot, &update, nil)
	w.WriteHeader(http.StatusOK)
}