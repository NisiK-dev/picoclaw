package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// Provider implementa DBProvider para PostgreSQL/Supabase
type Provider struct {
	DB     *sql.DB
	config DBConfig
}

// NewProvider cria uma nova conexão com o banco
func NewProvider() (*Provider, error) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return nil, fmt.Errorf("DATABASE_URL não configurada")
	}

	db, err := sql.Open("pgx", dbURL)
	if err != nil {
		return nil, fmt.Errorf("erro ao abrir conexão: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("erro ao pingar banco: %w", err)
	}

	return &Provider{DB: db}, nil
}

// IsConnected verifica se a conexão está ativa
func (p *Provider) IsConnected() bool {
	if p.DB == nil {
		return false
	}
	return p.DB.Ping() == nil
}

// LoadSession carrega mensagens de uma sessão do banco (stub)
func (p *Provider) LoadSession(ctx context.Context, chatID string) ([]Message, error) {
	// TODO: implementar SELECT no banco
	return []Message{}, nil
}

// SaveSession salva mensagens de uma sessão no banco (stub)
func (p *Provider) SaveSession(ctx context.Context, chatID string, messages []Message) error {
	// TODO: implementar INSERT/UPDATE
	return nil
}

// SaveMessage salva uma mensagem no banco (stub)
func (p *Provider) SaveMessage(msg *Message) error {
	// TODO: implementar INSERT
	return nil
}

// GetMessages recupera mensagens do banco (stub)
func (p *Provider) GetMessages(chatID string, limit int) ([]Message, error) {
	// TODO: implementar SELECT
	return []Message{}, nil
}

// Close fecha a conexão com o banco
func (p *Provider) Close() error {
	if p.DB != nil {
		return p.DB.Close()
	}
	return nil
}
