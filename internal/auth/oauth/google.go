package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// Google is the real provider using Google's OAuth 2.0 + userinfo endpoint.
type Google struct {
	cfg *oauth2.Config
}

func NewGoogle(clientID, clientSecret, redirectURL string) *Google {
	return &Google{cfg: &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
		Scopes:       []string{"openid", "email", "profile"},
		Endpoint:     google.Endpoint,
	}}
}

func (g *Google) AuthCodeURL(state string) string {
	return g.cfg.AuthCodeURL(state, oauth2.AccessTypeOnline)
}

func (g *Google) Exchange(ctx context.Context, code string) (*Identity, error) {
	tok, err := g.cfg.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("google exchange: %w", err)
	}
	client := g.cfg.Client(ctx, tok)
	resp, err := client.Get("https://openidconnect.googleapis.com/v1/userinfo")
	if err != nil {
		return nil, fmt.Errorf("google userinfo: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("google userinfo: status %d", resp.StatusCode)
	}
	var ui struct {
		Sub           string `json:"sub"`
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Name          string `json:"name"`
		Picture       string `json:"picture"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&ui); err != nil {
		return nil, err
	}
	if !ui.EmailVerified {
		return nil, fmt.Errorf("google account email is not verified")
	}
	return &Identity{Subject: ui.Sub, Email: ui.Email, Name: ui.Name, AvatarURL: ui.Picture}, nil
}

func (g *Google) Mount(Router) {}
