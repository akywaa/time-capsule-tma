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
	"os"
	"sort"
	"strings"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/google/uuid"
)

// ValidateTWA
// TODO: вынести куда то логику валидации
// https://core.telegram.org/bots/webapps#validating-data-received-via-the-web-app
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

func parseUserFromTWA(data map[string]string) int64 {
	if userJSON := data["user"]; userJSON != "" {
		var u struct {
			ID int64 `json:"id"`
		}
		if json.Unmarshal([]byte(userJSON), &u) == nil {
			return u.ID
		}
	}
	return 0
}

func RequireTWA(botToken string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		initData := r.Header.Get("X-Telegram-Init-Data")
		if initData == "" {
			initData = r.URL.Query().Get("init_data")
		}
		if botToken == "" {
			next(w, r)
			return
		}
		if _, ok := ValidateTWA(initData, botToken); !ok {
			http.Error(w, `{"error":"Unauthorized"}`, http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

func MakeCreateHandler(store Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			SenderID     int64   `json:"sender_id"`
			Content      string  `json:"content"`
			Hours        int     `json:"hours"`
			Passcode     string  `json:"passcode"`
			MediaType    string  `json:"media_type"`
			HackPrice    int     `json:"hack_price"`
			AllowHack    bool    `json:"allow_hack"`
			AllowHackSet bool    `json:"allow_hack_set"`
			CapsuleType  string  `json:"capsule_type"`
			GoalStars    int     `json:"goal_stars"`
			GeoLat       float64 `json:"geo_lat"`
			GeoLng       float64 `json:"geo_lng"`
			GeoRadius    int     `json:"geo_radius"`
			ModelType    string  `json:"model_type"`
		}

		// log.Println("try to create capsule...") // дебаг
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"Invalid request payload"}`, http.StatusBadRequest)
			return
		}

		if req.MediaType == "" {
			req.MediaType = "text"
		}
		if req.CapsuleType == "" {
			req.CapsuleType = "personal"
		}
		if req.ModelType == "" {
			req.ModelType = "safe"
		}

		c := &Capsule{
			ID:                 uuid.New().String(),
			SenderID:           req.SenderID,
			Content:            req.Content,
			UnlockAt:           time.Now().UTC().Add(time.Duration(req.Hours) * time.Hour),
			IsHacked:           false,
			Passcode:           req.Passcode,
			PasscodeAttempts:   3,
			MediaType:          req.MediaType,
			Reactions:          make(map[string]int),
			ReactionsUsers:     make(map[int64]string),
			ReminderSent:       false,
			HackPrice:          req.HackPrice,
			AllowHack:          req.AllowHack,
			CapsuleType:        req.CapsuleType,
			GoalStars:          req.GoalStars,
			StarsContributions: make(map[int64]int),
			GeoLat:             req.GeoLat,
			GeoLng:             req.GeoLng,
			GeoRadius:          req.GeoRadius,
			ModelType:          req.ModelType,
		}

		if c.HackPrice <= 0 {
			c.HackPrice = 50
		}
		if !req.AllowHackSet {
			c.AllowHack = true
		}

		if err := store.Insert(c); err != nil {
			log.Printf("db insert error: %v", err)
			http.Error(w, `{"error":"Internal Server Error"}`, http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":           c.ID,
			"has_passcode": c.Passcode != "",
		})
	}
}

func MakeGetHandler(store Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, `{"error":"Missing capsule ID"}`, http.StatusBadRequest)
			return
		}

		c, err := store.GetByID(id)
		if err != nil {
			http.Error(w, `{"error":"Capsule not found"}`, http.StatusNotFound)
			return
		}

		if viewerStr := r.URL.Query().Get("viewer_id"); viewerStr != "" {
			var viewerID int64
			if _, scanErr := fmt.Sscanf(viewerStr, "%d", &viewerID); scanErr == nil && viewerID != 0 {
				_ = store.SetViewer(id, viewerID)
			}
		}

		resp := map[string]interface{}{
			"id":                  c.ID,
			"sender_id":           c.SenderID,
			"unlock_at":           c.UnlockAt,
			"is_hacked":           c.IsHacked,
			"media_type":          c.MediaType,
			"has_passcode":        c.Passcode != "",
			"passcode_attempts":   c.PasscodeAttempts,
			"reactions":           c.Reactions,
			"hack_price":          c.HackPrice,
			"allow_hack":          c.AllowHack,
			"capsule_type":        c.CapsuleType,
			"goal_stars":          c.GoalStars,
			"stars_contributions": c.StarsContributions,
			"geo_lat":             c.GeoLat,
			"geo_lng":             c.GeoLng,
			"geo_radius":          c.GeoRadius,
			"model_type":          c.ModelType,
		}

		isUnlocked := time.Now().UTC().After(c.UnlockAt) || c.IsHacked
		if isUnlocked {
			resp["content"] = c.Content
		} else if c.MediaType == "photo" {
			resp["content"] = c.Content
			resp["preview"] = true
		} else {
			resp["content"] = "Secret is safely hidden :)"
		}

		json.NewEncoder(w).Encode(resp)
	}
}

func MakeInvoiceHandler(store Store, bot *gotgbot.Bot) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, `{"error":"Missing capsule ID"}`, http.StatusBadRequest)
			return
		}

		c, err := store.GetByID(id)
		if err != nil {
			http.Error(w, `{"error":"Capsule not found"}`, http.StatusNotFound)
			return
		}
		if !c.AllowHack {
			http.Error(w, `{"error":"Hack not allowed"}`, http.StatusForbidden)
			return
		}

		price := c.HackPrice
		if price <= 0 {
			price = 50
		}

		link, err := bot.CreateInvoiceLink(
			"Capsule Hack",
			"Instant access to the secret!",
			id,
			"XTR",
			[]gotgbot.LabeledPrice{{Label: "Hack", Amount: int64(price)}},
			nil,
		)
		if err != nil {
			log.Printf("invoice creation failed: %v", err)
			http.Error(w, `{"error":"Failed to generate invoice"}`, http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(map[string]string{"url": link})
	}
}

func MakeReactionHandler(store Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     string `json:"id"`
			Emoji  string `json:"emoji"`
			UserID int64  `json:"user_id"`
		}
		
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"Invalid payload"}`, http.StatusBadRequest)
			return
		}
		if req.ID == "" || req.Emoji == "" || req.UserID == 0 {
			http.Error(w, `{"error":"Missing required fields"}`, http.StatusBadRequest)
			return
		}

		c, err := store.ToggleReaction(req.ID, req.UserID, req.Emoji)
		if err != nil {
			http.Error(w, `{"error":"Not Found"}`, http.StatusNotFound)
			return
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"reactions": c.Reactions,
		})
	}
}

