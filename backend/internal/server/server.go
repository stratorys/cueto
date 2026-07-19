// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

// Package server owns the HTTP serving loop shared by the dev server command and
// cueto serve: explicit connection timeouts, serve-until-signal, and a graceful
// drain so running evaluations finish (or hit their own deadline) instead of
// being cut off.
package server

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"
)

// Run serves handler on :port until SIGINT or SIGTERM, then drains in-flight
// requests. evalTimeout sizes the write timeout: it must exceed the evaluation
// deadline or long evaluations get cut off mid-response.
func Run(handler http.Handler, port string, evalTimeout time.Duration) error {
	// Explicit server timeouts bound the connection layer that the body cap and
	// eval deadline do not: slow-client (slowloris) reads and stuck writes.
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      evalTimeout + 10*time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}
	stop()
	log.Println("Shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
