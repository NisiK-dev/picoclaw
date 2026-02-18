// Package: database
// File: types.go

package database

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// DBProvider interface atualizada para pgx
// Todos os métodos aceitam context.Context como primeiro parâmetro
// para permitir timeout e cancelamento adequados
type DBProvider interface {
	IsConnected() bool
	Connect(ctx context.Context) error
	Disconnect() error
	LoadSession(ctx context.Context, chatID string) ([]Message, error)
	SaveSession(ctx context.Context, chatID string, messages []Message) error
	SaveMessage(ctx context.Context, msg *Message) error
	GetMessages(ctx context.Context, chatID string, limit int) ([]Message, error)
	Close() error
	
	// Métodos pgx nativos - TODOS aceitam context.Context como primeiro parâmetro
	Exec(ctx context.Context, query string, args ...interface{}) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, query string, args ...interface{}) pgx.Row
	Query(ctx context.Context, query string, args ...interface{}) (pgx.Rows, error)
}

// Message modelo de mensagem
// Representa uma única mensagem na conversa
type Message struct {
	ID        string    // Identificador único da mensagem
	Role      string    // "user", "assistant", "system", "tool"
	Content   string    // Conteúdo da mensagem
	SenderID  string    // ID do remetente (para canais)
	ChatID    string    // ID do chat/sessão
	Channel   string    // Canal de origem (telegram, cli, etc)
	Timestamp time.Time // Quando a mensagem foi enviada
	CreatedAt time.Time // Quando a mensagem foi armazenada
}

// Session representa uma sessão de conversa completa
type Session struct {
	ID        string    // ID único da sessão
	ChatID    string    // ID do chat
	Channel   string    // Canal de origem
	Messages  []Message // Mensagens da sessão
	Summary   string    // Resumo da conversa
	CreatedAt time.Time // Quando a sessão foi criada
	UpdatedAt time.Time // Última atualização
}

// DBConfig configuração para conexão com banco de dados
type DBConfig struct {
	SupabaseURL string // URL completa do Supabase (ou outro PostgreSQL)
	MaxConns    int32  // Máximo de conexões no pool
	MinConns    int32  // Mínimo de conexões no pool
}

// LockManager interface para locks distribuídos usando PostgreSQL advisory locks
type LockManager interface {
	// TryAcquire tenta adquirir um lock sem bloquear (pg_try_advisory_lock)
	// Retorna true se conseguiu, false se não conseguiu
	TryAcquire(ctx context.Context, lockID int64) (bool, error)
	
	// Acquire adquire um lock bloqueante (pg_advisory_lock)
	// Cuidado: pode causar deadlock se não for liberado!
	Acquire(ctx context.Context, lockID int64) error
	
	// Release libera um lock específico
	Release(ctx context.Context, lockID int64) error
	
	// ReleaseAll libera todos os locks da sessão atual
	ReleaseAll(ctx context.Context) error
}

// MachineState representa o estado de uma "máquina" virtual no banco de dados
// Todas as sessões compartilham o mesmo estado, simulando uma única máquina
type MachineState struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Data        map[string]interface{} `json:"data"`        // Dados persistentes da máquina
	Preferences map[string]interface{} `json:"preferences"` // Preferências do usuário
	Memory      map[string]interface{} `json:"memory"`      // Memória de curto prazo
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

// MachineStateStore interface para gerenciar estado da máquina
type MachineStateStore interface {
	// Load carrega o estado da máquina
	Load(ctx context.Context, machineID string) (*MachineState, error)
	
	// Save salva o estado da máquina
	Save(ctx context.Context, state *MachineState) error
	
	// UpdateField atualiza um campo específico do estado
	UpdateField(ctx context.Context, machineID string, field string, value interface{}) error
	
	// GetField recupera um campo específico do estado
	GetField(ctx context.Context, machineID string, field string) (interface{}, error)
}
