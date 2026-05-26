package googleauth

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

type IDTokenAuthorizationSession struct {
	server *http.Server
	result chan IDTokenAuthorizationResult
}

type IDTokenAuthorizationResult struct {
	IDToken string
	Err     error
}

const (
	defaultOAuthClientID     = "863485242223-ghqf2jt00v710rkt27oieivkg7h613nr.apps.googleusercontent.com"
	defaultOAuthClientSecret = "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"
	oauthExchangeTimeout     = 30 * time.Second
	serverShutdownTimeout    = 5 * time.Second
)

func OAuthClientID() string {
	if override := strings.TrimSpace(os.Getenv("KOKO_TOOLS_GOOGLE_CLIENT_ID")); override != "" {
		return override
	}
	return strings.TrimSpace(defaultOAuthClientID)
}

func OAuthClientSecret() string {
	if override := strings.TrimSpace(os.Getenv("KOKO_TOOLS_GOOGLE_CLIENT_SECRET")); override != "" {
		return override
	}
	return strings.TrimSpace(defaultOAuthClientSecret)
}

func HasCredentials() bool {
	return OAuthClientID() != "" && OAuthClientSecret() != ""
}

func StartLocalIDTokenAuthorization() (string, *IDTokenAuthorizationSession, error) {
	config, err := oauthConfigForScopes([]string{"openid", "email", "profile"})
	if err != nil {
		return "", nil, err
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, fmt.Errorf("start local callback listener: %w", err)
	}
	state := fmt.Sprintf("koko-tools-firebase-%d", time.Now().UnixNano())
	config.RedirectURL = fmt.Sprintf("http://%s/oauth2/callback", listener.Addr().String())

	session := &IDTokenAuthorizationSession{
		result: make(chan IDTokenAuthorizationResult, 1),
	}

	mux := http.NewServeMux()
	server := &http.Server{Handler: mux}
	session.server = server

	sendResult := func(result IDTokenAuthorizationResult) {
		select {
		case session.result <- result:
		default:
		}
	}
	shutdown := func() {
		go func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), serverShutdownTimeout)
			defer cancel()
			_ = server.Shutdown(shutdownCtx)
		}()
	}

	mux.HandleFunc("/oauth2/callback", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != state {
			http.Error(w, "Invalid OAuth state.", http.StatusBadRequest)
			sendResult(IDTokenAuthorizationResult{Err: errors.New("invalid OAuth state")})
			shutdown()
			return
		}

		authCode := r.URL.Query().Get("code")
		if authCode == "" {
			http.Error(w, "Missing authorization code.", http.StatusBadRequest)
			sendResult(IDTokenAuthorizationResult{Err: errors.New("missing authorization code in callback")})
			shutdown()
			return
		}

		exchangeCtx, cancel := context.WithTimeout(context.Background(), oauthExchangeTimeout)
		defer cancel()
		token, err := config.Exchange(exchangeCtx, authCode)
		if err != nil {
			http.Error(w, "Authorization failed.", http.StatusBadRequest)
			sendResult(IDTokenAuthorizationResult{Err: fmt.Errorf("exchange authorization code: %w", err)})
			shutdown()
			return
		}
		idToken, _ := token.Extra("id_token").(string)
		if strings.TrimSpace(idToken) == "" {
			http.Error(w, "Google did not return an ID token.", http.StatusBadRequest)
			sendResult(IDTokenAuthorizationResult{Err: errors.New("google authorization did not return an id token")})
			shutdown()
			return
		}

		_, _ = fmt.Fprintf(w, "<html><body><h1>Google sign-in complete</h1><p>You can return to %s.</p></body></html>", html.EscapeString("Koko Tools"))
		sendResult(IDTokenAuthorizationResult{IDToken: idToken})
		shutdown()
	})

	go func() {
		err := server.Serve(listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			sendResult(IDTokenAuthorizationResult{Err: fmt.Errorf("authorization callback server: %w", err)})
		}
	}()

	return config.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce), session, nil
}

func (s *IDTokenAuthorizationSession) Wait() <-chan IDTokenAuthorizationResult {
	return s.result
}

func (s *IDTokenAuthorizationSession) Close() error {
	if s == nil || s.server == nil {
		return nil
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), serverShutdownTimeout)
	defer cancel()
	return s.server.Shutdown(shutdownCtx)
}

func oauthConfigForScopes(scopes []string) (*oauth2.Config, error) {
	clientID := OAuthClientID()
	clientSecret := OAuthClientSecret()
	if clientID == "" || clientSecret == "" {
		return nil, errors.New("missing Google OAuth client configuration")
	}

	return &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     google.Endpoint,
		Scopes:       scopes,
	}, nil
}
