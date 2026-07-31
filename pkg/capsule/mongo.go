package capsule

import (
	"context"
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

// Ensure interface satisfaction
var _ Store = (*MongoStore)(nil)
