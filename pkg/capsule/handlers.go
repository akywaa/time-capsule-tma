package capsule

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/google/uuid"
)

// --- HTTP API Handlers (возвращают http.HandlerFunc) ---

// MakeCreateHandler — POST /api/create
func MakeCreateHandler(store Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			SenderID int64  `json:"sender_id"`
			Content  string `json:"content"`
			Hours    int    `json:"hours"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"Invalid payload"}`, 400)
			return
		}

		c := &Capsule{
			ID:       uuid.New().String(),
			SenderID: req.SenderID,
			Content:  req.Content,
			UnlockAt: time.Now().Add(time.Duration(req.Hours) * time.Hour),
			IsHacked: false,
		}

		if err := store.Insert(c); err != nil {
			log.Println("DB Insert Error:", err)
			http.Error(w, `{"error":"DB Error"}`, 500)
			return
		}

		json.NewEncoder(w).Encode(map[string]string{"id": c.ID})
	}
}

// MakeGetHandler — GET /api/get?id=...
func MakeGetHandler(store Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, `{"error":"Missing ID"}`, 400)
			return
		}

		c, err := store.GetByID(id)
		if err != nil {
			http.Error(w, `{"error":"Not Found"}`, 404)
			return
		}

		// Скрываем контент, если время не пришло и сейф не взломан
		if time.Now().Before(c.UnlockAt) && !c.IsHacked {
			c.Content = "Секрет надежно скрыт :)"
		}

		json.NewEncoder(w).Encode(c)
	}
}

// MakeInvoiceHandler — GET /api/invoice?id=...
func MakeInvoiceHandler(bot *gotgbot.Bot) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")

		link, err := bot.CreateInvoiceLink(
			"Взлом капсулы",
			"Моментальный доступ к секрету!",
			id,    // Payload (ID капсулы)
			"XTR", // Валюта Telegram Stars
			[]gotgbot.LabeledPrice{{Label: "Взлом", Amount: 50}},
			nil,
		)
		if err != nil {
			log.Println("Invoice error:", err)
			http.Error(w, `{"error":"Invoice generation failed"}`, 500)
			return
		}

		json.NewEncoder(w).Encode(map[string]string{"url": link})
	}
}

// --- Telegram Payment Handlers ---

// MakePreCheckoutHandler — одобряет попытку оплаты
func MakePreCheckoutHandler() func(*gotgbot.Bot, *ext.Context) error {
	return func(b *gotgbot.Bot, ctx *ext.Context) error {
		_, err := b.AnswerPreCheckoutQuery(ctx.PreCheckoutQuery.Id, true, nil)
		return err
	}
}

// MakeSuccessfulPaymentHandler — обрабатывает успешный платёж и шлёт пуш отправителю
func MakeSuccessfulPaymentHandler(store Store, bot *gotgbot.Bot) func(*gotgbot.Bot, *ext.Context) error {
	return func(b *gotgbot.Bot, ctx *ext.Context) error {
		capsuleID := ctx.Message.SuccessfulPayment.InvoicePayload

		hackerUsername := ctx.EffectiveUser.Username
		if hackerUsername == "" {
			hackerUsername = ctx.EffectiveUser.FirstName
		}

		c, err := store.SetHacked(capsuleID)
		if err != nil {
			log.Println("SetHacked error:", err)
			return nil
		}

		msg := fmt.Sprintf("Ахах, твой друг @%s не выдержал и взломал твою капсулу за деньги! 🌟", hackerUsername)
		_, err = b.SendMessage(c.SenderID, msg, nil)
		if err != nil {
			log.Println("SendMessage error:", err)
		}

		return nil
	}
}
