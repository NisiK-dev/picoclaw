package database

import (
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

	return &Provider{DB: db}, nil
}

func (p *Provider) Close() error {
	if p.DB != nil {
		return p.DB.Close()
	}
	return nil
}
