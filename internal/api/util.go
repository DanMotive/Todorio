package api

import "github.com/DanMotive/Todorio/internal/auth"

// authHash — thin wrapper so we don't have to import auth in every file just for one function.
func authHash(password string) (string, error) {
	return auth.HashPassword(password)
}
