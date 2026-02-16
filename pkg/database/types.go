package database

import (
	"context"
	"fmt"
	"time"
)

// DBProvider interface para operações de banco de dados
type DBProvider interface {
	IsConnected() bool
	LoadSession(ctx context.Context, chatID string) ([]Message, error)
	SaveSession(ctx context.Context, chatID string, messages []Message) error
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
	Password     string
	Database     string
	DBName       string
	SSLMode      string
}

// GetConnectionString retorna a string de conexão PostgreSQL
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

// Message representa uma mensagem armazenada no banco
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

// Session representa uma sessão de conversa
type Session struct {
	ID           string    `json:"id"`
	ChatID       string    `json:"chat_id"`
	Channel      string    `json:"channel"`
	Messages     []Message `json:"messages"`
	StartedAt    time.Time `json:"started_at"`
	LastActivity time.Time `json:"last_activity"`
}
