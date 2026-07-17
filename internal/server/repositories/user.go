package repositories

import (
	"context"
	"time"

	"github.com/yourname/nscan/pkg/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

const collUser = "users"

type UserRepo struct {
	db *mongo.Database
}

func NewUserRepo(db *mongo.Database) *UserRepo {
	return &UserRepo{db: db}
}

func (r *UserRepo) FindByUsername(ctx context.Context, username string) (*models.User, error) {
	var user models.User
	err := r.db.Collection(collUser).FindOne(ctx, bson.M{"username": username}).Decode(&user) // @check-ignore: User auth: lookup by username, not user-scoped
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *UserRepo) Create(ctx context.Context, user *models.User) error {
	user.CreatedAt = time.Now()
	user.UpdatedAt = time.Now()
	res, err := r.db.Collection(collUser).InsertOne(ctx, user)
	if err == nil && res.InsertedID != nil {
		if oid, ok := res.InsertedID.(primitive.ObjectID); ok {
			user.ID = oid
		}
	}
	return err
}

func (r *UserRepo) UpdatePassword(ctx context.Context, id primitive.ObjectID, passwordHash string) error {
	_, err := r.db.Collection(collUser).UpdateOne(ctx, bson.M{"_id": id}, bson.M{
		"$set": bson.M{"password_hash": passwordHash, "updated_at": time.Now()},
	})
	return err
}
