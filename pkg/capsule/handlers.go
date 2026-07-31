package capsule

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/google/uuid"
)

// ValidateTWA проверяет подпись initData от Telegram WebApp
func ValidateTWA(initData string, botToken string) (map[string]string, bool) {
	if initData == "" || botToken == "" {
		return nil, false
	}
	vals, err := url.ParseQuery(initData)
	if err != nil {
		return nil, false
	}
	hash := vals.Get("hash")
	if hash == "" {
		return nil, false
	}
	vals.Del("hash")
	// Сортируем ключи и строим data-check-string
	keys := make([]string, 0, len(vals))
	for k := range vals {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		parts = append(parts, k+"="+vals.Get(k))
	}
	checkString := strings.Join(parts, "\n")
	// HMAC-SHA256 с bot_token как ключом
	secret := hmac.New(sha256.New, []byte("WebAppData"))
	secret.Write([]byte(botToken))
	h := hmac.New(sha256.New, secret.Sum(nil))
	h.Write([]byte(checkString))
	if hex.EncodeToString(h.Sum(nil)) != hash {
		return nil, false
	}
	result := make(map[string]string)
	for k, v := range vals {
		if len(v) > 0 {
			result[k] = v[0]
		}
	}
	return result, true
}

// parseUserFromTWA извлекает user.id из валидированных данных TWA
func parseUserFromTWA(data map[string]string) int64 {
	if userJSON := data["user"]; userJSON != "" {
		var u struct{ ID int64 `json:"id"` }
		if json.Unmarshal([]byte(userJSON), &u) == nil {
			return u.ID
		}
	}
	return 0
}

// RequireTWA — middleware для проверки initData
func RequireTWA(botToken string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		initData := r.Header.Get("X-Telegram-Init-Data")
		if initData == "" {
			initData = r.URL.Query().Get("init_data")
		}
		// В dev-режиме без BOT_TOKEN пропускаем
		if botToken == "" {
			next(w, r)
			return
		}
		if _, ok := ValidateTWA(initData, botToken); !ok {
			http.Error(w, `{"error":"Invalid Telegram signature"}`, 403)
			return
		}
		next(w, r)
	}
}

// --- HTTP API Handlers (возвращают http.HandlerFunc) ---

// MakeCreateHandler — POST /api/create
func MakeCreateHandler(store Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			SenderID     int64  `json:"sender_id"`
			Content      string `json:"content"`
			Hours        int    `json:"hours"`
			Passcode     string `json:"passcode"`     // 4 цифры или пусто
			MediaType    string `json:"media_type"`   // "text", "photo", "voice"
			HackPrice    int    `json:"hack_price"`   // цена взлома (0 = default 50)
			AllowHack    bool   `json:"allow_hack"`   // разрешить взлом за звёзды
			AllowHackSet bool   `json:"allow_hack_set"` // был ли явно установлен флаг
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
			UnlockAt:         time.Now().UTC().Add(time.Duration(req.Hours) * time.Hour),
			IsHacked:         false,
			Passcode:         req.Passcode,
			PasscodeAttempts: 3,
			MediaType:        req.MediaType,
			Reactions:        make(map[string]int),
			ReactionsUsers:   make(map[int64]string),
			ReminderSent:     false,
			HackPrice:        req.HackPrice,
			AllowHack:        req.AllowHack,
		}
		if c.HackPrice <= 0 {
			c.HackPrice = 50 // default
		}
		if !req.AllowHackSet {
			c.AllowHack = true // default
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
			"hack_price":        c.HackPrice,
			"allow_hack":        c.AllowHack,
		}

		isUnlocked := time.Now().UTC().After(c.UnlockAt) || c.IsHacked
		if isUnlocked {
			resp["content"] = c.Content
		} else if c.MediaType == "photo" {
			// Размытое превью для фото
			resp["content"] = c.Content
			resp["preview"] = true
		} else {
			resp["content"] = "Секрет надежно скрыт :)"
		}

		json.NewEncoder(w).Encode(resp)
	}
}

// MakeInvoiceHandler — GET /api/invoice?id=... (использует hack_price из капсулы)
func MakeInvoiceHandler(store Store, bot *gotgbot.Bot) http.HandlerFunc {
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
		if !c.AllowHack {
			http.Error(w, `{"error":"Hack not allowed"}`, 403)
			return
		}

		price := c.HackPrice
		if price <= 0 {
			price = 50
		}

		link, err := bot.CreateInvoiceLink(
			"Взлом капсулы",
			"Моментальный доступ к секрету!",
			id,    // Payload (ID капсулы)
			"XTR", // Валюта Telegram Stars
			[]gotgbot.LabeledPrice{{Label: "Взлом", Amount: int64(price)}},
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

// MakePasscodeHandler — POST /api/passcode (атомарный, без race condition)
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

		success, remaining, err := store.AttemptPasscode(req.ID, req.Passcode)
		if err != nil {
			http.Error(w, `{"error":"Server error"}`, 500)
			return
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":  success,
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
