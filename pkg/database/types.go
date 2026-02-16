// Package: database
// File: types.go

package database

import (
	"context"
	"fmt"
	"strings"
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
// CORREÇÃO: Adicionado suporte para connection pooler do Supabase
type DBConfig struct {
	Driver      string // "supabase", "postgres", "sqlite", "mysql"
	SupabaseURL string // Pode ser URL direta (IPv6) ou pooler (IPv4)
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
	// NOVO: Campos específicos para connection pooler
	UsePooler   bool   // Se true, usa connection pooler em vez de conexão direta
	ProjectRef  string // Referência do projeto Supabase (ex: czsqjrgjjgrpwuoimllb)
	PoolerHost  string // Host do pooler (ex: aws-0-us-west-1.pooler.supabase.com)
}

// GetConnectionString retorna string de conexão
// CORREÇÃO: Detecta automaticamente se deve usar pooler (IPv4) ou conexão direta (IPv6)
func (c DBConfig) GetConnectionString() string {
	// Prioridade: SupabaseURL direto (se já estiver configurado corretamente)
	if c.SupabaseURL != "" {
		// Se a URL contém "pooler", já está usando IPv4
		// Se contém "db." + "supabase.co", é conexão direta IPv6 (pode não funcionar no Render)
		if strings.Contains(c.SupabaseURL, "pooler.supabase.com") {
			// Já está usando pooler, retorna como está
			return c.SupabaseURL
		}
		
		// Verifica se é conexão direta Supabase (IPv6)
		if strings.Contains(c.SupabaseURL, "db.") && strings.Contains(c.SupabaseURL, "supabase.co") && !strings.Contains(c.SupabaseURL, "pooler") {
			// Tenta converter para pooler automaticamente
			// postgresql://postgres:senha@db.czsqjrgjjgrpwuoimllb.supabase.co:5432/postgres
			// para:
			// postgresql://postgres.czsqjrgjjgrpwuoimllb:senha@aws-0-us-west-1.pooler.supabase.com:6543/postgres
			
			// Extrai componentes da URL
			var user, password, host, port, dbname string
			fmt.Sscanf(c.SupabaseURL, "postgresql://%s:%s@%s:%s/%s", &user, &password, &host, &port, &dbname)
			
			// Limpa a senha (remove @ se presente)
			password = strings.TrimSuffix(password, "@")
			
			// Extrai project ref do host
			parts := strings.Split(host, ".")
			if len(parts) >= 2 {
				projectRef := parts[1]
				// Assume região us-west-1 (pode ser ajustado)
				poolerHost := "aws-0-us-west-1.pooler.supabase.com"
				poolerUser := fmt.Sprintf("postgres.%s", projectRef)
				
				return fmt.Sprintf("postgresql://%s:%s@%s:6543/%s?sslmode=require",
					poolerUser, password, poolerHost, dbname)
			}
		}
		
		return c.SupabaseURL
	}
	
	// Se tem configuração de pooler específica
	if c.UsePooler && c.PoolerHost != "" {
		user := c.Username
		if user == "" {
			user = c.User
		}
		if user == "" {
			user = "postgres"
		}
		
		// Adiciona project ref ao usuário se necessário
		if c.ProjectRef != "" && !strings.Contains(user, ".") {
			user = fmt.Sprintf("%s.%s", user, c.ProjectRef)
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
		
		return fmt.Sprintf("postgresql://%s:%s@%s:6543/%s?sslmode=%s",
			user, c.Password, c.PoolerHost, dbname, ssl)
	}
	
	// Monta PostgreSQL padrão (direto - pode ser IPv6)
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
