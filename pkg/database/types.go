package database

import (
	"fmt"
	"time"
)

// DBConfig configuração do Supabase/PostgreSQL
// Suporta tanto conexão URI quanto parâmetros individuais
type DBConfig struct {
	// Para conexão via Supabase REST API
	SupabaseURL string
	SupabaseKey string
	
	// Para conexão direta PostgreSQL
	Host     string
	Port     int
	User     string
	Password string
	Database string
	DBName   string // alias para Database
	SSLMode  string
}

// GetConnectionString retorna a string de conexão PostgreSQL
func (c DBConfig) GetConnectionString() string {
	// Se tiver SupabaseURL, usa ele
	if c.SupabaseURL != "" {
		return c.SupabaseURL
	}
	// Senão, monta a string
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
	StartedAt    time.Time `json:"started_at"`
	LastActivity time.Time `json:"last_activity"`
}
