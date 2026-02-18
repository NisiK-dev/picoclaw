// Package: database
// File: provider.go

package database

import (
	"context"
	"encoding/json"
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
// Inclui suporte a MachineStateStore para simular uma única máquina
type Provider struct {
	pool        *pgxpool.Pool
	config      DBConfig
	machineID   string // ID da máquina virtual única
}

// NewDBProvider cria provider a partir de config
// Configura automaticamente o pool de conexões para otimização
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
	// Usa valores da config ou defaults seguros
	if config.MaxConns > 0 {
		dbConfig.MaxConns = config.MaxConns
	} else {
		dbConfig.MaxConns = 5 // Reduzido para evitar exaustão
	}
	if config.MinConns > 0 {
		dbConfig.MinConns = config.MinConns
	} else {
		dbConfig.MinConns = 1 // Mantém 1 conexão mínima
	}
	dbConfig.MaxConnLifetime = 10 * time.Minute
	dbConfig.MaxConnIdleTime = 5 * time.Minute
	dbConfig.HealthCheckPeriod = 30 * time.Second

	// Cria pool
	pool, err := pgxpool.NewWithConfig(context.Background(), dbConfig)
	if err != nil {
		return nil, fmt.Errorf("erro ao criar pool: %w", err)
	}

	// Testa conexão com timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("erro ao ping banco: %w", err)
	}

	fmt.Printf("[database] ✅ Conectado com sucesso (pool: %d max)\n", dbConfig.MaxConns)

	p := &Provider{
		pool:      pool,
		config:    config,
		machineID: "picoclaw-main", // ID único da máquina virtual
	}

	// Inicializa schema
	if err := p.initSchema(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("erro ao inicializar schema: %w", err)
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

// initSchema cria as tabelas necessárias se não existirem
func (p *Provider) initSchema(ctx context.Context) error {
	// Tabela de mensagens
	_, err := p.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS messages (
			id TEXT PRIMARY KEY,
			role TEXT NOT NULL,
			content TEXT NOT NULL,
			sender_id TEXT,
			chat_id TEXT NOT NULL,
			channel TEXT,
			timestamp TIMESTAMPTZ DEFAULT NOW(),
			created_at TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_messages_chat_id ON messages(chat_id);
		CREATE INDEX IF NOT EXISTS idx_messages_timestamp ON messages(timestamp);
	`)
	if err != nil {
		return fmt.Errorf("erro ao criar tabela messages: %w", err)
	}

	// Tabela de estado da máquina (simula uma única máquina)
	_, err = p.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS machine_state (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			data JSONB DEFAULT '{}',
			preferences JSONB DEFAULT '{}',
			memory JSONB DEFAULT '{}',
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW()
		);
	`)
	if err != nil {
		return fmt.Errorf("erro ao criar tabela machine_state: %w", err)
	}

	// Tabela de sessões
	_, err = p.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			chat_id TEXT NOT NULL UNIQUE,
			channel TEXT,
			summary TEXT DEFAULT '',
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_sessions_chat_id ON sessions(chat_id);
	`)
	if err != nil {
		return fmt.Errorf("erro ao criar tabela sessions: %w", err)
	}

	// Tabela de locks distribuídos (para controle de instâncias)
	_, err = p.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS distributed_locks (
			lock_id BIGINT PRIMARY KEY,
			owner TEXT NOT NULL,
			acquired_at TIMESTAMPTZ DEFAULT NOW(),
			expires_at TIMESTAMPTZ
		);
	`)
	if err != nil {
		return fmt.Errorf("erro ao criar tabela distributed_locks: %w", err)
	}

	// Insere máquina principal se não existir
	_, err = p.pool.Exec(ctx, `
		INSERT INTO machine_state (id, name, data, preferences, memory)
		VALUES ($1, $2, '{}', '{}', '{}')
		ON CONFLICT (id) DO NOTHING
	`, p.machineID, "PicoClaw Main Machine")
	if err != nil {
		return fmt.Errorf("erro ao inserir máquina principal: %w", err)
	}

	fmt.Printf("[database] ✅ Schema inicializado\n")
	return nil
}

