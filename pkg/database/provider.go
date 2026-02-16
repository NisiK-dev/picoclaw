package database

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/lib/pq" // Driver PostgreSQL
)

// Provider gerencia a conexão com o banco
type Provider struct {
	DB *sql.DB
}

// NewProvider cria uma conexão usando a URI do Supabase
func NewProvider() (*Provider, error) {
	// Pega a URI das variáveis de ambiente (Render/Supabase)
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return nil, fmt.Errorf("DATABASE_URL não configurada")
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		return nil, fmt.Errorf("erro ao abrir conexão: %w", err)
	}

	// Testa a conexão
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("erro ao conectar no banco: %w", err)
	}

	// Configurações recomendadas para pool de conexões (Render/Supabase)
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)

	return &Provider{DB: db}, nil
}

// Close fecha a conexão
func (p *Provider) Close() error {
	return p.DB.Close()
}
