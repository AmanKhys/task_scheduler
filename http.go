package main

import (
	"net/http"
	"time"
)

func newRouter(h Handler) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "web/index.html")
	})
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("web"))))

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	mux.HandleFunc("POST /tasks", h.CreateTask)
	mux.HandleFunc("GET /tasks", h.GetTasks)
	mux.HandleFunc("GET /tasks/{id}", h.GetTask)
	mux.HandleFunc("PUT /tasks/{id}", h.UpdateTask)
	mux.HandleFunc("DELETE /tasks/{id}", h.DeleteTask)

	mux.HandleFunc("POST /tasks/{id}/reminder-rules", h.CreateReminder)
	mux.HandleFunc("GET /reminder-rules", h.GetReminders)
	mux.HandleFunc("GET /reminder-rules/{id}", h.GetReminder)
	mux.HandleFunc("PUT /reminder-rules/{id}", h.UpdateReminder)
	mux.HandleFunc("PATCH /reminder-rules/{id}/status", h.SetReminderStatus)
	mux.HandleFunc("DELETE /reminder-rules/{id}", h.DeleteReminder)

	mux.HandleFunc("GET /audit-logs", h.GetAllAuditLogs)
	mux.HandleFunc("GET /audit-logs/{id}", h.GetAuditLog)
	mux.HandleFunc("GET /audit-trail", h.GetAllAuditLogs)

	return mux
}

func listenAndServe(addr string, handler http.Handler) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	return srv.ListenAndServe()
}
