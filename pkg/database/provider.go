package database

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// DBProvider interface unificada para qualquer banco de dados
type DBProvider interface {
	// Conexão
	Connect(ctx context.Context) error
	Disconnect() error
	IsConnected() bool

	// CRUD Básico
	Create(ctx context.Context, table string, data map[string]interface{}) (string, error)
	Read(ctx context.Context, table string, id string) (map[string]interface{}, error)
	Update(ctx context.Context, table string, id string, data map[string]interface{}) error
	Delete(ctx context.Context, table string, id string) error

	// Queries
	Query(ctx context.Context, table string, filters map[string]interface{}) ([]map[string]interface{}, error)
	QueryRaw(ctx context.Context, query string, args ...interface{}) ([]map[string]interface{}, error)

	// Específico para agente
	SaveSession(ctx context.Context, sessionKey string, messages []Message) error
	LoadSession(ctx context.Context, sessionKey string) ([]Message, error)
	SaveMemory(ctx context.Context, key string, content string, metadata map[string]interface{}) error
	LoadMemory(ctx context.Context, key string) (string, error)
}

// Message estrutura unificada de mensagem
type Message struct {
	ID        string                 `json:"id"`
	Role      string                 `json:"role"` // user, assistant, system, tool
	Content   string                 `json:"content"`
	Metadata  map[string]interface{} `json:"metadata"`
	CreatedAt time.Time              `json:"created_at"`
}

// DBConfig configuração genérica de banco
type DBConfig struct {
	Driver   string // "postgres", "mysql", "sqlite", "supabase"
	Host     string
	Port     string
	Database string
	Username string
	Password string
	SSLMode  string
	
	// Específico Supabase
	SupabaseURL string
	SupabaseKey string
	
	// Específico SQLite
	SQLitePath string
}

// NewDBProvider factory para criar provider correto
func NewDBProvider(config DBConfig) (DBProvider, error) {
	switch config.Driver {
	case "postgres", "postgresql":
		return NewPostgresProvider(config), nil
	case "supabase":
		return NewSupabaseProvider(config), nil
	case "sqlite":
		return NewSQLiteProvider(config), nil
	case "mysql":
		return NewMySQLProvider(config), nil
	default:
		return nil, fmt.Errorf("driver não suportado: %s", config.Driver)
	}
}
