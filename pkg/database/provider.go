package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type Provider struct {
	DB *sql.DB
}

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

	// Criar tabela se não existir
	_, _ = db.Exec(`
		CREATE TABLE IF NOT EXISTS messages (
			id TEXT PRIMARY KEY,
			role TEXT,
			content TEXT,
			sender_id TEXT,
			chat_id TEXT,
			channel TEXT,
			timestamp TIMESTAMPTZ,
			created_at TIMESTAMPTZ DEFAULT NOW()
		)
	`)

	return &Provider{DB: db}, nil
}

func (p *Provider) IsConnected() bool {
	if p.DB == nil {
		return false
	}
	return p.DB.Ping() == nil
}

// LoadSession retorna []Message diretamente (para range em loop.go:446)
func (p *Provider) LoadSession(ctx context.Context, chatID string) ([]Message, error) {
	return p.GetMessages(chatID, 100)
}

// SaveSession recebe []Message (compatível com loop.go:498)
func (p *Provider) SaveSession(ctx context.Context, chatID string, messages []Message) error {
	for _, msg := range messages {
		if err := p.SaveMessage(&msg); err != nil {
			return err
		}
	}
	return nil
}

func (p *Provider) SaveMessage(msg *Message) error {
	_, err := p.DB.Exec(`
		INSERT INTO messages (id, role, content, sender_id, chat_id, channel, timestamp)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (id) DO UPDATE SET
			role = EXCLUDED.role,
			content = EXCLUDED.content,
			timestamp = EXCLUDED.timestamp
	`, msg.ID, msg.Role, msg.Content, msg.SenderID, msg.ChatID, msg.Channel, msg.Timestamp)
	return err
}

func (p *Provider) GetMessages(chatID string, limit int) ([]Message, error) {
	rows, err := p.DB.Query(`
		SELECT id, role, content, sender_id, chat_id, channel, timestamp, created_at
		FROM messages 
		WHERE chat_id = $1 
		ORDER BY timestamp DESC 
		LIMIT $2
	`, chatID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var m Message
		err := rows.Scan(&m.ID, &m.Role, &m.Content, &m.SenderID, &m.ChatID, &m.Channel, &m.Timestamp, &m.CreatedAt)
		if err != nil {
			continue
		}
		messages = append(messages, m)
	}
	return messages, nil
}

func (p *Provider) Close() error {
	if p.DB != nil {
		return p.DB.Close()
	}
	return nil
}
