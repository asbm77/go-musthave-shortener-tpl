// middleware/auth.go
package middleware

import (
	"context"
	"net/http"

	"github.com/asbm77/go-musthave-shortener-tpl/internal/auth"
)

const cookieName = "user_token"

type contextKey string

const (
	UserIDKey contextKey = "userID"
)

// AuthMiddleware проверяет или создает аутентификационную куку
func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var userID string

		// Пытаемся получить куку
		cookie, err := r.Cookie(cookieName)
		if err == nil && cookie.Value != "" {
			// Валидируем токен
			userID, err = auth.ValidateToken(cookie.Value)
			if err == nil && userID != "" {
				// Токен валиден, сохраняем userID в контексте
				ctx := context.WithValue(r.Context(), UserIDKey, userID)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}

		// Создаем нового пользователя
		userID = auth.GenerateUserID()
		token, err := auth.GenerateToken(userID)
		if err != nil {
			http.Error(w, "Failed to generate token", http.StatusInternalServerError)
			return
		}

		// Устанавливаем куку
		http.SetCookie(w, &http.Cookie{
			Name:     cookieName,
			Value:    token,
			Path:     "/",
			MaxAge:   30 * 24 * 3600, // 30 дней
			HttpOnly: true,
			Secure:   false, // Для development, в production true
			SameSite: http.SameSiteLaxMode,
		})

		// Сохраняем userID в контексте
		ctx := context.WithValue(r.Context(), UserIDKey, userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetUserID извлекает userID из контекста
func GetUserID(ctx context.Context) string {
	if userID, ok := ctx.Value(UserIDKey).(string); ok {
		return userID
	}
	return ""
}
