// Package: database
// File: provider.go

package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// Provider implementa DBProvider
type Provider struct {
	DB     *sql.DB
	config DBConfig
}

// NewDBProvider cria provider a partir de config (usado em main.go)
func NewDBProvider(config DBConfig) (DBProvider, error) {
	dbURL := getDatabaseURL()
	
	if dbURL == "" {
		return nil, fmt.Errorf("nenhuma variável de ambiente de banco de dados configurada")
	}

	// Log para debug (máscara a senha)
	maskedURL := maskPassword(dbURL)
	fmt.Printf("[database] Conectando com: %s\n", maskedURL)

	db, err := sql.Open("pgx", dbURL)
	if err != nil {
		return nil, fmt.Errorf("erro ao abrir conexão: %w", err)
	}

	// Configura pool de conexões para ambiente serverless
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(2 * time.Minute)

	p := &Provider{
		DB:     db,
		config: config,
	}

	return p, nil
}

// NewProvider alias para compatibilidade
func NewProvider() (*Provider, error) {
	dbProvider, err := NewDBProvider(DBConfig{})
	if err != nil {
		return nil, err
	}
	return dbProvider.(*Provider), nil
}

// getDatabaseURL obtém a URL de conexão do banco de dados
// Prioridade: DATABASE_URL > SUPABASE_URL > variáveis individuais
func getDatabaseURL() string {
	// 1. Tenta DATABASE_URL primeiro (formato padrão do Render/Supabase)
	if dbURL := os.Getenv("DATABASE_URL"); dbURL != "" {
		// Verifica se é uma URL do Supabase e precisa de ajustes
		return normalizeSupabaseURL(dbURL)
	}

	// 2. Tenta SUPABASE_URL (específica do Supabase)
	if dbURL := os.Getenv("SUPABASE_URL"); dbURL != "" {
		return normalizeSupabaseURL(dbURL)
	}

	// 3. Tenta DATABASE_POOLER_URL (alternativa)
	if dbURL := os.Getenv("DATABASE_POOLER_URL"); dbURL != "" {
		return dbURL
	}

	// 4. Monta a partir de variáveis individuais
	return buildURLFromComponents()
}

// normalizeSupabaseURL ajusta a URL do Supabase para funcionar corretamente
func normalizeSupabaseURL(dbURL string) string {
	// Se já é pooler, retorna como está
	if strings.Contains(dbURL, "pooler.supabase.com") {
		return dbURL
	}

	// Se é conexão direta (db.xxx.supabase.co), converte para pooler
	// Padrão: postgresql://postgres:password@db.PROJECT_REF.supabase.co:5432/postgres
	if strings.Contains(dbURL, "db.") && strings.Contains(dbURL, "supabase.co") {
		return convertDirectToPooler(dbURL)
	}

	return dbURL
}

// convertDirectToPooler converte URL direta do Supabase para pooler
func convertDirectToPooler(directURL string) string {
	// Extrai componentes da URL usando regex
	// postgresql://user:pass@host:port/db?params
	
	re := regexp.MustCompile(`postgresql://([^:]+):([^@]+)@db\.([^.]+)\.supabase\.co:(\d+)/([^?]+)(\?.*)?`)
	matches := re.FindStringSubmatch(directURL)
	
	if len(matches) < 6 {
		// Não conseguiu fazer parse, retorna original
		return directURL
	}

	user := matches[1]
	password := matches[2]
	projectRef := matches[3]
	// port := matches[4] // ignoramos, pooler usa 6543
	database := matches[5]
	params := matches[6]
	if params == "" {
		params = "?sslmode=require"
	}

	// Constrói URL do pooler de transação (IPv4 compatível)
	// Formato: postgresql://user.project_ref:password@aws-0-region.pooler.supabase.com:6543/database
	poolerURL := fmt.Sprintf(
		"postgresql://%s.%s:%s@aws-0-us-east-1.pooler.supabase.com:6543/%s%s",
		user, projectRef, password, database, params,
	)

	return poolerURL
}

// buildURLFromComponents monta URL a partir de variáveis individuais
func buildURLFromComponents() string {
	host := getEnv("DB_HOST", "localhost")
	port := getEnv("DB_PORT", "5432")
	user := getEnv("DB_USER", "postgres")
	password := os.Getenv("DB_PASSWORD")
	dbname := getEnv("DB_NAME", "postgres")
	sslmode := getEnv("DB_SSLMODE", "require")

	// Se for Supabase direto, converte para pooler
	if strings.Contains(host, "supabase.co") && !strings.Contains(host, "pooler") {
		// Extrai project ref do host (db.xxx.supabase.co)
		parts := strings.Split(host, ".")
		if len(parts) >= 3 && parts[0] == "db" {
			projectRef := parts[1]
			return fmt.Sprintf(
				"postgresql://%s.%s:%s@aws-0-us-east-1.pooler.supabase.com:6543/%s?sslmode=%s",
				user, projectRef, password, dbname, sslmode,
			)
		}
	}

	return fmt.Sprintf(
		"postgresql://%s:%s@%s:%s/%s?sslmode=%s",
		user, password, host, port, dbname, sslmode,
	)
}

// maskPassword mascara a senha em uma URL para logging
func maskPassword(dbURL string) string {
	// Regex para encontrar senha em URL postgresql
	re := regexp.MustCompile(`(postgresql://[^:]+):([^@]+)@`)
	return re.ReplaceAllString(dbURL, "$1:****@")
}

// getEnv obtém variável de ambiente ou retorna valor padrão
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
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
		msg.ID = fmt.Sprintf("%d", time.Now().UnixNano())
	}
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
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

// Exec executa uma query sem retornar linhas
func (p *Provider) Exec(query string, args ...interface{}) (sql.Result, error) {
	if p.DB == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	return p.DB.Exec(query, args...)
}

// QueryRow executa uma query retornando uma única linha
func (p *Provider) QueryRow(query string, args ...interface{}) *sql.Row {
	if p.DB == nil {
		return nil
	}
	return p.DB.QueryRow(query, args...)
}

// Query executa uma query retornando múltiplas linhas
func (p *Provider) Query(query string, args ...interface{}) (*sql.Rows, error) {
	if p.DB == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	return p.DB.Query(query, args...)
}

// Close fecha conexão
func (p *Provider) Close() error {
	if p.DB != nil {
		return p.DB.Close()
	}
	return nil
}
