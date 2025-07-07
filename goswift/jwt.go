// quikdocs/backend/goswift/jwt.go
package goswift

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Define a secret key for signing JWTs.
// In a real application, this should be a strong, randomly generated key
// loaded from environment variables or a secure configuration system.
var jwtSecret = []byte("supersecretjwtkeythatshouldbemoresecureinproduction")

// Claims defines the JWT claims structure.
type Claims struct {
	UserID string `json:"user_id"`
	jwt.RegisteredClaims
}

// GenerateJWT generates a new JWT for the given user ID.
func GenerateJWT(userID string) (string, error) {
	expirationTime := time.Now().Add(24 * time.Hour) // Token valid for 24 hours
	claims := &Claims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    "quikdocs-goswift",
			Subject:   userID,
			ID:        fmt.Sprintf("%d", time.Now().UnixNano()), // Unique ID for the token
			Audience:  []string{"quikdocs-users"},
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(jwtSecret)
	if err != nil {
		return "", fmt.Errorf("failed to sign JWT: %w", err)
	}
	return tokenString, nil
}

// ValidateJWT validates a JWT string and returns the claims if valid.
func ValidateJWT(tokenString string) (*Claims, error) {
	claims := &Claims{}

	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		// Verify the signing method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return jwtSecret, nil
	})

	if err != nil {
		return nil, fmt.Errorf("invalid JWT: %w", err)
	}

	if !token.Valid {
		return nil, fmt.Errorf("JWT is invalid")
	}

	return claims, nil
}
