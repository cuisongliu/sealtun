package publicauth

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode"

	"golang.org/x/crypto/bcrypt"
)

type BasicAuth struct {
	Username     string
	PasswordHash string
}

func NewBasicAuth(username, password string) (*BasicAuth, error) {
	username = strings.TrimSpace(username)
	if err := ValidateUsername(username); err != nil {
		return nil, err
	}
	if password == "" {
		return nil, fmt.Errorf("basic auth password is required")
	}
	hash, err := HashPassword(password)
	if err != nil {
		return nil, err
	}
	return &BasicAuth{
		Username:     username,
		PasswordHash: hash,
	}, nil
}

func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash basic auth password: %w", err)
	}
	return string(hash), nil
}

func LegacySHA256Hash(password string) string {
	sum := sha256.Sum256([]byte(password))
	return hex.EncodeToString(sum[:])
}

func ValidateUsername(username string) error {
	if username == "" {
		return fmt.Errorf("basic auth username is required")
	}
	if strings.Contains(username, ":") {
		return fmt.Errorf("basic auth username must not contain ':'")
	}
	for _, r := range username {
		if unicode.IsControl(r) {
			return fmt.Errorf("basic auth username must not contain control characters")
		}
	}
	return nil
}

func ValidatePasswordHash(hash string) error {
	if isLegacySHA256Hash(hash) {
		return nil
	}
	if _, err := bcrypt.Cost([]byte(hash)); err != nil {
		return fmt.Errorf("basic auth password hash must be bcrypt")
	}
	return nil
}

func Validate(config BasicAuth) error {
	if err := ValidateUsername(strings.TrimSpace(config.Username)); err != nil {
		return err
	}
	return ValidatePasswordHash(config.PasswordHash)
}

func Check(config BasicAuth, username, password string) bool {
	if Validate(config) != nil {
		return false
	}
	usernameMatch := subtle.ConstantTimeCompare([]byte(username), []byte(strings.TrimSpace(config.Username))) == 1
	passwordMatch := false
	if isLegacySHA256Hash(config.PasswordHash) {
		passwordHash := LegacySHA256Hash(password)
		passwordMatch = subtle.ConstantTimeCompare([]byte(passwordHash), []byte(config.PasswordHash)) == 1
	} else {
		passwordMatch = bcrypt.CompareHashAndPassword([]byte(config.PasswordHash), []byte(password)) == nil
	}
	return usernameMatch && passwordMatch
}

func isLegacySHA256Hash(hash string) bool {
	if len(hash) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(hash)
	return err == nil
}
