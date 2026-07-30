package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

var db *sql.DB
var bot *gotgbot.Bot

// Структура нашей капсулы
type Capsule struct {
	ID        string    `json:"id"`
	SenderID  int64     `json:"sender_id"`
	Content   string    `json:"content"`
	UnlockAt  time.Time `json:"unlock_at"`
	IsHacked  bool      `json:"is_hacked"`
}

func main() {
	token := os.Getenv("BOT_TOKEN")
	if token == "" {
		log.Fatal("Установи BOT_TOKEN в переменных окружения")
	}

	// 1. Инициализация БД SQLite
	var err error
	db, err = sql.Open("sqlite", "capsules.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS capsules (
		id TEXT PRIMARY KEY,
		sender_id INTEGER,
		content TEXT,
		unlock_at DATETIME,
		is_hacked BOOLEAN DEFAULT FALSE
	)`)
	if err != nil {
		log.Fatal(err)
	}

	// 2. Инициализация Telegram Бота
	bot, err = gotgbot.NewBot(token, nil)
	if err != nil {
		log.Fatal("Ошибка запуска бота:", err)
	}

	dispatcher := ext.NewDispatcher(&ext.DispatcherOpts{})
	updater := ext.NewUpdater(&ext.UpdaterOpts{Dispatcher: dispatcher})

	// Обработчики платежей Telegram Stars
	dispatcher.AddHandler(handlers.NewPreCheckoutQuery(nil, preCheckoutHandler))
	dispatcher.AddHandler(handlers.NewMessage(
		func(msg *gotgbot.Message) bool { return msg.SuccessfulPayment != nil },
		successfulPaymentHandler,
	))

	// Запускаем бота (Long Polling)
	err = updater.StartPolling(bot, &ext.PollingOpts{DropPendingUpdates: true})
	if err != nil {
		log.Fatal("Ошибка Polling:", err)
	}
	log.Printf("%s начал работу!", bot.User.Username)

	// 3. Роутинг Web API (net/http)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "index.html")
	})

	http.HandleFunc("/api/create", apiCreateCapsule)
	http.HandleFunc("/api/get", apiGetCapsule)
	http.HandleFunc("/api/invoice", apiGenerateInvoice)

	log.Println("Web-сервер запущен на порту :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

// --- API Endpoints ---

func apiCreateCapsule(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SenderID int64  `json:"sender_id"`
		Content  string `json:"content"`
		Hours    int    `json:"hours"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	id := uuid.New().String()
	unlockAt := time.Now().Add(time.Duration(req.Hours) * time.Hour)

	_, err := db.Exec("INSERT INTO capsules (id, sender_id, content, unlock_at) VALUES (?, ?, ?, ?)",
		id, req.SenderID, req.Content, unlockAt)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"id": id})
}

func apiGetCapsule(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	var c Capsule
	err := db.QueryRow("SELECT id, sender_id, content, unlock_at, is_hacked FROM capsules WHERE id = ?", id).
		Scan(&c.ID, &c.SenderID, &c.Content, &c.UnlockAt, &c.IsHacked)
	if err != nil {
		http.Error(w, "Капсула не найдена", 404)
		return
	}

	// Если капсула заблокирована и не взломана, скрываем контент от фронтенда!
	if time.Now().Before(c.UnlockAt) && !c.IsHacked {
		c.Content = "Секрет надежно скрыт :)"
	}

	json.NewEncoder(w).Encode(c)
}

func apiGenerateInvoice(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")

	// Генерируем ссылку на оплату Stars. Валюта "XTR" — это Telegram Stars.
	link, err := bot.CreateInvoiceLink(
		"Взлом капсулы",
		"Получи доступ к секрету моментально!",
		id, // Payload - передаем ID капсулы
		"", // Provider token для Stars не нужен, оставляем пустым
		"XTR",
		[]gotgbot.LabeledPrice{{Label: "Взлом", Amount: 50}},
		nil,
	)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"url": link})
}

// --- Telegram Handlers ---

// Разрешаем транзакцию
func preCheckoutHandler(b *gotgbot.Bot, ctx *ext.Context) error {
	_, err := b.AnswerPreCheckoutQuery(ctx.PreCheckoutQuery.Id, true, nil)
	return err
}

// Обработка успешного платежа и эмоциональный луп
func successfulPaymentHandler(b *gotgbot.Bot, ctx *ext.Context) error {
	capsuleID := ctx.Message.SuccessfulPayment.InvoicePayload
	hackerUsername := ctx.EffectiveUser.Username
	if hackerUsername == "" {
		hackerUsername = ctx.EffectiveUser.FirstName
	}

	// Обновляем статус в БД
	_, err := db.Exec("UPDATE capsules SET is_hacked = true WHERE id = ?", capsuleID)
	if err != nil {
		return err
	}

	// Достаем ID отправителя
	var senderID int64
	db.QueryRow("SELECT sender_id FROM capsules WHERE id = ?", capsuleID).Scan(&senderID)

	// ОТПРАВЛЯЕМ ТОТ САМЫЙ ПУШ ОТПРАВИТЕЛЮ
	msg := fmt.Sprintf("Ахах, твой друг @%s не выдержал и взломал капсулу за деньги! 🌟", hackerUsername)
	b.SendMessage(senderID, msg, nil)

	return nil
}