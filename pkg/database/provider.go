package database

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// Provider implementa DBProvider para PostgreSQL/Supabase
type Provider struct {
	DB *sql.DB
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

// SaveMessage salva uma mensagem no banco (stub - implementar depois)
func (p *Provider) SaveMessage(msg *Message) error {
	// TODO: implementar INSERT
	return nil
}

// GetMessages recupera mensagens do banco (stub - implementar depois)
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
