package models

import "time"

type Form struct {
	ID              int       `json:"id" db:"id"`
	UUID            string    `json:"uuid" db:"uuid"`
	Name            string    `json:"name" db:"name"`
	QueryPassword   string    `json:"-" db:"query_password"`
	NotifyEmail     string    `json:"notify_email" db:"notify_email"`
	SubmissionCount int       `json:"submission_count"`
	CreatedAt       time.Time `json:"created_at" db:"created_at"`
}

type PaginatedResponse struct {
	Items interface{} `json:"items"`
	Total int         `json:"total"`
	Page  int         `json:"page"`
	Limit int         `json:"limit"`
}

type Submission struct {
	ID           int       `json:"id" db:"id"`
	FormID       int       `json:"form_id" db:"form_id"`
	Name         string    `json:"name" db:"name"`
	Email        string    `json:"email" db:"email"`
	Phone        string    `json:"phone" db:"phone"`
	Content      string    `json:"content" db:"content"`
	SourceURL    string    `json:"source_url" db:"source_url"`
	ResendStatus string    `json:"resend_status" db:"resend_status"`
	ClientIP     string    `json:"client_ip" db:"client_ip"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
}