func MakePasscodeHandler(store Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID       string `json:"id"`
			Passcode string `json:"passcode"`
		}
		
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"Invalid payload"}`, http.StatusBadRequest)
			return
		}

		success, remaining, err := store.AttemptPasscode(req.ID, req.Passcode)
		if err != nil {
			log.Printf("passcode error: %v", err)
			http.Error(w, `{"error":"Internal Server Error"}`, http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":  success,
			"attempts": remaining,
		})
	}
}

func MakeContributeHandler(store Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     string `json:"id"`
			UserID int64  `json:"user_id"`
			Amount int    `json:"amount"`
		}
		
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"Invalid payload"}`, http.StatusBadRequest)
			return
		}
		
		c, err := store.Contribute(req.ID, req.UserID, req.Amount)
		if err != nil {
			log.Printf("contribution failed: %v", err)
			http.Error(w, `{"error":"Failed to process contribution"}`, http.StatusInternalServerError)
			return
		}
		
		total := 0
		for _, v := range c.StarsContributions {
			total += v
		}
		
		json.NewEncoder(w).Encode(map[string]interface{}{
			"is_hacked":           c.IsHacked,
			"stars_contributions": c.StarsContributions,
			"goal_stars":          c.GoalStars,
			"total":               total,
		})
	}
}

