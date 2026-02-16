package database

import "time"

// DBConfig configuração do banco de dados Supabase/PostgreSQL
type DBConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	DBName   string
	SSLMode  string
}

// Message representa uma mensagem armazenada no banco
type Message struct {
	ID        string    `json:"id"`
	Content   string    `json:"content"`
	SenderID  string    `json:"sender_id"`
	ChatID    string    `json:"chat_id"`
	Channel   string    `json:"channel"` // telegram, discord, etc
	Timestamp time.Time `json:"timestamp"`
	CreatedAt time.Time `json:"created_at"`
}

// Session representa uma sessão de conversa
type Session struct {
	ID        string    `json:"id"`
	ChatID    string    `json:"chat_id"`
	Channel   string    `json:"channel"`
	StartedAt time.Time `json:"started_at"`
	LastActivity time.Time `json:"last_activity"`
}

// User representa um usuário do sistema
type User struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}
