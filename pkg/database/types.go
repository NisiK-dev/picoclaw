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
type DBProvider interface {
	IsConnected() bool
	Connect(ctx context.Context) error
	Disconnect() error
	LoadSession(ctx context.Context, chatID string) ([]Message, error)
	SaveSession(ctx context.Context, chatID string, messages []Message) error
	SaveMessage(ctx context.Context, msg *Message) error
	GetMessages(ctx context.Context, chatID string, limit int) ([]Message, error)
	Close() error
	
	// Métodos pgx nativos
	Exec(ctx context.Context, query string, args ...interface{}) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, query string, args ...interface{}) pgx.Row
	Query(ctx context.Context, query string, args ...interface{}) (pgx.Rows, error)
}

// Message modelo de mensagem
type Message struct {
	ID        string
	Role      string
	Content   string
	SenderID  string
	ChatID    string
	Channel   string
	Timestamp time.Time
	CreatedAt time.Time
}

// DBConfig configuração
type DBConfig struct {
	SupabaseURL string
}
