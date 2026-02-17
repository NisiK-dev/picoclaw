// Package: database
// File: provider.go

package database

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Provider implementa DBProvider usando pgxpool nativo
type Provider struct {
	pool   *pgxpool.Pool
	config DBConfig
}

// NewDBProvider cria provider a partir de config
func NewDBProvider(config DBConfig) (DBProvider, error) {
	dbURL := getDatabaseURL()
	
	if dbURL == "" {
		return nil, fmt.Errorf("nenhuma variável de ambiente de banco de dados configurada")
	}

	// Log para debug (máscara a senha)
	maskedURL := maskPassword(dbURL)
	fmt.Printf("[database] Conectando com: %s\n", maskedURL)

	// Parse config com opções especiais para Supabase Pooler
	dbConfig, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		return nil, fmt.Errorf("erro ao parse config: %w", err)
	}

	// CORREÇÃO 1: Desabilita prepared statement cache (evita conflito com pooler)
	dbConfig.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeExec

	// CORREÇÃO 2: Configurações otimizadas para serverless/Render
	dbConfig.MaxConns = 5                    // Reduzido para evitar exaustão
	dbConfig.MinConns = 1                    // Mantém 1 conexão mínima
	dbConfig.MaxConnLifetime = 10 * time.Minute
	dbConfig.MaxConnIdleTime = 5 * time.Minute
	dbConfig.HealthCheckPeriod = 30 * time.Second

	// Cria pool
	pool, err := pgxpool.NewWithConfig(context.Background(), dbConfig)
	if err != nil {
		return nil, fmt.Errorf("erro ao criar pool: %w", err)
	}

	// Testa conexão
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("erro ao ping banco: %w", err)
	}

	fmt.Printf("[database] ✅ Conectado com sucesso (pool: %d max)\n", dbConfig.MaxConns)

	p := &Provider{
		pool:   pool,
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

// getDatabaseURL obtém a URL de conexão
func getDatabaseURL() string {
	// 1. DATABASE_URL direta (prioridade)
	if dbURL := os.Getenv("DATABASE_URL"); dbURL != "" {
		return normalizeSupabaseURL(dbURL)
	}

	// 2. SUPABASE_URL
	if dbURL := os.Getenv("SUPABASE_URL"); dbURL != "" {
		return normalizeSupabaseURL(dbURL)
	}

	// 3. DATABASE_POOLER_URL
	if dbURL := os.Getenv("DATABASE_POOLER_URL"); dbURL != "" {
		return dbURL
	}

	// 4. Monta de componentes
	return buildURLFromComponents()
}

// normalizeSupabaseURL ajusta URL do Supabase
func normalizeSupabaseURL(dbURL string) string {
	// Se já é pooler, retorna como está
	if strings.Contains(dbURL, "pooler.supabase.com") {
		return dbURL
	}

	// Se é direta, converte para pooler (IPv4 compatível)
	if strings.Contains(dbURL, "db.") && strings.Contains(dbURL, "supabase.co") {
		return convertDirectToPooler(dbURL)
	}

	return dbURL
}

// convertDirectToPooler converte URL direta para pooler
func convertDirectToPooler(directURL string) string {
	re := regexp.MustCompile(`postgresql://([^:]+):([^@]+)@db\.([^.]+)\.supabase\.co:(\d+)/([^?]+)(\?.*)?`)
	matches := re.FindStringSubmatch(directURL)
	
	if len(matches) < 6 {
		return directURL
	}

	user := matches[1]
	password := matches[2]
	projectRef := matches[3]
	database := matches[5]
	params := matches[6]
	if params == "" {
		params = "?sslmode=require"
	}

	// Pooler de transação (porta 6543)
	poolerURL := fmt.Sprintf(
		"postgresql://%s.%s:%s@aws-0-us-east-1.pooler.supabase.com:6543/%s%s",
		user, projectRef, password, database, params,
	)

	return poolerURL
}

// buildURLFromComponents monta URL de variáveis individuais
func buildURLFromComponents() string {
	host := getEnv("DB_HOST", "localhost")
	port := getEnv("DB_PORT", "5432")
	user := getEnv("DB_USER", "postgres")
	password := os.Getenv("DB_PASSWORD")
	dbname := getEnv("DB_NAME", "postgres")
	sslmode := getEnv("DB_SSLMODE", "require")

	// Converte Supabase direto para pooler
	if strings.Contains(host, "supabase.co") && !strings.Contains(host, "pooler") {
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

// maskPassword mascara senha no log
func maskPassword(dbURL string) string {
	re := regexp.MustCompile(`(postgresql://[^:]+):([^@]+)@`)
	return re.ReplaceAllString(dbURL, "$1:****@")
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// Connect verifica conexão
func (p *Provider) Connect(ctx context.Context) error {
	if p.pool == nil {
		return fmt.Errorf("pool not initialized")
	}
	return p.pool.Ping(ctx)
}

// Disconnect fecha pool
func (p *Provider) Disconnect() error {
	if p.pool != nil {
		p.pool.Close()
	}
	return nil
}

// IsConnected verifica saúde
func (p *Provider) IsConnected() bool {
	if p.pool == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return p.pool.Ping(ctx) == nil
}

// LoadSession carrega mensagens
func (p *Provider) LoadSession(ctx context.Context, chatID string) ([]Message, error) {
	return p.GetMessages(ctx, chatID, 100)
}

// SaveSession salva mensagens
func (p *Provider) SaveSession(ctx context.Context, chatID string, messages []Message) error {
	for _, msg := range messages {
		msg.ChatID = chatID
		if err := p.SaveMessage(ctx, &msg); err != nil {
			return err
		}
	}
	return nil
}

// SaveMessage salva mensagem (CORRIGIDO: aceita contexto)
func (p *Provider) SaveMessage(ctx context.Context, msg *Message) error {
	if msg.ID == "" {
		msg.ID = fmt.Sprintf("%d", time.Now().UnixNano())
	}
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}
	
	// Cria tabela se não existir
	_, _ = p.pool.Exec(ctx, `
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

	_, err := p.pool.Exec(ctx, `
		INSERT INTO messages (id, role, content, sender_id, chat_id, channel, timestamp)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (id) DO UPDATE SET
			role = EXCLUDED.role,
			content = EXCLUDED.content,
			timestamp = EXCLUDED.timestamp
	`, msg.ID, msg.Role, msg.Content, msg.SenderID, msg.ChatID, msg.Channel, msg.Timestamp)
	
	return err
}

// GetMessages recupera mensagens (CORRIGIDO: aceita contexto)
func (p *Provider) GetMessages(ctx context.Context, chatID string, limit int) ([]Message, error) {
	// Garante tabela
	_, _ = p.pool.Exec(ctx, `
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

	rows, err := p.pool.Query(ctx, `
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
	
	return messages, rows.Err()
}

// Exec executa query
func (p *Provider) Exec(ctx context.Context, query string, args ...interface{}) (pgconn.CommandTag, error) {
	if p.pool == nil {
		return pgconn.CommandTag{}, fmt.Errorf("pool not initialized")
	}
	return p.pool.Exec(ctx, query, args...)
}

// QueryRow executa query single row
func (p *Provider) QueryRow(ctx context.Context, query string, args ...interface{}) pgx.Row {
	if p.pool == nil {
		return nil
	}
	return p.pool.QueryRow(ctx, query, args...)
}

// Query executa query multi-row
func (p *Provider) Query(ctx context.Context, query string, args ...interface{}) (pgx.Rows, error) {
	if p.pool == nil {
		return nil, fmt.Errorf("pool not initialized")
	}
	return p.pool.Query(ctx, query, args...)
}

// Close fecha pool
func (p *Provider) Close() error {
	return p.Disconnect()
}
