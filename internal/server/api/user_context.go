package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// UserID extracts the authenticated user's ObjectID from the gin context.
// Returns ObjectID{} if not present (should not happen on authenticated routes).
func UserID(c *gin.Context) primitive.ObjectID {
	v, exists := c.Get("user_id")
	if !exists {
		return primitive.ObjectID{}
	}
	switch id := v.(type) {
	case primitive.ObjectID:
		return id
	case string:
		if oid, err := primitive.ObjectIDFromHex(id); err == nil {
			return oid
		}
	}
	return primitive.ObjectID{}
}

// RequireUser returns the userID and true, or writes a 401 and returns false.
func RequireUser(c *gin.Context) (primitive.ObjectID, bool) {
	uid := UserID(c)
	if uid.IsZero() {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		c.Abort()
		return primitive.ObjectID{}, false
	}
	return uid, true
}
