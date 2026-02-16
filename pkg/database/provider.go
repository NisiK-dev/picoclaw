package database

import (
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

// LoadSession carrega uma sessão do banco (stub)
func (p *Provider) LoadSession(chatID string) (*Session, error) {
	// TODO: implementar SELECT no banco
	return &Session{
		ID:     chatID,
		ChatID: chatID,
	}, nil
}

// SaveSession salva uma sessão no banco (stub)
func (p *Provider) SaveSession(session *Session) error {
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
