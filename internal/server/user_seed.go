package server

import (
	"context"

	"github.com/yourname/nscan/internal/server/repositories"
	"github.com/yourname/nscan/pkg/models"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

func SeedAdminUser(ctx context.Context, repo *repositories.UserRepo, username, password string, log *zap.Logger) {
	if username == "" {
		return
	}
	// 检查是否已经有该用户
	user, err := repo.FindByUsername(ctx, username)
	if err == nil && user != nil {
		return // 用户已存在
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Error("Failed to hash admin password", zap.Error(err))
		return
	}

	newUser := &models.User{
		Username:     username,
		PasswordHash: string(hash),
		Role:         "admin",
	}
	if err := repo.Create(ctx, newUser); err != nil {
		log.Error("Failed to seed admin user", zap.Error(err))
	} else {
		log.Info("Seeded default admin user", zap.String("username", username))
	}
}
