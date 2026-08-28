package main

import (
	"net/http"
	"sheduler/internal/db"

	log "github.com/sirupsen/logrus"
)

func main() {
	mux := http.NewServeMux()
	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatal(err.Error())
	}
	h := Handler{q: &db.Queries{}}
	mux.HandleFunc("GET /task:id", h.CreateTask)
	mux.HandleFunc("GET /tasks", h.GetTasks)
	mux.HandleFunc("Delete /task:id", h.DeleteTask)
	mux.HandleFunc("PUT /task:id", h.UpdateTask)

	mux.HandleFunc("GET /reminder:id", h.CreateReminder)
	mux.HandleFunc("GET /reminders", h.GetReminders)
	mux.HandleFunc("PUT /reminder:id", h.UpdateReminder)
	mux.HandleFunc("Delete /reminder:id", h.DeleteReminder)

	mux.HandleFunc("GET /audit-trial", h.GetAuditTrial)
	mux.HandleFunc("GET /audit-logs", h.GetAllAuditLogs)
}

type Handler struct {
	q *db.Queries
}

func (h Handler) CreateTask(w http.ResponseWriter, r *http.Request)
func (h Handler) GetTasks(w http.ResponseWriter, r *http.Request)
func (h Handler) DeleteTask(w http.ResponseWriter, r *http.Request)
func (h Handler) UpdateTask(w http.ResponseWriter, r *http.Request)

func (h Handler) CreateReminder(w http.ResponseWriter, r *http.Request)
func (h Handler) GetReminders(w http.ResponseWriter, r *http.Request)
func (h Handler) UpdateReminder(w http.ResponseWriter, r *http.Request)
func (h Handler) DeleteReminder(w http.ResponseWriter, r *http.Request)

func (h Handler) GetAuditTrial(w http.ResponseWriter, r *http.Request)
func (h Handler) GetAllAuditLogs(w http.ResponseWriter, r *http.Request)
