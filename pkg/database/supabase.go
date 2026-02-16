package database

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/sipeed/picoclaw/pkg/logger"
)

// SupabaseProvider implementa DBProvider para Supabase
type SupabaseProvider struct {
	config     DBConfig
	httpClient *http.Client
	headers    map[string]string
	connected  bool
}

func NewSupabaseProvider(config DBConfig) *SupabaseProvider {
	// Prioriza env vars se não configurado
	url := config.SupabaseURL
	if url == "" {
		url = os.Getenv("SUPABASE_URL")
	}
	key := config.SupabaseKey
	if key == "" {
		key = os.Getenv("SUPABASE_KEY")
	}

	return &SupabaseProvider{
		config: DBConfig{
			SupabaseURL: url,
			SupabaseKey: key,
			Database:    config.Database,
		},
		httpClient: &http.Client{Timeout: 30 * time.Second},
		headers: map[string]string{
			"apikey":        key,
			"Authorization": "Bearer " + key,
			"Content-Type":  "application/json",
		},
	}
}

func (s *SupabaseProvider) Connect(ctx context.Context) error {
	// Testa conexão fazendo uma query simples
	// FIX: Adicionada vírgula faltante entre ctx e "sessions"
	_, err := s.Query(ctx, "sessions", map[string]interface{}{"limit": 1})
	if err != nil {
		return fmt.Errorf("falha ao conectar ao Supabase: %w", err)
	}
	s.connected = true
	logger.InfoC("database", "✓ Conectado ao Supabase")
	return nil
}

func (s *SupabaseProvider) Disconnect() error {
	s.connected = false
	return nil
}

func (s *SupabaseProvider) IsConnected() bool {
	return s.connected
}

// request faz requisição HTTP para Supabase
func (s *SupabaseProvider) request(ctx context.Context, method, endpoint string, body interface{}) ([]byte, error) {
	url := fmt.Sprintf("%s/rest/v1/%s", s.config.SupabaseURL, endpoint)

	var bodyReader io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewBuffer(jsonBody)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, err
	}

	for k, v := range s.headers {
		req.Header.Set(k, v)
	}

	// Preferências para inserts/updates
	if method == "POST" || method == "PATCH" {
		req.Header.Set("Prefer", "return=representation")
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("Supabase error %d: %s", resp.StatusCode, string(responseBody))
	}

	return responseBody, nil
}

func (s *SupabaseProvider) Create(ctx context.Context, table string, data map[string]interface{}) (string, error) {
	// Adiciona timestamps
	data["created_at"] = time.Now().Format(time.RFC3339)
	data["updated_at"] = time.Now().Format(time.RFC3339)

	result, err := s.request(ctx, "POST", table, data)
	if err != nil {
		return "", err
	}

	// Extrai ID do resultado
	var records []map[string]interface{}
	if err := json.Unmarshal(result, &records); err != nil {
		return "", err
	}
	if len(records) > 0 && records[0]["id"] != nil {
		return fmt.Sprintf("%v", records[0]["id"]), nil
	}

	return "", nil
}

func (s *SupabaseProvider) Read(ctx context.Context, table string, id string) (map[string]interface{}, error) {
	endpoint := fmt.Sprintf("%s?id=eq.%s&limit=1", table, id)
	result, err := s.request(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}

	var records []map[string]interface{}
	if err := json.Unmarshal(result, &records); err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("registro não encontrado")
	}
	return records[0], nil
}

func (s *SupabaseProvider) Update(ctx context.Context, table string, id string, data map[string]interface{}) error {
	data["updated_at"] = time.Now().Format(time.RFC3339)
	endpoint := fmt.Sprintf("%s?id=eq.%s", table, id)
	_, err := s.request(ctx, "PATCH", endpoint, data)
	return err
}

func (s *SupabaseProvider) Delete(ctx context.Context, table string, id string) error {
	endpoint := fmt.Sprintf("%s?id=eq.%s", table, id)
	_, err := s.request(ctx, "DELETE", endpoint, nil)
	return err
}

func (s *SupabaseProvider) Query(ctx context.Context, table string, filters map[string]interface{}) ([]map[string]interface{}, error) {
	// Constrói query string
	query := table + "?"
	for k, v := range filters {
		query += fmt.Sprintf("%s=eq.%v&", k, v)
	}
	query += "order=created_at.desc"

	result, err := s.request(ctx, "GET", query, nil)
	if err != nil {
		return nil, err
	}

	var records []map[string]interface{}
	if err := json.Unmarshal(result, &records); err != nil {
		return nil, err
	}
	return records, nil
}

func (s *SupabaseProvider) QueryRaw(ctx context.Context, query string, args ...interface{}) ([]map[string]interface{}, error) {
	// Supabase REST não suporta SQL raw diretamente
	// Usa RPC (stored procedures) ou fallback para query simplificada
	return nil, fmt.Errorf("raw query não suportado em Supabase REST API. Use Query() ou crie uma RPC")
}

// ============== MÉTODOS ESPECÍFICOS DO AGENTE ==============

func (s *SupabaseProvider) SaveSession(ctx context.Context, sessionKey string, messages []Message) error {
	// Serializa mensagens
	messagesJSON, err := json.Marshal(messages)
	if err != nil {
		return err
	}

	data := map[string]interface{}{
		"session_key":   sessionKey,
		"messages":      string(messagesJSON),
		"message_count": len(messages),
		"updated_at":    time.Now().Format(time.RFC3339),
	}

	// Tenta update primeiro
	endpoint := fmt.Sprintf("sessions?session_key=eq.%s", sessionKey)
	_, err = s.request(ctx, "PATCH", endpoint, data)
	if err != nil {
		// Se não existe, cria
		data["created_at"] = time.Now().Format(time.RFC3339)
		_, err = s.request(ctx, "POST", "sessions", data)
	}

	return err
}

func (s *SupabaseProvider) LoadSession(ctx context.Context, sessionKey string) ([]Message, error) {
	endpoint := fmt.Sprintf("sessions?session_key=eq.%s&limit=1", sessionKey)
	result, err := s.request(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}

	var records []struct {
		SessionKey string `json:"session_key"`
		Messages   string `json:"messages"`
	}
	if err := json.Unmarshal(result, &records); err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return []Message{}, nil // Sessão nova
	}

	var messages []Message
	if err := json.Unmarshal([]byte(records[0].Messages), &messages); err != nil {
		return nil, err
	}
	return messages, nil
}

func (s *SupabaseProvider) SaveMemory(ctx context.Context, key string, content string, metadata map[string]interface{}) error {
	metadataJSON, _ := json.Marshal(metadata)

	data := map[string]interface{}{
		"key":      key,
		"content":  content,
		"metadata": string(metadataJSON),
	}

	// Upsert
	endpoint := fmt.Sprintf("memories?key=eq.%s", key)
	_, err := s.request(ctx, "PATCH", endpoint, data)
	if err != nil {
		data["created_at"] = time.Now().Format(time.RFC3339)
		_, err = s.request(ctx, "POST", "memories", data)
	}
	return err
}

func (s *SupabaseProvider) LoadMemory(ctx context.Context, key string) (string, error) {
	endpoint := fmt.Sprintf("memories?key=eq.%s&limit=1", key)
	result, err := s.request(ctx, "GET", endpoint, nil)
	if err != nil {
		return "", err
	}

	var records []struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(result, &records); err != nil {
		return "", err
	}
	if len(records) == 0 {
		return "", fmt.Errorf("memória não encontrada")
	}
	return records[0].Content, nil
}
