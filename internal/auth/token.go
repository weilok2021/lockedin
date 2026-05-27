package auth

import "crypto/rand"

func GenerateToken() string {
	return rand.Text()
}