// getDatabaseURL obtém a URL de conexão
// Prioridade: DATABASE_URL > SUPABASE_URL > DATABASE_POOLER_URL > variáveis individuais
func getDatabaseURL() string {
	// 1. DATABASE_URL direta (prioridade máxima)
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

	// 4. Monta de componentes individuais
	return buildURLFromComponents()
}

// normalizeSupabaseURL ajusta URL do Supabase
// Converte conexões diretas (IPv6) para pooler (IPv4) automaticamente
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
// Necessário porque Render não suporta IPv6
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

	// Detecta a região do projeto para usar o pooler correto
	region := "us-east-1" // default
	if strings.Contains(directURL, "eu-") {
		region = "eu-west-1"
	} else if strings.Contains(directURL, "ap-") {
		region = "ap-southeast-1"
	}

	// Pooler de transação (porta 6543)
	poolerURL := fmt.Sprintf(
		"postgresql://%s.%s:%s@aws-0-%s.pooler.supabase.com:6543/%s%s",
		user, projectRef, password, region, database, params,
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
			region := "us-east-1"
			if strings.Contains(host, "eu-") {
				region = "eu-west-1"
			} else if strings.Contains(host, "ap-") {
				region = "ap-southeast-1"
			}
			return fmt.Sprintf(
				"postgresql://%s.%s:%s@aws-0-%s.pooler.supabase.com:6543/%s?sslmode=%s",
				user, projectRef, password, region, dbname, sslmode,
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

// ============================================
// IMPLEMENTAÇÃO DBProvider
// ============================================

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

// IsConnected verifica saúde da conexão
func (p *Provider) IsConnected() bool {
	if p.pool == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return p.pool.Ping(ctx) == nil
}

// LoadSession carrega mensagens de uma sessão
func (p *Provider) LoadSession(ctx context.Context, chatID string) ([]Message, error) {
	return p.GetMessages(ctx, chatID, 100)
}

// SaveSession salva mensagens de uma sessão
func (p *Provider) SaveSession(ctx context.Context, chatID string, messages []Message) error {
	for _, msg := range messages {
		msg.ChatID = chatID
		if err := p.SaveMessage(ctx, &msg); err != nil {
			return err
		}
	}
	return nil
}

// SaveMessage salva uma mensagem individual
func (p *Provider) SaveMessage(ctx context.Context, msg *Message) error {
	if msg.ID == "" {
		msg.ID = fmt.Sprintf("%d", time.Now().UnixNano())
	}
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}
	
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

// GetMessages recupera mensagens de um chat
func (p *Provider) GetMessages(ctx context.Context, chatID string, limit int) ([]Message, error) {
	if limit <= 0 {
		limit = 100
	}

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

// Close fecha o pool
func (p *Provider) Close() error {
	return p.Disconnect()
}

// Exec executa uma query
func (p *Provider) Exec(ctx context.Context, query string, args ...interface{}) (pgconn.CommandTag, error) {
	if p.pool == nil {
		return pgconn.CommandTag{}, fmt.Errorf("pool not initialized")
	}
	return p.pool.Exec(ctx, query, args...)
}

// QueryRow executa query de uma única linha
func (p *Provider) QueryRow(ctx context.Context, query string, args ...interface{}) pgx.Row {
	if p.pool == nil {
		return nil
	}
	return p.pool.QueryRow(ctx, query, args...)
}

// Query executa query de múltiplas linhas
func (p *Provider) Query(ctx context.Context, query string, args ...interface{}) (pgx.Rows, error) {
	if p.pool == nil {
		return nil, fmt.Errorf("pool not initialized")
	}
	return p.pool.Query(ctx, query, args...)
}

// ============================================
// IMPLEMENTAÇÃO LockManager
// ============================================

// TryAcquire tenta adquirir um lock sem bloquear
// Usa pg_try_advisory_lock - retorna imediatamente
func (p *Provider) TryAcquire(ctx context.Context, lockID int64) (bool, error) {
	var acquired bool
	err := p.pool.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", lockID).Scan(&acquired)
	return acquired, err
}

// Acquire adquire um lock bloqueante
// CUIDADO: Pode causar deadlock! Use com timeout
func (p *Provider) Acquire(ctx context.Context, lockID int64) error {
	_, err := p.pool.Exec(ctx, "SELECT pg_advisory_lock($1)", lockID)
	return err
}

// Release libera um lock específico
func (p *Provider) Release(ctx context.Context, lockID int64) error {
	_, err := p.pool.Exec(ctx, "SELECT pg_advisory_unlock($1)", lockID)
	return err
}

// ReleaseAll libera todos os locks da sessão
func (p *Provider) ReleaseAll(ctx context.Context) error {
	_, err := p.pool.Exec(ctx, "SELECT pg_advisory_unlock_all()")
	return err
}

// ============================================
// IMPLEMENTAÇÃO MachineStateStore
// Simula uma única máquina compartilhada entre todas as sessões
// ============================================

// LoadMachineState carrega o estado da máquina virtual
func (p *Provider) LoadMachineState(ctx context.Context) (*MachineState, error) {
	var state MachineState
	var dataJSON, prefsJSON, memoryJSON []byte

	err := p.pool.QueryRow(ctx, `
		SELECT id, name, data, preferences, memory, created_at, updated_at
		FROM machine_state
		WHERE id = $1
	`, p.machineID).Scan(
		&state.ID, &state.Name, &dataJSON, &prefsJSON, &memoryJSON,
		&state.CreatedAt, &state.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("erro ao carregar estado da máquina: %w", err)
	}

	// Deserializa JSONB
	if err := json.Unmarshal(dataJSON, &state.Data); err != nil {
		state.Data = make(map[string]interface{})
	}
	if err := json.Unmarshal(prefsJSON, &state.Preferences); err != nil {
		state.Preferences = make(map[string]interface{})
	}
	if err := json.Unmarshal(memoryJSON, &state.Memory); err != nil {
		state.Memory = make(map[string]interface{})
	}

	return &state, nil
}

// SaveMachineState salva o estado da máquina virtual
func (p *Provider) SaveMachineState(ctx context.Context, state *MachineState) error {
	dataJSON, _ := json.Marshal(state.Data)
	prefsJSON, _ := json.Marshal(state.Preferences)
	memoryJSON, _ := json.Marshal(state.Memory)

	_, err := p.pool.Exec(ctx, `
		INSERT INTO machine_state (id, name, data, preferences, memory, updated_at)
		VALUES ($1, $2, $3, $4, $5, NOW())
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			data = EXCLUDED.data,
			preferences = EXCLUDED.preferences,
			memory = EXCLUDED.memory,
			updated_at = EXCLUDED.updated_at
	`, p.machineID, state.Name, dataJSON, prefsJSON, memoryJSON)

	return err
}

// UpdateMachineField atualiza um campo específico do estado
func (p *Provider) UpdateMachineField(ctx context.Context, field string, value interface{}) error {
	var query string
	switch field {
	case "data":
		query = `UPDATE machine_state SET data = $2, updated_at = NOW() WHERE id = $1`
	case "preferences":
		query = `UPDATE machine_state SET preferences = $2, updated_at = NOW() WHERE id = $1`
	case "memory":
		query = `UPDATE machine_state SET memory = $2, updated_at = NOW() WHERE id = $1`
	default:
		return fmt.Errorf("campo inválido: %s", field)
	}

	jsonValue, _ := json.Marshal(value)
	_, err := p.pool.Exec(ctx, query, p.machineID, jsonValue)
	return err
}

// GetMachineField recupera um campo específico do estado
func (p *Provider) GetMachineField(ctx context.Context, field string) (map[string]interface{}, error) {
	var query string
	switch field {
	case "data":
		query = `SELECT data FROM machine_state WHERE id = $1`
	case "preferences":
		query = `SELECT preferences FROM machine_state WHERE id = $1`
	case "memory":
		query = `SELECT memory FROM machine_state WHERE id = $1`
	default:
		return nil, fmt.Errorf("campo inválido: %s", field)
	}

	var jsonValue []byte
	err := p.pool.QueryRow(ctx, query, p.machineID).Scan(&jsonValue)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(jsonValue, &result); err != nil {
		return make(map[string]interface{}), nil
	}
	return result, nil
}

// GetMachineID retorna o ID da máquina virtual
func (p *Provider) GetMachineID() string {
	return p.machineID
}
