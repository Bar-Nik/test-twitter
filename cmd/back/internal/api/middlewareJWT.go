package api

import (
	"context"
	"fmt"
	"strings"

	"github.com/golang-jwt/jwt/v4"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type Claims struct {
	UserID string `json:"user_id"`
	// Email  string `json:"email"`
	jwt.RegisteredClaims
}

type contextKey string

const (
	UserIDKey contextKey = "user_id"
)

// JWTConfig конфигурация для JWT
type JWTConfig struct {
	SecretKey string
}

// AuthInterceptor для gRPC
func AuthInterceptor(jwtSecret string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// Пропускаем некоторые методы (например, health check)
		// if isPublicMethod(info.FullMethod) {
		// 	return handler(ctx, req)
		// }

		// Извлекаем токен из метаданных
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "metadata is not provided")
		}

		authHeaders := md["authorization"]
		if len(authHeaders) == 0 {
			return nil, status.Error(codes.Unauthenticated, "authorization token is not provided")
		}

		token := strings.TrimPrefix(authHeaders[0], "Bearer ")

		// Валидируем токен
		claims, err := ValidateToken(token, jwtSecret)
		if err != nil {
			return nil, status.Error(codes.Unauthenticated, fmt.Sprintf("invalid token: %v", err))
		}

		if claims.UserID == "" {
			// return nil, status.Error(codes.Unauthenticated, "user_id empty, invalid token")
			return nil, fmt.Errorf("user_id empty, invalid token")
		}
		ctx = context.WithValue(ctx, UserIDKey, claims.UserID)

		return handler(ctx, req)
	}
}

// ValidateToken проверяет и расшифровывает JWT токен
func ValidateToken(tokenString, secretKey string) (*Claims, error) {
	claims := &Claims{}

	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(secretKey), nil
	})

	if err != nil {
		return nil, err
	}

	if !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	return claims, nil
}

// func isPublicMethod(method string) bool {
// 	publicMethods := []string{
// 		"/grpc.health.v1.Health/Check",
// 		// добавьте другие публичные методы при необходимости
// 	}

// 	for _, m := range publicMethods {
// 		if method == m {
// 			return true
// 		}
// 	}
// 	return false
// }

// GetUserIDFromContext извлекает user_id из контекста
func GetUserIDFromContext(ctx context.Context) (string, error) {
	userID, ok := ctx.Value(UserIDKey).(string)
	if !ok {
		return "", fmt.Errorf("user_id not found in context")
	}
	return userID, nil
}
