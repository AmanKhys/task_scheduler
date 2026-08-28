package main

import (
	"context"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"sheduler/internal/db"

	"github.com/jackc/pgx/v5/pgxpool"
	log "github.com/sirupsen/logrus"
)

func main() {
	log.SetFormatter(&log.TextFormatter{FullTimestamp: true})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://postgres:postgres@localhost:5433/task_scheduler?sslmode=disable"
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("database ping: %v", err)
	}

	schemaSQL, err := os.ReadFile("db/schema.sql")
	if err != nil {
		log.Fatalf("read schema: %v", err)
	}
	if err := applySchema(ctx, pool, string(schemaSQL)); err != nil {
		log.Fatalf("apply schema: %v", err)
	}

	q := db.New(pool)
	if err := seedIfEmpty(ctx, q); err != nil {
		log.Fatalf("seed: %v", err)
	}

	h := Handler{q: q}
	mux := newRouter(h)

	sched := &Scheduler{q: q, interval: 15 * time.Second}
	go sched.Run(ctx)

	addr := ":8080"
	if v := os.Getenv("PORT"); v != "" {
		addr = ":" + v
	}

	srvErr := make(chan error, 1)
	go func() {
		log.Infof("listening on %s", addr)
		srvErr <- listenAndServe(addr, mux)
	}()

	select {
	case <-ctx.Done():
		log.Info("shutting down")
	case err := <-srvErr:
		if err != nil {
			log.Fatal(err)
		}
	}
}

func applySchema(ctx context.Context, pool *pgxpool.Pool, sql string) error {
	for _, stmt := range splitSQL(sql) {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

func splitSQL(sql string) []string {
	parts := strings.Split(sql, ";")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" || strings.HasPrefix(p, "--") && !strings.Contains(p, "\n") {
			continue
		}
		out = append(out, p)
	}
	return out
}
