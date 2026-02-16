package database

import (
	"context"
	"fmt"
	"time"
)

// DBProvider interface - compatível com loop.go
type DBProvider interface {
	IsConnected() bool
	LoadSession(ctx context.Context, chatID string) ([]Message, error)  // retorna []Message direto
	SaveSession(ctx context.Context, chatID string, messages []Message) error  // recebe []Message
	SaveMessage(msg *Message) error
	GetMessages(chatID string, limit int) ([]Message, error)
	Close() error
}

// DBConfig configuração do Supabase/PostgreSQL
type DBConfig struct {
	SupabaseURL string
	SupabaseKey string
	Host        string
	Port        int
	User        string
	Password    string
	Database    string
	DBName      string
	SSLMode     string
}

func (c DBConfig) GetConnectionString() string {
	if c.SupabaseURL != "" {
		return c.SupabaseURL
	}
	dbname := c.Database
	if dbname == "" {
		dbname = c.DBName
	}
	ssl := c.SSLMode
	if ssl == "" {
		ssl = "require"
	}
	return fmt.Sprintf("postgresql://%s:%s@%s:%d/%s?sslmode=%s",
		c.User, c.Password, c.Host, c.Port, dbname, ssl)
}

// Message representa uma mensagem
type Message struct {
	ID        string    `json:"id"`
	Role      string    `json:"role"` // user, assistant, system
	Content   string    `json:"content"`
	SenderID  string    `json:"sender_id"`
	ChatID    string    `json:"chat_id"`
	Channel   string    `json:"channel"`
	Timestamp time.Time `json:"timestamp"`
	CreatedAt time.Time `json:"created_at"`
}

// Session - mantido para compatibilidade futura
type Session struct {
	ID           string    `json:"id"`
	ChatID       string    `json:"chat_id"`
	Channel      string    `json:"channel"`
	Messages     []Message `json:"messages"`
	StartedAt    time.Time `json:"started_at"`
	LastActivity time.Time `json:"last_activity"`
}
