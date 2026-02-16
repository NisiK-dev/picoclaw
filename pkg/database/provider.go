package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"  // ← ADICIONADO: import do pacote time

	_ "github.com/jackc/pgx/v5/stdlib"
)

// Provider implementa DBProvider
type Provider struct {
	DB     *sql.DB
	config DBConfig
}

// NewDBProvider cria provider a partir de config (usado em main.go)
func NewDBProvider(config DBConfig) (DBProvider, error) {
	// Se tiver SupabaseURL, usa ela
	dbURL := config.SupabaseURL
	if dbURL == "" {
		dbURL = os.Getenv("DATABASE_URL")
	}
	if dbURL == "" {
		dbURL = config.GetConnectionString()
	}

	// Adiciona a chave do Supabase como parâmetro na URL se disponível
	if config.SupabaseKey != "" {
		separator := "?"
		if strings.Contains(dbURL, "?") {
			separator = "&"
		}
		dbURL = dbURL + separator + "apikey=" + config.SupabaseKey
	}

	db, err := sql.Open("pgx", dbURL)
	if err != nil {
		return nil, fmt.Errorf("erro ao abrir conexão: %w", err)
	}

	p := &Provider{
		DB:     db,
		config: config,
	}

	return p, nil
}
// NewProvider alias para compatibilidade
func NewProvider() (*Provider, error) {
	// ← CORRIGIDO: type assertion para converter DBProvider para *Provider
	dbProvider, err := NewDBProvider(DBConfig{})
	if err != nil {
		return nil, err
	}
	return dbProvider.(*Provider), nil
}

// Connect estabelece conexão com contexto (main.go)
func (p *Provider) Connect(ctx context.Context) error {
	if p.DB == nil {
		return fmt.Errorf("database not initialized")
	}
	return p.DB.PingContext(ctx)
}

// Disconnect fecha conexão (main.go)
func (p *Provider) Disconnect() error {
	return p.Close()
}

// IsConnected verifica conexão
func (p *Provider) IsConnected() bool {
	if p.DB == nil {
		return false
	}
	return p.DB.Ping() == nil
}

// LoadSession carrega mensagens (loop.go)
func (p *Provider) LoadSession(ctx context.Context, chatID string) ([]Message, error) {
	return p.GetMessages(chatID, 100)
}

// SaveSession salva mensagens (loop.go)
func (p *Provider) SaveSession(ctx context.Context, chatID string, messages []Message) error {
	for _, msg := range messages {
		msg.ChatID = chatID
		if err := p.SaveMessage(&msg); err != nil {
			return err
		}
	}
	return nil
}

// SaveMessage salva uma mensagem
func (p *Provider) SaveMessage(msg *Message) error {
	if msg.ID == "" {
		msg.ID = fmt.Sprintf("%d", time.Now().UnixNano())  // ← time agora funciona
	}
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()  // ← time agora funciona
	}
	
	// Cria tabela se não existir
	_, _ = p.DB.Exec(`
		CREATE TABLE IF NOT EXISTS messages (
			id TEXT PRIMARY KEY,
			role TEXT,
			content TEXT,
			sender_id TEXT,
			chat_id TEXT,
			channel TEXT,
			timestamp TIMESTAMPTZ DEFAULT NOW(),
			created_at TIMESTAMPTZ DEFAULT NOW()
		)
	`)

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

// GetMessages recupera mensagens
func (p *Provider) GetMessages(chatID string, limit int) ([]Message, error) {
	// Garante que tabela existe
	_, _ = p.DB.Exec(`
		CREATE TABLE IF NOT EXISTS messages (
			id TEXT PRIMARY KEY,
			role TEXT,
			content TEXT,
			sender_id TEXT,
			chat_id TEXT,
			channel TEXT,
			timestamp TIMESTAMPTZ DEFAULT NOW(),
			created_at TIMESTAMPTZ DEFAULT NOW()
		)
	`)

	rows, err := p.DB.Query(`
		SELECT id, role, content, sender_id, chat_id, channel, timestamp, created_at
		FROM messages 
		WHERE chat_id = $1 
		ORDER BY timestamp ASC 
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

// Close fecha conexão
func (p *Provider) Close() error {
	if p.DB != nil {
		return p.DB.Close()
	}
	return nil
}
