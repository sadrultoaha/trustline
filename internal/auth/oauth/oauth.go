// Package oauth abstracts the Google sign-in provider so the full redirect
// flow can run against a fake in development, tests and E2E.
package oauth

import (
	"context"
	"net/http"
)

// Identity is what a provider knows about the signed-in person.
type Identity struct {
	Subject   string // stable provider user id
	Email     string
	Name      string
	AvatarURL string
}

// Provider is implemented by the real Google client and by the fake.
type Provider interface {
	// AuthCodeURL returns the URL to send the browser to.
	AuthCodeURL(state string) string
	// Exchange turns the callback code into an identity.
	Exchange(ctx context.Context, code string) (*Identity, error)
	// Mount lets a provider register extra routes (the fake serves its consent page).
	Mount(mux Router)
}

// Router is the subset of chi.Router providers need.
type Router interface {
	Get(pattern string, h http.HandlerFunc)
	Post(pattern string, h http.HandlerFunc)
}
