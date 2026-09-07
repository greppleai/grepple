package router

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// refreshedToken is the subset of GitHub's token response relayed back to the
// CLI after a refresh. It carries the new access token and both lifetimes so the
// client can schedule the next refresh.
type refreshedToken struct {
	AccessToken           string `json:"access_token"`
	ExpiresIn             int    `json:"expires_in"`
	RefreshToken          string `json:"refresh_token"`
	RefreshTokenExpiresIn int    `json:"refresh_token_expires_in"`
	TokenType             string `json:"token_type,omitempty"`
	Scope                 string `json:"scope,omitempty"`
}

// refreshUserToken exchanges a GitHub App user refresh token for a fresh user
// access token, supplying the server-held client secret. It returns the token
// set to relay, an HTTP status to respond with, and an error to surface. Tokens
// (in or out) are never logged. A GitHub-reported error (e.g. an expired or
// revoked refresh token) maps to 401 so the CLI knows to prompt a fresh login.
func refreshUserToken(client *http.Client, webBase, clientID, clientSecret, refreshToken string) (refreshedToken, int, error) {
	form := url.Values{
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	}
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(webBase, "/")+"/login/oauth/access_token", strings.NewReader(form.Encode()))
	if err != nil {
		return refreshedToken{}, http.StatusInternalServerError, err
	}
	req.Header.Set("accept", "application/json")
	req.Header.Set("content-type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		return refreshedToken{}, http.StatusBadGateway, err
	}
	defer resp.Body.Close()
	var body struct {
		refreshedToken
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return refreshedToken{}, http.StatusBadGateway, fmt.Errorf("decode refresh response: %w", err)
	}
	if body.Error != "" {
		message := body.ErrorDescription
		if message == "" {
			message = body.Error
		}
		return refreshedToken{}, http.StatusUnauthorized, fmt.Errorf("refresh rejected: %s", message)
	}
	if body.AccessToken == "" {
		return refreshedToken{}, http.StatusBadGateway, fmt.Errorf("refresh response contained no access token")
	}
	return body.refreshedToken, http.StatusOK, nil
}
