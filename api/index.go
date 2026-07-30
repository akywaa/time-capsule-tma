package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	bot        *gotgbot.Bot
	dispatcher *ext.Dispatcher
	capsulesDB *mongo.Collection
)

// Структура документа в MongoDB
type Capsule struct {
	ID        string    `json:"id" bson:"_id"`
	SenderID  int64     `json:"sender_id" bson:"sender_id"`
	Content   string    `json:"content" bson:"content"`
	UnlockAt  time.Time `json:"unlock_at" bson:"unlock_at"`
	IsHacked  bool      `json:"is_hacked" bson:"is_hacked"`
}

// Ленивая инициализация: проверяем и подключаем сервисы только если они еще не созданы
func initServices() error {
	// 1. Подключение к MongoDB
	if capsulesDB == nil {
		mongoURI := os.Getenv("MONGO_URI")
		if mongoURI == "" {
			return fmt.Errorf("переменная MONGO_URI пустая")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		clientOptions := options.Client().ApplyURI(mongoURI)
		client, err := mongo.Connect(ctx, clientOptions)
		if err != nil {
			return fmt.Errorf("ошибка подключения к Mongo: %v", err)
		}
		
		// Обязательно "пингуем" базу, чтобы убедиться, что логин/пароль верные
		if err := client.Ping(ctx, nil); err != nil {
			return fmt.Errorf("ошибка пинга Mongo: %v", err)
		}
		capsulesDB = client.Database("capsule_app").Collection("capsules")
	}

	// 2. Инициализация Telegram Бота
	if bot == nil {
		botToken := os.Getenv("BOT_TOKEN")
		if botToken == "" {
			return fmt.Errorf("переменная BOT_TOKEN пустая")
		}
		b, err := gotgbot.NewBot(botToken, nil)
		if err != nil {
			return fmt.Errorf("ошибка запуска бота: %v", err)
		}
		bot = b
		dispatcher = ext.NewDispatcher(&ext.DispatcherOpts{})
		
		// Обработчики платежей Stars
		dispatcher.AddHandler(handlers.NewPreCheckoutQuery(nil, preCheckoutHandler))
		dispatcher.AddHandler(handlers.NewMessage(
			func(msg *gotgbot.Message) bool { return msg.SuccessfulPayment != nil },
			successfulPaymentHandler,
		))
	}

	return nil
}

// Handler — точка входа для всех запросов /api/* в Vercel
func Handler(w http.ResponseWriter, r *http.Request) {
	// CORS заголовки
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	// ГАРАНТИРУЕМ, что база и бот подключены перед обработкой запроса
	if err := initServices(); err != nil {
		log.Println("Service Init Error:", err)
		// Возвращаем точную ошибку на фронтенд, чтобы было видно, в чем проблема
		http.Error(w, fmt.Sprintf(`{"error": "Init failed: %s"}`, err.Error()), 500)
		return
	}

	// Наш мини-роутер
	switch {
	case strings.HasPrefix(r.URL.Path, "/api/create"):
		apiCreateCapsule(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/get"):
		apiGetCapsule(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/invoice"):
		apiGenerateInvoice(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/webhook"):
		apiWebhook(w, r)
	default:
		http.Error(w, `{"error": "Endpoint Not Found"}`, 404)
	}
}

// --- API Endpoints ---

func apiCreateCapsule(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SenderID int64  `json:"sender_id"`
		Content  string `json:"content"`
		Hours    int    `json:"hours"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "Invalid payload"}`, 400)
		return
	}

	capsule := Capsule{
		ID:       uuid.New().String(),
		SenderID: req.SenderID,
		Content:  req.Content,
		UnlockAt: time.Now().Add(time.Duration(req.Hours) * time.Hour),
		IsHacked: false,
	}

	_, err := capsulesDB.InsertOne(context.TODO(), capsule)
	if err != nil {
		log.Println("DB Insert Error:", err)
		http.Error(w, `{"error": "DB Error"}`, 500)
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"id": capsule.ID})
}

func apiGetCapsule(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, `{"error": "Missing ID"}`, 400)
		return
	}

	var c Capsule
	err := capsulesDB.FindOne(context.TODO(), bson.M{"_id": id}).Decode(&c)
	if err != nil {
		http.Error(w, `{"error": "Not Found"}`, 404)
		return
	}

	// Если время не пришло и сейф не взломан, скрываем контент
	if time.Now().Before(c.UnlockAt) && !c.IsHacked {
		c.Content = "Секрет надежно скрыт :)"
	}

	json.NewEncoder(w).Encode(c)
}

func apiGenerateInvoice(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")

	// Генерируем ссылку на оплату Stars. Валюта "XTR"
	link, err := bot.CreateInvoiceLink(
		"Взлом капсулы",
		"Моментальный доступ к секрету!",
		id,      // Payload (наш id)
		"XTR",   // Currency (Валюта)
		[]gotgbot.LabeledPrice{{Label: "Взлом", Amount: 50}},
		nil,     // Дополнительные опции
	)
	if err != nil {
		log.Println("Ошибка генерации инвойса:", err)
		http.Error(w, `{"error": "Invoice generation failed"}`, 500)
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"url": link})
}

// Эндпоинт для приема вебхуков от Telegram
func apiWebhook(w http.ResponseWriter, r *http.Request) {
	var update gotgbot.Update
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		http.Error(w, `{"error": "Bad Request"}`, 400)
		return
	}
	
	// Передаем апдейт в диспетчер на обработку (платежи)
	dispatcher.ProcessUpdate(bot, &update, nil)
	
	// Обязательно возвращаем 200 OK Телеграму
	w.WriteHeader(http.StatusOK)
}

// --- Telegram Handlers (Логика монетизации) ---

func preCheckoutHandler(b *gotgbot.Bot, ctx *ext.Context) error {
	// Одобряем попытку оплаты
	_, err := b.AnswerPreCheckoutQuery(ctx.PreCheckoutQuery.Id, true, nil)
	return err
}

func successfulPaymentHandler(b *gotgbot.Bot, ctx *ext.Context) error {
	capsuleID := ctx.Message.SuccessfulPayment.InvoicePayload
	
	hackerUsername := ctx.EffectiveUser.Username
	if hackerUsername == "" {
		hackerUsername = ctx.EffectiveUser.FirstName
	}

	// 1. Обновляем статус капсулы в MongoDB (Взломана = true)
	var c Capsule
	err := capsulesDB.FindOneAndUpdate(
		context.TODO(),
		bson.M{"_id": capsuleID},
		bson.M{"$set": bson.M{"is_hacked": true}},
	).Decode(&c)
	
	if err != nil {
		log.Println("Ошибка обновления БД при платеже:", err)
		return nil
	}

	// 2. ОТПРАВЛЯЕМ ЭМОЦИОНАЛЬНЫЙ ПУШ ОТПРАВИТЕЛЮ
	msg := fmt.Sprintf("Ахах, твой друг @%s не выдержал и взломал твою капсулу за деньги! 🌟", hackerUsername)
	_, err = b.SendMessage(c.SenderID, msg, nil)
	if err != nil {
		log.Println("Ошибка отправки уведомления:", err)
	}

	return nil
}