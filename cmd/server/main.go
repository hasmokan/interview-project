package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/crypto/bcrypt"
	"interview-project/internal/config"
	"interview-project/internal/httpapi"
	"interview-project/internal/store"
)

func main() {
	if len(os.Args) > 1 {
		if e := httpapi.HashInput(os.Args[1], os.Stdin, os.Stdout); e != nil {
			log.Fatal(e)
		}
		return
	}
	cfg, e := config.Load()
	if e != nil {
		log.Fatal(e)
	}
	if _, e = bcrypt.Cost([]byte(cfg.PasswordHash)); e != nil {
		log.Fatal("invalid password hash")
	}
	s, e := store.Open(cfg.DataDir, cfg.PublicURL, cfg.MaxUpload)
	if e != nil {
		log.Fatal(e)
	}
	defer s.Close()
	srv := &http.Server{Addr: cfg.Addr, Handler: httpapi.New(cfg, s), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		srv.Shutdown(shutdown)
	}()
	log.Printf("fileshare listening on %s", cfg.Addr)
	if e = srv.ListenAndServe(); e != nil && e != http.ErrServerClosed {
		log.Fatal(e)
	}
}
