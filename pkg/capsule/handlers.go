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
			SenderID   int64  `json:"sender_id"`
			Content    string `json:"content"`
			Hours      int    `json:"hours"`
			Passcode   string `json:"passcode"`    // 4 цифры или пусто
			MediaType  string `json:"media_type"`  // "text", "photo", "voice"
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"Invalid payload"}`, 400)
			return
		}

		if req.MediaType == "" {
			req.MediaType = "text"
		}

		c := &Capsule{
			ID:               uuid.New().String(),
			SenderID:         req.SenderID,
			Content:          req.Content,
			UnlockAt:         time.Now().Add(time.Duration(req.Hours) * time.Hour),
			IsHacked:         false,
			Passcode:         req.Passcode,
			PasscodeAttempts: 3,
			MediaType:        req.MediaType,
			Reactions:        make(map[string]int),
			ReactionsUsers:   make(map[int64]string),
			ReminderSent:     false,
		}

		if err := store.Insert(c); err != nil {
			log.Println("DB Insert Error:", err)
			http.Error(w, `{"error":"DB Error"}`, 500)
			return
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":          c.ID,
			"has_passcode": c.Passcode != "",
		})
	}
}

// MakeGetHandler — GET /api/get?id=...&viewer_id=...
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

		// Если передан viewer_id — запоминаем получателя (первый открывший)
		if viewerStr := r.URL.Query().Get("viewer_id"); viewerStr != "" {
			var viewerID int64
			if _, scanErr := fmt.Sscanf(viewerStr, "%d", &viewerID); scanErr == nil && viewerID != 0 {
				_ = store.SetViewer(id, viewerID)
			}
		}

		// Собираем ответ
		resp := map[string]interface{}{
			"id":                c.ID,
			"sender_id":         c.SenderID,
			"unlock_at":         c.UnlockAt,
			"is_hacked":         c.IsHacked,
			"media_type":        c.MediaType,
			"has_passcode":      c.Passcode != "",
			"passcode_attempts": c.PasscodeAttempts,
			"reactions":         c.Reactions,
		}

		isUnlocked := time.Now().After(c.UnlockAt) || c.IsHacked
		if isUnlocked {
			resp["content"] = c.Content
		} else {
			resp["content"] = "Секрет надежно скрыт :)"
		}

		json.NewEncoder(w).Encode(resp)
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

// MakeReactionHandler — POST /api/reaction
func MakeReactionHandler(store Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     string `json:"id"`
			Emoji  string `json:"emoji"`
			UserID int64  `json:"user_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"Invalid payload"}`, 400)
			return
		}
		if req.ID == "" || req.Emoji == "" || req.UserID == 0 {
			http.Error(w, `{"error":"Missing id, emoji, or user_id"}`, 400)
			return
		}

		c, err := store.ToggleReaction(req.ID, req.UserID, req.Emoji)
		if err != nil {
			http.Error(w, `{"error":"Not Found"}`, 404)
			return
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"reactions": c.Reactions,
		})
	}
}

// MakePasscodeHandler — POST /api/passcode
func MakePasscodeHandler(store Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID       string `json:"id"`
			Passcode string `json:"passcode"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"Invalid payload"}`, 400)
			return
		}

		// Проверяем капсулу (один запрос к БД)
		c, err := store.GetByID(req.ID)
		if err != nil {
			http.Error(w, `{"error":"Not Found"}`, 404)
			return
		}

		if c.Passcode == "" {
			http.Error(w, `{"error":"No passcode set"}`, 400)
			return
		}

		if c.PasscodeAttempts <= 0 {
			http.Error(w, `{"error":"No attempts left","attempts":0}`, 403)
			return
		}

		if c.Passcode == req.Passcode {
			// Правильный код — взламываем бесплатно
			_, err := store.SetHacked(req.ID)
			if err != nil {
				http.Error(w, `{"error":"Server error"}`, 500)
				return
			}
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success":  true,
				"attempts": c.PasscodeAttempts - 1,
			})
			return
		}

		// Неправильный код
		remaining, err := store.IncrementPasscodeAttempts(req.ID)
		if err != nil {
			http.Error(w, `{"error":"Server error"}`, 500)
			return
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":  false,
			"attempts": remaining,
		})
	}
}

// MakeReminderHandler — POST /api/cron/reminders (для Vercel Cron / ручного вызова)
func MakeReminderHandler(store Store, bot *gotgbot.Bot) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		capsules, err := store.FindPendingReminders()
		if err != nil {
			log.Println("Reminder find error:", err)
			http.Error(w, `{"error":"DB Error"}`, 500)
			return
		}

		sent := 0
		for _, c := range capsules {
			// Отправляем получателю (viewer) если известен, иначе создателю
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
				continue
			}
			sent++
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"reminders_sent": sent,
		})
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
