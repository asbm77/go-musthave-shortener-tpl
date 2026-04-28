package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	secretKey = "your-secret-key-change-in-production" // В реальном приложении - из переменных окружения
)

type Claims struct {
	UserID string `json:"user_id"`
	Exp    int64  `json:"exp"`
}

// GenerateUserID генерирует новый ID пользователя
func GenerateUserID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// GenerateToken создает JWT-подобный токен
func GenerateToken(userID string) (string, error) {
	claims := Claims{
		UserID: userID,
		Exp:    time.Now().Add(30 * 24 * time.Hour).Unix(), // 30 дней
	}

	// Кодируем claims в JSON
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}

	// Кодируем в Base64URL
	encodedClaims := base64.RawURLEncoding.EncodeToString(claimsJSON)

	// Создаем подпись
	signature := createSignature(encodedClaims)

	// Формируем токен
	token := encodedClaims + "." + signature

	return token, nil
}

// ValidateToken проверяет и декодирует токен
func ValidateToken(token string) (string, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid token format")
	}

	encodedClaims := parts[0]
	signature := parts[1]

	// Проверяем подпись
	expectedSignature := createSignature(encodedClaims)
	if !hmac.Equal([]byte(signature), []byte(expectedSignature)) {
		return "", fmt.Errorf("invalid signature")
	}

	// Декодируем claims
	claimsJSON, err := base64.RawURLEncoding.DecodeString(encodedClaims)
	if err != nil {
		return "", fmt.Errorf("failed to decode claims: %w", err)
	}

	var claims Claims
	if err := json.Unmarshal(claimsJSON, &claims); err != nil {
		return "", fmt.Errorf("failed to parse claims: %w", err)
	}

	// Проверяем expiration
	if claims.Exp < time.Now().Unix() {
		return "", fmt.Errorf("token expired")
	}

	return claims.UserID, nil
}

func createSignature(data string) string {
	h := hmac.New(sha256.New, []byte(secretKey))
	h.Write([]byte(data))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}
