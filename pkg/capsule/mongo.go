package capsule

import (
	"context"
	"fmt"
	"math"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// MongoStore — реализация Store на MongoDB
type MongoStore struct {
	col *mongo.Collection
}

// NewMongoStore подключается к MongoDB и возвращает Store
func NewMongoStore(uri, dbName, collName string) (*MongoStore, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return nil, err
	}

	if err := client.Ping(ctx, nil); err != nil {
		return nil, err
	}

	col := client.Database(dbName).Collection(collName)
	return &MongoStore{col: col}, nil
}

// CreateIndexes создаёт индексы для быстрого поиска
func (s *MongoStore) CreateIndexes() error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	indexes := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "sender_id", Value: 1}},
			Options: options.Index().SetName("idx_sender_id"),
		},
		{
			Keys: bson.D{
				{Key: "reminder_sent", Value: 1},
				{Key: "unlock_at", Value: 1},
			},
			Options: options.Index().SetName("idx_reminder"),
		},
	}

	_, err := s.col.Indexes().CreateMany(ctx, indexes)
	return err
}

func (s *MongoStore) Insert(c *Capsule) error {
	_, err := s.col.InsertOne(context.TODO(), c)
	return err
}

func (s *MongoStore) GetByID(id string) (*Capsule, error) {
	var c Capsule
	err := s.col.FindOne(context.TODO(), bson.M{"_id": id}).Decode(&c)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *MongoStore) SetHacked(id string) (*Capsule, error) {
	var c Capsule
	err := s.col.FindOneAndUpdate(
		context.TODO(),
		bson.M{"_id": id},
		bson.M{"$set": bson.M{"is_hacked": true}},
	).Decode(&c)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// ToggleReaction — добавляет/убирает реакцию пользователя (1 чел = 1 реакция)
func (s *MongoStore) ToggleReaction(id string, userID int64, emoji string) (*Capsule, error) {
	// Читаем текущее состояние
	c, err := s.GetByID(id)
	if err != nil {
		return nil, err
	}

	if c.ReactionsUsers == nil {
		c.ReactionsUsers = make(map[int64]string)
	}
	if c.Reactions == nil {
		c.Reactions = make(map[string]int)
	}

	oldEmoji, hasReaction := c.ReactionsUsers[userID]

	update := bson.M{}
	if hasReaction && oldEmoji == emoji {
		// Убираем реакцию (повторный клик по той же)
		update["$unset"] = bson.M{"reactions_users." + fmtKey(userID): ""}
		if c.Reactions[emoji] > 0 {
			update["$inc"] = bson.M{"reactions." + emoji: -1}
		}
	} else if hasReaction && oldEmoji != emoji {
		// Меняем реакцию
		update["$set"] = bson.M{"reactions_users." + fmtKey(userID): emoji}
		inc := bson.M{"reactions." + emoji: 1}
		if c.Reactions[oldEmoji] > 0 {
			inc["reactions."+oldEmoji] = -1
		}
		update["$inc"] = inc
	} else {
		// Новая реакция
		update["$set"] = bson.M{"reactions_users." + fmtKey(userID): emoji}
		update["$inc"] = bson.M{"reactions." + emoji: 1}
	}

	var result Capsule
	err = s.col.FindOneAndUpdate(
		context.TODO(),
		bson.M{"_id": id},
		update,
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func fmtKey(k int64) string { return fmt.Sprintf("%d", k) }

// SetViewer сохраняет ID первого открывшего капсулу (получателя)
func (s *MongoStore) SetViewer(id string, viewerID int64) error {
	_, err := s.col.UpdateOne(
		context.TODO(),
		bson.M{"_id": id, "viewer_id": int64(0)},
		bson.M{"$set": bson.M{"viewer_id": viewerID}},
	)
	return err
}

// AttemptPasscode — атомарная проверка пасскода (без race condition)
// Возвращает: success=true если код верный (капсула взломана), remaining попыток после операции
func (s *MongoStore) AttemptPasscode(id string, passcode string) (bool, int, error) {
	ctx := context.TODO()
	// Пробуем атомарно: если passcode совпал И попытки ещё есть → взламываем
	var c Capsule
	err := s.col.FindOneAndUpdate(ctx,
		bson.M{"_id": id, "passcode": passcode, "passcode_attempts": bson.M{"$gt": 0}},
		bson.M{"$set": bson.M{"is_hacked": true}},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&c)
	if err == nil {
		// Успех — код верный
		return true, c.PasscodeAttempts, nil
	}
	if err != mongo.ErrNoDocuments {
		return false, 0, err
	}
	// Код неверный — декрементируем попытки (если ещё остались)
	err = s.col.FindOneAndUpdate(ctx,
		bson.M{"_id": id, "passcode_attempts": bson.M{"$gt": 0}},
		bson.M{"$inc": bson.M{"passcode_attempts": -1}},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&c)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return false, 0, nil // попытки кончились
		}
		return false, 0, err
	}
	return false, c.PasscodeAttempts, nil
}

// FindPendingReminders находит капсулы, для которых пора отправить напоминание
// (~за час до открытия, окно: 50-70 минут до unlock)
func (s *MongoStore) FindPendingReminders() ([]*Capsule, error) {
	now := time.Now()
	from := now.Add(50 * time.Minute)
	to := now.Add(70 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cursor, err := s.col.Find(ctx, bson.M{
		"reminder_sent": false,
		"is_hacked":     false,
		"unlock_at": bson.M{
			"$gte": from,
			"$lte": to,
		},
	})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var capsules []*Capsule
	if err := cursor.All(ctx, &capsules); err != nil {
		return nil, err
	}
	return capsules, nil
}

// MarkReminderSent помечает, что напоминание отправлено
func (s *MongoStore) MarkReminderSent(id string) error {
	_, err := s.col.UpdateOne(
		context.TODO(),
		bson.M{"_id": id},
		bson.M{"$set": bson.M{"reminder_sent": true}},
	)
	return err
}

// Ensure interface satisfaction
var _ Store = (*MongoStore)(nil)

// Contribute — добавляет вклад пользователя в групповой сбор
func (s *MongoStore) Contribute(id string, userID int64, amount int) (*Capsule, error) {
	ctx := context.TODO()
	// Атомарно добавляем вклад
	key := fmt.Sprintf("stars_contributions.%d", userID)
	var c Capsule
	err := s.col.FindOneAndUpdate(ctx,
		bson.M{"_id": id, "capsule_type": "group", "is_hacked": false},
		bson.M{"$inc": bson.M{key: amount}},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&c)
	if err != nil {
		return nil, err
	}
	// Проверяем, достигнута ли цель
	total := 0
	for _, v := range c.StarsContributions {
		total += v
	}
	if total >= c.GoalStars {
		return s.SetHacked(id)
	}
	return &c, nil
}

// GeoCheck — проверяет расстояние до цели и автоматически разблокирует
func (s *MongoStore) GeoCheck(id string, lat, lng float64) (bool, float64, error) {
	c, err := s.GetByID(id)
	if err != nil {
		return false, 0, err
	}
	// Haversine formula
	dist := haversine(lat, lng, c.GeoLat, c.GeoLng)
	if dist <= float64(c.GeoRadius) {
		_, err := s.SetHacked(id)
		return true, dist, err
	}
	return false, dist, nil
}

func haversine(lat1, lng1, lat2, lng2 float64) float64 {
	const R = 6371000.0
	dLat := (lat2 - lat1) * math.Pi / 180
	dLng := (lng2 - lng1) * math.Pi / 180
	a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(lat1*math.Pi/180)*math.Cos(lat2*math.Pi/180)*math.Sin(dLng/2)*math.Sin(dLng/2)
	return R * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}
