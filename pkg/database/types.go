package database

import (
	"context"
	"fmt"
	"time"
)

// DBProvider interface completa - compatível com main.go e loop.go
type DBProvider interface {
	IsConnected() bool
	Connect(ctx context.Context) error      // main.go usa Connect(ctx)
	Disconnect() error                      // main.go usa Disconnect()
	LoadSession(ctx context.Context, chatID string) ([]Message, error)
	SaveSession(ctx context.Context, chatID string, messages []Message) error
	SaveMessage(msg *Message) error
	GetMessages(chatID string, limit int) ([]Message, error)
	Close() error
}

// DBConfig completa - 100% compatível com main.go
type DBConfig struct {
	Driver      string // "supabase", "postgres", "sqlite", "mysql"
	SupabaseURL string
	SupabaseKey string
	Host        string
	Port        string  // ← string (main.go passa "5432")
	Database    string  // DBName alias
	DBName      string
	Username    string  // ← User alias (main.go usa Username)
	User        string  // ← alias para Username
	Password    string
	SQLitePath  string
	SSLMode     string
}

// GetConnectionString retorna string de conexão
func (c DBConfig) GetConnectionString() string {
	// Prioridade: SupabaseURL direto
	if c.SupabaseURL != "" {
		return c.SupabaseURL
	}
	
	// Monta PostgreSQL
	host := c.Host
	if host == "" {
		host = "localhost"
	}
	
	port := c.Port
	if port == "" {
		port = "5432"
	}
	
	user := c.Username
	if user == "" {
		user = c.User
	}
	if user == "" {
		user = "postgres"
	}
	
	dbname := c.Database
	if dbname == "" {
		dbname = c.DBName
	}
	if dbname == "" {
		dbname = "postgres"
	}
	
	ssl := c.SSLMode
	if ssl == "" {
		ssl = "require"
	}
	
	return fmt.Sprintf("postgresql://%s:%s@%s:%s/%s?sslmode=%s",
		user, c.Password, host, port, dbname, ssl)
}

// Message representa uma mensagem
type Message struct {
	ID        string    `json:"id"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	SenderID  string    `json:"sender_id"`
	ChatID    string    `json:"chat_id"`
	Channel   string    `json:"channel"`
	Timestamp time.Time `json:"timestamp"`
	CreatedAt time.Time `json:"created_at"`
}

// Session representa uma sessão
type Session struct {
	ID           string    `json:"id"`
	ChatID       string    `json:"chat_id"`
	Channel      string    `json:"channel"`
	Messages     []Message `json:"messages"`
	StartedAt    time.Time `json:"started_at"`
	LastActivity time.Time `json:"last_activity"`
}
