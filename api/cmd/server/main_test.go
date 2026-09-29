package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestServeWaitsForActiveRequestDuringShutdown(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	requestStarted := make(chan struct{})
	releaseRequest := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseRequest) }) }
	defer release()
	shutdownStarted := make(chan struct{})
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(requestStarted)
		<-releaseRequest
		w.WriteHeader(http.StatusOK)
	})}
	server.RegisterOnShutdown(func() { close(shutdownStarted) })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	serveDone := make(chan error, 1)
	go func() { serveDone <- serve(ctx, server, listener) }()

	requestDone := make(chan error, 1)
	go func() {
		client := &http.Client{Timeout: 3 * time.Second}
		response, err := client.Get("http://" + listener.Addr().String())
		if err == nil {
			response.Body.Close()
		}
		requestDone <- err
	}()
	select {
	case <-requestStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("request did not reach the server")
	}

	cancel()
	select {
	case <-shutdownStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("server did not start graceful shutdown")
	}
	select {
	case err := <-serveDone:
		t.Fatalf("serve returned before active request completed: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	release()
	select {
	case err := <-requestDone:
		if err != nil {
			t.Errorf("active request failed: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("active request did not complete")
	}
	select {
	case err := <-serveDone:
		if err != nil {
			t.Errorf("serve returned error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("serve did not return after request completed")
	}
}

func TestHandlerKeepsLivenessIndependentFromDatabaseOutage(t *testing.T) {
	pool, err := pgxpool.New(context.Background(), "postgresql://private-user:private-password@127.0.0.1:1/private-db?connect_timeout=1")
	if err != nil {
		t.Fatal(err)
	}
	pool.Close()
	handler := newHandler(pool, "http://localhost:3000")

	live := httptest.NewRecorder()
	handler.ServeHTTP(live, httptest.NewRequest(http.MethodGet, "/api/v1/health/live", nil))
	if live.Code != http.StatusOK {
		t.Fatalf("liveness = %d: %s", live.Code, live.Body.String())
	}

	ready := httptest.NewRecorder()
	handler.ServeHTTP(ready, httptest.NewRequest(http.MethodGet, "/api/v1/health/ready", nil))
	if ready.Code != http.StatusServiceUnavailable {
		t.Fatalf("readiness = %d: %s", ready.Code, ready.Body.String())
	}
	var response struct {
		Code      string `json:"code"`
		RequestID string `json:"requestId"`
	}
	if err := json.Unmarshal(ready.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Code != "NOT_READY" || response.RequestID == "" {
		t.Fatalf("readiness response = %#v", response)
	}
	for _, secret := range []string{"private-user", "private-password", "127.0.0.1", "private-db"} {
		if strings.Contains(ready.Body.String(), secret) {
			t.Fatalf("readiness leaked %q: %s", secret, ready.Body.String())
		}
	}

	options := httptest.NewRecorder()
	handler.ServeHTTP(options, httptest.NewRequest(http.MethodGet, "/api/v1/booking-options", nil))
	if options.Code != http.StatusInternalServerError || strings.Contains(options.Body.String(), "closed pool") {
		t.Fatalf("booking options outage = %d: %s", options.Code, options.Body.String())
	}
}