func MakeGeoCheckHandler(store Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID  string  `json:"id"`
			Lat float64 `json:"lat"`
			Lng float64 `json:"lng"`
		}
		
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"Invalid payload"}`, http.StatusBadRequest)
			return
		}
		
		unlocked, dist, err := store.GeoCheck(req.ID, req.Lat, req.Lng)
		if err != nil {
			http.Error(w, `{"error":"Not Found"}`, http.StatusNotFound)
			return
		}
		
		json.NewEncoder(w).Encode(map[string]interface{}{
			"unlocked": unlocked,
			"distance": dist,
		})
	}
}

func MakeMyCapsulesHandler(store Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		initData := r.Header.Get("X-Telegram-Init-Data")
		data, ok := ValidateTWA(initData, os.Getenv("BOT_TOKEN"))
		
		if !ok {
			http.Error(w, `{"error":"Unauthorized"}`, http.StatusForbidden)
			return
		}
		
		userID := parseUserFromTWA(data)
		if userID == 0 {
			http.Error(w, `{"error":"Invalid user context"}`, http.StatusBadRequest)
			return
		}
		
		capsules, err := store.FindBySenderID(userID)
		if err != nil {
			log.Printf("error fetching capsules for %d: %v", userID, err)
			json.NewEncoder(w).Encode([]interface{}{})
			return
		}
		
		json.NewEncoder(w).Encode(capsules)
	}
}

func MakeReminderHandler(store Store, bot *gotgbot.Bot) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		capsules, err := store.FindPendingReminders()
		if err != nil {
			log.Printf("cron reminder db error: %v", err)
			http.Error(w, `{"error":"Database error"}`, http.StatusInternalServerError)
			return
		}

		sent := 0
		for _, c := range capsules {
			targetID := c.ViewerID
			if targetID == 0 {
				targetID = c.SenderID
			}
			
			msg := "🔔 Safe opens in 1 hour! Ready to see the secret?"
			if _, err := bot.SendMessage(targetID, msg, nil); err != nil {
				log.Printf("failed to send reminder to %d: %v", targetID, err)
				continue
			}
			
			if err := store.MarkReminderSent(c.ID); err != nil {
				log.Printf("failed to mark reminder sent for %s: %v", c.ID, err)
				continue
			}
			sent++
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"reminders_sent": sent,
		})
	}
}

func MakePreCheckoutHandler() func(*gotgbot.Bot, *ext.Context) error {
	return func(b *gotgbot.Bot, ctx *ext.Context) error {
		_, err := b.AnswerPreCheckoutQuery(ctx.PreCheckoutQuery.Id, true, nil)
		return err
	}
}

func MakeSuccessfulPaymentHandler(store Store, bot *gotgbot.Bot) func(*gotgbot.Bot, *ext.Context) error {
	return func(b *gotgbot.Bot, ctx *ext.Context) error {
		capsuleID := ctx.Message.SuccessfulPayment.InvoicePayload
		hackerUsername := ctx.EffectiveUser.Username
		
		if hackerUsername == "" {
			hackerUsername = ctx.EffectiveUser.FirstName
		}

		c, err := store.SetHacked(capsuleID)
		if err != nil {
			log.Printf("payment success, but failed to set hack status for %s: %v", capsuleID, err)
			return nil
		}

		msg := fmt.Sprintf("Haha, your friend @%s couldn't resist and hacked your capsule for money! 🌟", hackerUsername)
		if _, err = b.SendMessage(c.SenderID, msg, nil); err != nil {
			log.Printf("failed to send hack notification to sender: %v", err)
		}

		return nil
	}
}