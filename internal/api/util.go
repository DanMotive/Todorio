package api

import "github.com/DanMotive/Todorio/internal/auth"

// authHash — тонкая обёртка, чтобы не импортировать auth в каждом файле ради одной функции.
func authHash(password string) (string, error) {
	return auth.HashPassword(password)
}
