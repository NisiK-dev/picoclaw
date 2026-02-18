// PicoClaw - Ultra-lightweight personal AI agent
// Inspired by and based on nanobot: https://github.com/HKUDS/nanobot  
// License: MIT
//
// CORREÇÕES APLICADAS:
// - Sistema de raciocínio antes de usar LLMs (ReasoningEngine)
// - Respostas mais humanas com personalidade adaptativa
// - Fallback para múltiplos provedores de LLM
// - Cache de respostas para perguntas frequentes
//
// Copyright (c) 2026 PicoClaw contributors

package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/constants"
	"github.com/sipeed/picoclaw/pkg/database"
	"github.com/sipeed/picoclaw/pkg/logger"
	"github.com/sipeed/picoclaw/pkg/providers"
	"github.com/sipeed/picoclaw/pkg/session"
	"github.com/sipeed/picoclaw/pkg/state"
	"github.com/sipeed/picoclaw/pkg/tools"
	"github.com/sipeed/picoclaw/pkg/utils"
)

// AgentLoop gerencia o ciclo de vida do agente
type AgentLoop struct {
	bus            *bus.MessageBus
	provider       providers.LLMProvider
	providers      []providers.LLMProvider // Múltiplos provedores para fallback
	workspace      string
	model          string
	contextWindow  int // Maximum context window size in tokens
	maxIterations  int
	sessions       *session.SessionManager
	state          *state.Manager
	contextBuilder *ContextBuilder
	tools          *tools.ToolRegistry
	running        atomic.Bool
	summarizing    sync.Map // Tracks which sessions are currently being summarized
	dbProvider     database.DBProvider
	reasoning      *ReasoningEngine // NOVO: Motor de raciocínio
	responseCache  *ResponseCache   // NOVO: Cache de respostas
	personality    *Personality     // NOVO: Personalidade adaptativa
}

// processOptions configures how a message is processed
type processOptions struct {
	SessionKey      string // Session identifier for history/context
	Channel         string // Target channel for tool execution
	ChatID          string // Target chat ID for tool execution
	UserMessage     string // User message content (may include prefix)
	DefaultResponse string // Response when LLM returns empty
	EnableSummary   bool   // Whether to trigger summarization
	SendResponse    bool   // Whether to send response via bus
	NoHistory       bool   // If true, don't load/save session history (for heartbeat)
}

// ReasoningEngine implementa raciocínio antes de chamar LLMs
type ReasoningEngine struct {
	enabled         bool
	quickResponses  map[string]QuickResponse
	patternMatchers []PatternMatcher
}

// QuickResponse resposta rápida sem chamar LLM
type QuickResponse struct {
	Pattern     string
	Response    string
	Confidence  float64
	NeedsLLM    bool // Se true, ainda precisa validar com LLM
}

// PatternMatcher identifica padrões na mensagem
type PatternMatcher struct {
	Pattern     *regexp.Regexp
	Type        string
	Confidence  float64
}

// ResponseCache cache de respostas frequentes
type ResponseCache struct {
	mu       sync.RWMutex
	entries  map[string]CacheEntry
	maxSize  int
	ttl      time.Duration
}

// CacheEntry entrada no cache
type CacheEntry struct {
	Response   string
	Timestamp  time.Time
	HitCount   int
}

// Personality personalidade adaptativa do bot
type Personality struct {
	Name        string
	Tone        string // "friendly", "professional", "casual", "enthusiastic"
	EmojiStyle  string // "minimal", "moderate", "expressive"
	UseEmojis   bool
	Greeting    string
	Farewell    string
}

// createToolRegistry creates a tool registry with common tools.
// This is shared between main agent and subagents.
func createToolRegistry(workspace string, restrict bool, cfg *config.Config, msgBus *bus.MessageBus) *tools.ToolRegistry {
	registry := tools.NewToolRegistry()

	// File system tools
	registry.Register(tools.NewReadFileTool(workspace, restrict))
	registry.Register(tools.NewWriteFileTool(workspace, restrict))
	registry.Register(tools.NewListDirTool(workspace, restrict))
	registry.Register(tools.NewEditFileTool(workspace, restrict))
	registry.Register(tools.NewAppendFileTool(workspace, restrict))

	// Shell execution
	registry.Register(tools.NewExecTool(workspace, restrict))

	if searchTool := tools.NewWebSearchTool(tools.WebSearchToolOptions{
		BraveAPIKey:          cfg.Tools.Web.Brave.APIKey,
		BraveMaxResults:      cfg.Tools.Web.Brave.MaxResults,
		BraveEnabled:         cfg.Tools.Web.Brave.Enabled,
		DuckDuckGoMaxResults: cfg.Tools.Web.DuckDuckGo.MaxResults,
		DuckDuckGoEnabled:    cfg.Tools.Web.DuckDuckGo.Enabled,
	}); searchTool != nil {
		registry.Register(searchTool)
	}
	registry.Register(tools.NewWebFetchTool(50000))

	// Hardware tools (I2C, SPI) - Linux only, returns error on other platforms
	registry.Register(tools.NewI2CTool())
	registry.Register(tools.NewSPITool())

	// Message tool - available to both agent and subagent
	// Subagent uses it to communicate directly with user
	messageTool := tools.NewMessageTool()
	messageTool.SetSendCallback(func(channel, chatID, content string) error {
		msgBus.PublishOutbound(bus.OutboundMessage{
			Channel: channel,
			ChatID:  chatID,
			Content: content,
		})
		return nil
	})
	registry.Register(messageTool)

	return registry
}

// NewReasoningEngine cria um novo motor de raciocínio
func NewReasoningEngine() *ReasoningEngine {
	re := &ReasoningEngine{
		enabled:        true,
		quickResponses: make(map[string]QuickResponse),
		patternMatchers: []PatternMatcher{
			{regexp.MustCompile(`(?i)^oi$|^ol[aá]$|^eai$|^hey$|^hi$|^hello$`), "greeting", 0.95},
			{regexp.MustCompile(`(?i)^tchau$|^adeus$|^at[eé] logo$|^bye$|^see ya$`), "farewell", 0.95},
			{regexp.MustCompile(`(?i)^obrigad[oa]|^valeu|^thanks|^thank you`), "gratitude", 0.90},
			{regexp.MustCompile(`(?i)^bom dia$|^boa tarde$|^boa noite$`), "time_greeting", 0.95},
			{regexp.MustCompile(`(?i)^como voc[eê] est[aá]|^tudo bem|^how are you`), "how_are_you", 0.90},
			{regexp.MustCompile(`(?i)^quem [eé] voc[eê]|^o que [eé] voc[eê]|^what are you`), "who_are_you", 0.90},
			{regexp.MustCompile(`(?i)^ajuda|^help|^socorro|^me ajude`), "help_request", 0.85},
			{regexp.MustCompile(`(?i)^hora|^que horas|^time`), "time_request", 0.85},
			{regexp.MustCompile(`(?i)^data|^que dia|^date`), "date_request", 0.85},
		},
	}

	// Respostas rápidas predefinidas (em português para corresponder ao usuário)
	re.quickResponses["greeting"] = QuickResponse{
		Pattern:    "greeting",
		Response:   "",
		Confidence: 0.95,
		NeedsLLM:   true, // Gera saudação personalizada via LLM
	}
	re.quickResponses["farewell"] = QuickResponse{
		Pattern:    "farewell",
		Response:   "",
		Confidence: 0.95,
		NeedsLLM:   true,
	}
	re.quickResponses["gratitude"] = QuickResponse{
		Pattern:    "gratitude",
		Response:   "",
		Confidence: 0.90,
		NeedsLLM:   true,
	}
	re.quickResponses["how_are_you"] = QuickResponse{
		Pattern:    "how_are_you",
		Response:   "",
		Confidence: 0.90,
		NeedsLLM:   true,
	}
	re.quickResponses["who_are_you"] = QuickResponse{
		Pattern:    "who_are_you",
		Response:   "",
		Confidence: 0.90,
		NeedsLLM:   true,
	}
	re.quickResponses["time_request"] = QuickResponse{
		Pattern:    "time_request",
		Response:   "",
		Confidence: 0.85,
		NeedsLLM:   true,
	}
	re.quickResponses["date_request"] = QuickResponse{
		Pattern:    "date_request",
		Response:   "",
		Confidence: 0.85,
		NeedsLLM:   true,
	}
	re.quickResponses["help_request"] = QuickResponse{
		Pattern:    "help_request",
		Response:   "",
		Confidence: 0.85,
		NeedsLLM:   true,
	}

	return re
}

// Analyze analisa a mensagem e retorna o tipo detectado
func (re *ReasoningEngine) Analyze(message string) (string, float64) {
	if !re.enabled {
		return "", 0
	}

	for _, matcher := range re.patternMatchers {
		if matcher.Pattern.MatchString(message) {
			return matcher.Type, matcher.Confidence
		}
	}

	return "complex", 0.5 // Necessita processamento completo
}

// NewResponseCache cria um novo cache de respostas
func NewResponseCache() *ResponseCache {
	return &ResponseCache{
		entries: make(map[string]CacheEntry),
		maxSize: 100,
		ttl:     30 * time.Minute,
	}
}

// Get busca uma resposta no cache
func (rc *ResponseCache) Get(key string) (string, bool) {
	rc.mu.RLock()
	defer rc.mu.RUnlock()

	entry, exists := rc.entries[key]
	if !exists {
		return "", false
	}

	// Verifica se expirou
	if time.Since(entry.Timestamp) > rc.ttl {
		return "", false
	}

	return entry.Response, true
}

// Set adiciona uma resposta ao cache
func (rc *ResponseCache) Set(key, response string) {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	// Limpa entradas antigas se necessário
	if len(rc.entries) >= rc.maxSize {
		rc.cleanup()
	}

	rc.entries[key] = CacheEntry{
		Response:  response,
		Timestamp: time.Now(),
		HitCount:  1,
	}
}

// cleanup remove entradas expiradas ou menos usadas
func (rc *ResponseCache) cleanup() {
	now := time.Now()
	for key, entry := range rc.entries {
		if now.Sub(entry.Timestamp) > rc.ttl {
			delete(rc.entries, key)
		}
	}
}

// NewPersonality cria uma nova personalidade
func NewPersonality() *Personality {
	return &Personality{
		Name:       "Pico",
		Tone:       "friendly",
		EmojiStyle: "moderate",
		UseEmojis:  true,
		Greeting:   "Olá! 👋",
		Farewell:   "Até logo! 👋",
	}
}

// GenerateGreeting gera uma saudação personalizada
func (p *Personality) GenerateGreeting() string {
	hour := time.Now().Hour()
	var greeting string

	switch {
	case hour >= 5 && hour < 12:
		greeting = "Bom dia"
	case hour >= 12 && hour < 18:
		greeting = "Boa tarde"
	default:
		greeting = "Boa noite"
	}

	if p.UseEmojis {
		greetings := []string{
			fmt.Sprintf("%s! ☀️ Como posso ajudar você hoje?", greeting),
			fmt.Sprintf("%s! 🌟 O que posso fazer por você?", greeting),
			fmt.Sprintf("Oi! %s! 👋 Pronto para ajudar!", greeting),
		}
		return greetings[time.Now().Second()%len(greetings)]
	}

	return fmt.Sprintf("%s! Como posso ajudar?", greeting)
}

// GenerateFarewell gera uma despedida personalizada
func (p *Personality) GenerateFarewell() string {
	if p.UseEmojis {
		farewells := []string{
			"Até logo! 👋 Foi um prazer conversar com você!",
			"Tchau! 🌟 Volte sempre que precisar!",
			"Até mais! 😊 Estou aqui quando precisar!",
		}
		return farewells[time.Now().Second()%len(farewells)]
	}
	return "Até logo! Volte sempre."
}

// GenerateGratitudeResponse gera resposta para agradecimento
func (p *Personality) GenerateGratitudeResponse() string {
	if p.UseEmojis {
		responses := []string{
			"De nada! 😊 Fico feliz em poder ajudar!",
			"Por nada! 🌟 É um prazer ajudar!",
			"Disponha sempre! 👍 Que bom que pude ser útil!",
		}
		return responses[time.Now().Second()%len(responses)]
	}
	return "De nada! Fico feliz em ajudar."
}

// GenerateHowAreYouResponse gera resposta para "como vai"
func (p *Personality) GenerateHowAreYouResponse() string {
	if p.UseEmojis {
		return "Estou ótimo! 🤖 Funcionando a todo vapor e pronto para ajudar! E você, como está?"
	}
	return "Estou bem, obrigado por perguntar! Pronto para ajudar. E você?"
}

// GenerateWhoAreYouResponse gera resposta para "quem é você"
func (p *Personality) GenerateWhoAreYouResponse() string {
	if p.UseEmojis {
		return fmt.Sprintf("Sou %s! 🦞🤖 Um assistente de IA criado para ajudar você com diversas tarefas. Posso responder perguntas, ajudar com código, pesquisar na web e muito mais! Como posso ajudar?", p.Name)
	}
	return fmt.Sprintf("Sou %s, um assistente de IA pronto para ajudar você com diversas tarefas.", p.Name)
}

// GenerateTimeResponse gera resposta com hora atual
func (p *Personality) GenerateTimeResponse() string {
	now := time.Now()
	timeStr := now.Format("15:04")
	if p.UseEmojis {
		return fmt.Sprintf("São %s ⏰ (horário local)", timeStr)
	}
	return fmt.Sprintf("São %s (horário local)", timeStr)
}

// GenerateDateResponse gera resposta com data atual
func (p *Personality) GenerateDateResponse() string {
	now := time.Now()
	dateStr := now.Format("02/01/2006")
	weekday := []string{"Domingo", "Segunda-feira", "Terça-feira", "Quarta-feira", "Quinta-feira", "Sexta-feira", "Sábado"}[now.Weekday()]
	
	if p.UseEmojis {
		return fmt.Sprintf("Hoje é %s, %s 📅", weekday, dateStr)
	}
	return fmt.Sprintf("Hoje é %s, %s", weekday, dateStr)
}

// GenerateHelpResponse gera resposta para pedido de ajuda
func (p *Personality) GenerateHelpResponse() string {
	if p.UseEmojis {
		return `Claro! 🆘 Aqui estão algumas coisas que posso fazer:

💬 *Conversar* - Bate-papo natural sobre qualquer assunto
🔍 *Pesquisar* - Buscar informações na web
💻 *Código* - Ajuda com programação em várias linguagens
📁 *Arquivos* - Ler, escrever e editar arquivos
⚙️ *Ferramentas* - Usar diversas ferramentas disponíveis

O que você gostaria de fazer? 😊`
	}
	return `Posso ajudar com:
- Conversas e perguntas gerais
- Pesquisa na web
- Programação e código
- Manipulação de arquivos
- Uso de ferramentas diversas

Como posso ajudar?`
}

func NewAgentLoop(cfg *config.Config, msgBus *bus.MessageBus, provider providers.LLMProvider) *AgentLoop {
	workspace := cfg.WorkspacePath()
	os.MkdirAll(workspace, 0755)

	restrict := cfg.Agents.Defaults.RestrictToWorkspace

	// Create tool registry for main agent
	toolsRegistry := createToolRegistry(workspace, restrict, cfg, msgBus)

	// Create subagent manager with its own tool registry
	subagentManager := tools.NewSubagentManager(provider, cfg.Agents.Defaults.Model, workspace, msgBus)
	subagentTools := createToolRegistry(workspace, restrict, cfg, msgBus)
	// Subagent doesn't need spawn/subagent tools to avoid recursion
	subagentManager.SetTools(subagentTools)

	// Register spawn tool (for main agent)
	spawnTool := tools.NewSpawnTool(subagentManager)
	toolsRegistry.Register(spawnTool)

	// Register subagent tool (synchronous execution)
	subagentTool := tools.NewSubagentTool(subagentManager)
	toolsRegistry.Register(subagentTool)

	sessionsManager := session.NewSessionManager(filepath.Join(workspace, "sessions"))

	// Create state manager for atomic state persistence
	stateManager := state.NewManager(workspace)

	// Create context builder and set tools registry
	contextBuilder := NewContextBuilder(workspace)
	contextBuilder.SetToolsRegistry(toolsRegistry)

	return &AgentLoop{
		bus:            msgBus,
		provider:       provider,
		providers:      []providers.LLMProvider{provider}, // Inicializa com provider principal
		workspace:      workspace,
		model:          cfg.Agents.Defaults.Model,
		contextWindow:  cfg.Agents.Defaults.MaxTokens,
		maxIterations:  cfg.Agents.Defaults.MaxToolIterations,
		sessions:       sessionsManager,
		state:          stateManager,
		contextBuilder: contextBuilder,
		tools:          toolsRegistry,
		summarizing:    sync.Map{},
		dbProvider:     nil,
		reasoning:      NewReasoningEngine(),
		responseCache:  NewResponseCache(),
		personality:    NewPersonality(),
	}
}

// SetDBProvider injeta o provider de banco de dados
func (al *AgentLoop) SetDBProvider(db database.DBProvider) {
	al.dbProvider = db
	logger.InfoC("agent", "Database provider injetado no AgentLoop")
}

// GetDBProvider retorna o provider de banco de dados
func (al *AgentLoop) GetDBProvider() database.DBProvider {
	return al.dbProvider
}

// AddProvider adiciona um provedor de LLM adicional (para fallback)
func (al *AgentLoop) AddProvider(provider providers.LLMProvider) {
	al.providers = append(al.providers, provider)
	logger.InfoC("agent", fmt.Sprintf("Provedor adicional adicionado. Total: %d", len(al.providers)))
}

// SetPersonality define a personalidade do bot
func (al *AgentLoop) SetPersonality(tone string, useEmojis bool) {
	al.personality.Tone = tone
	al.personality.UseEmojis = useEmojis
	logger.InfoC("agent", fmt.Sprintf("Personalidade atualizada: tone=%s, emojis=%v", tone, useEmojis))
}

func (al *AgentLoop) Run(ctx context.Context) error {
	al.running.Store(true)

	for al.running.Load() {
		select {
		case <-ctx.Done():
			return nil
		default:
			msg, ok := al.bus.ConsumeInbound(ctx)
			if !ok {
				continue
			}

			response, err := al.processMessage(ctx, msg)
			if err != nil {
				response = fmt.Sprintf("Error processing message: %v", err)
			}

			if response != "" {
				// Check if the message tool already sent a response during this round.
				// If so, skip publishing to avoid duplicate messages to the user.
				alreadySent := false
				if tool, ok := al.tools.Get("message"); ok {
					if mt, ok := tool.(*tools.MessageTool); ok {
						alreadySent = mt.HasSentInRound()
					}
				}

				if !alreadySent {
					al.bus.PublishOutbound(bus.OutboundMessage{
						Channel: msg.Channel,
						ChatID:  msg.ChatID,
						Content: response,
					})
				}
			}
		}
	}

	return nil
}

func (al *AgentLoop) Stop() {
	al.running.Store(false)
}

func (al *AgentLoop) RegisterTool(tool tools.Tool) {
	al.tools.Register(tool)
}

// RecordLastChannel records the last active channel for this workspace.
// This uses the atomic state save mechanism to prevent data loss on crash.
func (al *AgentLoop) RecordLastChannel(channel string) error {
	return al.state.SetLastChannel(channel)
}

// RecordLastChatID records the last active chat ID for this workspace.
// This uses the atomic state save mechanism to prevent data loss on crash.
func (al *AgentLoop) RecordLastChatID(chatID string) error {
	return al.state.SetLastChatID(chatID)
}

func (al *AgentLoop) ProcessDirect(ctx context.Context, content, sessionKey string) (string, error) {
	return al.ProcessDirectWithChannel(ctx, content, sessionKey, "cli", "direct")
}

func (al *AgentLoop) ProcessDirectWithChannel(ctx context.Context, content, sessionKey, channel, chatID string) (string, error) {
	msg := bus.InboundMessage{
		Channel:    channel,
		SenderID:   "cron",
		ChatID:     chatID,
		Content:    content,
		SessionKey: sessionKey,
	}

	return al.processMessage(ctx, msg)
}

// ProcessHeartbeat processes a heartbeat request without session history.
// Each heartbeat is independent and doesn't accumulate context.
func (al *AgentLoop) ProcessHeartbeat(ctx context.Context, content, channel, chatID string) (string, error) {
	return al.runAgentLoop(ctx, processOptions{
		SessionKey:      fmt.Sprintf("heartbeat:%d", time.Now().Unix()),
		Channel:         channel,
		ChatID:          chatID,
		UserMessage:     content,
		DefaultResponse: "I've completed processing but have no response to give.",
		EnableSummary:   false,
		SendResponse:    false,
		NoHistory:       true,
	})
}

func (al *AgentLoop) processMessage(ctx context.Context, msg bus.InboundMessage) (string, error) {
	// Add message preview to log (show full content for error messages)
	var logContent string
	if strings.Contains(msg.Content, "Error:") || strings.Contains(msg.Content, "error") {
		logContent = msg.Content
	} else {
		logContent = utils.Truncate(msg.Content, 80)
	}
	logger.InfoCF("agent", fmt.Sprintf("Processing message from %s:%s: %s", msg.Channel, msg.SenderID, logContent),
		map[string]interface{}{
			"channel":     msg.Channel,
			"chat_id":     msg.ChatID,
			"sender_id":   msg.SenderID,
			"session_key": msg.SessionKey,
		})

	// Route system messages to processSystemMessage
	if msg.Channel == "system" {
		return al.processSystemMessage(ctx, msg)
	}

	// Process as user message
	return al.runAgentLoop(ctx, processOptions{
		SessionKey:      msg.SessionKey,
		Channel:         msg.Channel,
		ChatID:          msg.ChatID,
		UserMessage:     msg.Content,
		DefaultResponse: "I've completed processing but have no response to give.",
		EnableSummary:   true,
		SendResponse:    false,
	})
}

func (al *AgentLoop) processSystemMessage(ctx context.Context, msg bus.InboundMessage) (string, error) {
	// Verify this is a system message
	if msg.Channel != "system" {
		return "", fmt.Errorf("processSystemMessage called with non-system message channel: %s", msg.Channel)
	}

	logger.InfoCF("agent", "Processing system message",
		map[string]interface{}{
			"sender_id": msg.SenderID,
			"chat_id":   msg.ChatID,
		})

	// Parse origin channel from chat_id (format: "channel:chat_id")
	var originChannel string
	if idx := strings.Index(msg.ChatID, ":"); idx > 0 {
		originChannel = msg.ChatID[:idx]
	} else {
		originChannel = "cli"
	}

	// Extract subagent result from message content
	content := msg.Content
	if idx := strings.Index(content, "Result:\n"); idx >= 0 {
		content = content[idx+8:]
	}

	// Skip internal channels - only log, don't send to user
	if constants.IsInternalChannel(originChannel) {
		logger.InfoCF("agent", "Subagent completed (internal channel)",
			map[string]interface{}{
				"sender_id":   msg.SenderID,
				"content_len": len(content),
				"channel":     originChannel,
			})
		return "", nil
	}

	logger.InfoCF("agent", "Subagent completed",
		map[string]interface{}{
			"sender_id":   msg.SenderID,
			"channel":     originChannel,
			"content_len": len(content),
		})

	return "", nil
}

// runAgentLoop is the core message processing logic.
// CORREÇÃO: Agora inclui raciocínio antes de chamar LLMs
func (al *AgentLoop) runAgentLoop(ctx context.Context, opts processOptions) (string, error) {
	// 0. Record last channel for heartbeat notifications (skip internal channels)
	if opts.Channel != "" && opts.ChatID != "" {
		if !constants.IsInternalChannel(opts.Channel) {
			channelKey := fmt.Sprintf("%s:%s", opts.Channel, opts.ChatID)
			if err := al.RecordLastChannel(channelKey); err != nil {
				logger.WarnCF("agent", "Failed to record last channel: %v", map[string]interface{}{"error": err.Error()})
			}
		}
	}

	// ============================================
	// 1. SISTEMA DE RACIOCÍNIO (NOVO)
	// ============================================
	// Analisa a mensagem antes de processar
	messageType, confidence := al.reasoning.Analyze(opts.UserMessage)
	
	logger.DebugCF("agent", "Raciocínio aplicado",
		map[string]interface{}{
			"message_type": messageType,
			"confidence":   confidence,
		})

	// Verifica cache primeiro
	cacheKey := fmt.Sprintf("%s:%s", messageType, utils.HashString(opts.UserMessage))
	if cachedResponse, found := al.responseCache.Get(cacheKey); found && messageType != "complex" {
		logger.InfoC("agent", "Resposta encontrada no cache")
		return cachedResponse, nil
	}

	// Respostas rápidas para padrões comuns (sem chamar LLM)
	var quickResponse string
	switch messageType {
	case "greeting":
		quickResponse = al.personality.GenerateGreeting()
	case "farewell":
		quickResponse = al.personality.GenerateFarewell()
	case "gratitude":
		quickResponse = al.personality.GenerateGratitudeResponse()
	case "how_are_you":
		quickResponse = al.personality.GenerateHowAreYouResponse()
	case "who_are_you":
		quickResponse = al.personality.GenerateWhoAreYouResponse()
	case "time_request":
		quickResponse = al.personality.GenerateTimeResponse()
	case "date_request":
		quickResponse = al.personality.GenerateDateResponse()
	case "help_request":
		quickResponse = al.personality.GenerateHelpResponse()
	}

	// Se temos uma resposta rápida e confiança é alta, retorna diretamente
	if quickResponse != "" && confidence >= 0.85 && messageType != "complex" {
		logger.InfoC("agent", fmt.Sprintf("Resposta rápida para: %s", messageType))
		
		// Salva no cache
		al.responseCache.Set(cacheKey, quickResponse)
		
		// Salva no histórico se necessário
		if !opts.NoHistory {
			al.sessions.AddMessage(opts.SessionKey, "user", opts.UserMessage)
			al.sessions.AddMessage(opts.SessionKey, "assistant", quickResponse)
			al.sessions.Save(opts.SessionKey)
			al.saveMessageToDB(ctx, opts.SessionKey, "user", opts.UserMessage)
			al.saveMessageToDB(ctx, opts.SessionKey, "assistant", quickResponse)
		}
		
		return quickResponse, nil
	}

	// ============================================
	// 2. PROCESSAMENTO NORMAL COM LLM
	// ============================================
	al.updateToolContexts(opts.Channel, opts.ChatID)

	var history []providers.Message
	var summary string
	if !opts.NoHistory {
		history = al.loadSessionFromDB(ctx, opts.SessionKey)
		if history == nil {
			history = al.sessions.GetHistory(opts.SessionKey)
		}
		summary = al.sessions.GetSummary(opts.SessionKey)
	}
	messages := al.contextBuilder.BuildMessages(
		history,
		summary,
		opts.UserMessage,
		nil,
		opts.Channel,
		opts.ChatID,
	)

	// Save user message to session
	if !opts.NoHistory {
		al.sessions.AddMessage(opts.SessionKey, "user", opts.UserMessage)
		al.saveMessageToDB(ctx, opts.SessionKey, "user", opts.UserMessage)
	}

	// Run LLM iteration loop
	finalContent, iteration, err := al.runLLMIteration(ctx, messages, opts)
	if err != nil {
		return "", err
	}

	if finalContent == "" {
		finalContent = opts.DefaultResponse
	}

	// Save final assistant message to session
	if !opts.NoHistory {
		al.sessions.AddMessage(opts.SessionKey, "assistant", finalContent)
		al.saveMessageToDB(ctx, opts.SessionKey, "assistant", finalContent)
		al.sessions.Save(opts.SessionKey)
		al.saveSessionToDB(ctx, opts.SessionKey)

		if opts.EnableSummary {
			al.maybeSummarize(opts.SessionKey)
		}
	}

	// Save to cache for future use
	if messageType != "complex" && len(finalContent) < 500 {
		al.responseCache.Set(cacheKey, finalContent)
	}

	// Optional: send response via bus
	if opts.SendResponse && !isToolCallFormat(finalContent) {
		al.bus.PublishOutbound(bus.OutboundMessage{
			Channel: opts.Channel,
			ChatID:  opts.ChatID,
			Content: finalContent,
		})
	}

	// Log response
	responsePreview := utils.Truncate(finalContent, 120)
	logger.InfoCF("agent", fmt.Sprintf("Response: %s", responsePreview),
		map[string]interface{}{
			"session_key":  opts.SessionKey,
			"iterations":   iteration,
			"final_length": len(finalContent),
		})

	return finalContent, nil
}

// loadSessionFromDB carrega sessão do banco de dados
func (al *AgentLoop) loadSessionFromDB(ctx context.Context, sessionKey string) []providers.Message {
	if al.dbProvider == nil || !al.dbProvider.IsConnected() {
		return nil
	}

	messages, err := al.dbProvider.LoadSession(ctx, sessionKey)
	if err != nil {
		logger.DebugC("database", "Sessão não encontrada no DB: "+err.Error())
		return nil
	}

	var result []providers.Message
	for _, msg := range messages {
		result = append(result, providers.Message{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	logger.DebugC("database", fmt.Sprintf("Sessão %s carregada do DB: %d mensagens", sessionKey, len(result)))
	return result
}

// saveMessageToDB salva mensagem individual no banco
func (al *AgentLoop) saveMessageToDB(ctx context.Context, sessionKey, role, content string) {
	if al.dbProvider == nil || !al.dbProvider.IsConnected() {
		return
	}

	if strings.HasPrefix(sessionKey, "heartbeat:") {
		return
	}

	messages, _ := al.dbProvider.LoadSession(ctx, sessionKey)
	
	messages = append(messages, database.Message{
		ID:        fmt.Sprintf("%d", time.Now().UnixNano()),
		Role:      role,
		Content:   content,
		CreatedAt: time.Now(),
	})

	if err := al.dbProvider.SaveSession(ctx, sessionKey, messages); err != nil {
		logger.WarnC("database", "Falha ao salvar mensagem no DB: "+err.Error())
	}
}

// saveSessionToDB salva sessão completa no banco
func (al *AgentLoop) saveSessionToDB(ctx context.Context, sessionKey string) {
	if al.dbProvider == nil || !al.dbProvider.IsConnected() {
		return
	}

	if strings.HasPrefix(sessionKey, "heartbeat:") {
		return
	}

	localHistory := al.sessions.GetHistory(sessionKey)
	var messages []database.Message
	for _, msg := range localHistory {
		messages = append(messages, database.Message{
			ID:        fmt.Sprintf("%d", time.Now().UnixNano()),
			Role:      msg.Role,
			Content:   msg.Content,
			CreatedAt: time.Now(),
		})
	}

	if err := al.dbProvider.SaveSession(ctx, sessionKey, messages); err != nil {
		logger.WarnC("database", "Falha ao salvar sessão no DB: "+err.Error())
	} else {
		logger.DebugC("database", "Sessão salva no DB: "+sessionKey)
	}
}

// runLLMIteration executes the LLM call loop with tool handling.
// CORREÇÃO: Agora suporta fallback para múltiplos provedores
func (al *AgentLoop) runLLMIteration(ctx context.Context, messages []providers.Message, opts processOptions) (string, int, error) {
	iteration := 0
	var finalContent string

	for iteration < al.maxIterations {
		iteration++

		logger.DebugCF("agent", "LLM iteration",
			map[string]interface{}{
				"iteration": iteration,
				"max":       al.maxIterations,
			})

		// Build tool definitions
		providerToolDefs := al.tools.ToProviderDefs()

		// Log LLM request details
		logger.DebugCF("agent", "LLM request",
			map[string]interface{}{
				"iteration":      iteration,
				"model":          al.model,
				"messages_count": len(messages),
				"tools_count":    len(providerToolDefs),
				"max_tokens":     8192,
				"temperature":    0.7,
			})

		// Call LLM com fallback para múltiplos provedores
		var response *providers.LLMResponse
		var err error
		
		for i, provider := range al.providers {
			if i > 0 {
				logger.WarnC("agent", fmt.Sprintf("Tentando provedor fallback %d...", i+1))
			}
			
			response, err = provider.Chat(ctx, messages, providerToolDefs, al.model, map[string]interface{}{
				"max_tokens":  8192,
				"temperature": 0.7,
			})
			
			if err == nil {
				break // Sucesso, sai do loop
			}
			
			logger.ErrorC("agent", fmt.Sprintf("Provedor %d falhou: %v", i+1, err))
		}

		if err != nil {
			logger.ErrorCF("agent", "Todos os provedores falharam",
				map[string]interface{}{
					"iteration": iteration,
					"error":     err.Error(),
				})
			return "", iteration, fmt.Errorf("LLM call failed: %w", err)
		}

		// Check if no tool calls - we're done
		if len(response.ToolCalls) == 0 {
			finalContent = response.Content
			logger.InfoCF("agent", "LLM response without tool calls (direct answer)",
				map[string]interface{}{
					"iteration":     iteration,
					"content_chars": len(finalContent),
				})
			break
		}

		// Log tool calls
		toolNames := make([]string, 0, len(response.ToolCalls))
		for _, tc := range response.ToolCalls {
			toolNames = append(toolNames, tc.Name)
		}
		logger.InfoCF("agent", "LLM requested tool calls",
			map[string]interface{}{
				"tools":     toolNames,
				"count":     len(response.ToolCalls),
				"iteration": iteration,
			})

		// Build assistant message with tool calls
		assistantMsg := providers.Message{
			Role:    "assistant",
			Content: response.Content,
		}
		for _, tc := range response.ToolCalls {
			argumentsJSON, _ := json.Marshal(tc.Arguments)
			assistantMsg.ToolCalls = append(assistantMsg.ToolCalls, providers.ToolCall{
				ID:   tc.ID,
				Type: "function",
				Function: &providers.FunctionCall{
					Name:      tc.Name,
					Arguments: string(argumentsJSON),
				},
			})
		}
		messages = append(messages, assistantMsg)

		// Save assistant message with tool calls to session
		if !opts.NoHistory {
			al.sessions.AddFullMessage(opts.SessionKey, assistantMsg)
		}

		// Execute tool calls
		for _, tc := range response.ToolCalls {
			argsJSON, _ := json.Marshal(tc.Arguments)
			argsPreview := utils.Truncate(string(argsJSON), 200)
			logger.InfoCF("agent", fmt.Sprintf("Tool call: %s(%s)", tc.Name, argsPreview),
				map[string]interface{}{
					"tool":      tc.Name,
					"iteration": iteration,
				})

			asyncCallback := func(callbackCtx context.Context, result *tools.ToolResult) {
				if !result.Silent && result.ForUser != "" {
					logger.InfoCF("agent", "Async tool completed",
						map[string]interface{}{
							"tool":        tc.Name,
							"content_len": len(result.ForUser),
						})
				}
			}

			toolResult := al.tools.ExecuteWithContext(ctx, tc.Name, tc.Arguments, opts.Channel, opts.ChatID, asyncCallback)

			// Send ForUser content to user immediately if not Silent
			if !toolResult.Silent && toolResult.ForUser != "" && opts.SendResponse {
				al.bus.PublishOutbound(bus.OutboundMessage{
					Channel: opts.Channel,
					ChatID:  opts.ChatID,
					Content: toolResult.ForUser,
				})
				logger.DebugCF("agent", "Sent tool result to user",
					map[string]interface{}{
						"tool":        tc.Name,
						"content_len": len(toolResult.ForUser),
					})
			}

			// Determine content for LLM based on tool result
			contentForLLM := toolResult.ForLLM
			if contentForLLM == "" && toolResult.Err != nil {
				contentForLLM = toolResult.Err.Error()
			}

			toolResultMsg := providers.Message{
				Role:       "tool",
				Content:    contentForLLM,
				ToolCallID: tc.ID,
			}
			messages = append(messages, toolResultMsg)

			// Save tool result message to session
			if !opts.NoHistory {
				al.sessions.AddFullMessage(opts.SessionKey, toolResultMsg)
			}
		}
	}

	return finalContent, iteration, nil
}

// updateToolContexts updates the context for tools that need channel/chatID info.
func (al *AgentLoop) updateToolContexts(channel, chatID string) {
	if tool, ok := al.tools.Get("message"); ok {
		if mt, ok := tool.(tools.ContextualTool); ok {
			mt.SetContext(channel, chatID)
		}
	}
	if tool, ok := al.tools.Get("spawn"); ok {
		if st, ok := tool.(tools.ContextualTool); ok {
			st.SetContext(channel, chatID)
		}
	}
	if tool, ok := al.tools.Get("subagent"); ok {
		if st, ok := tool.(tools.ContextualTool); ok {
			st.SetContext(channel, chatID)
		}
	}
}

// maybeSummarize triggers summarization if the session history exceeds thresholds.
func (al *AgentLoop) maybeSummarize(sessionKey string) {
	if strings.HasPrefix(sessionKey, "heartbeat:") {
		return
	}

	newHistory := al.sessions.GetHistory(sessionKey)
	tokenEstimate := al.estimateTokens(newHistory)
	threshold := al.contextWindow * 75 / 100

	if len(newHistory) > 20 || tokenEstimate > threshold {
		if _, loading := al.summarizing.LoadOrStore(sessionKey, true); !loading {
			go func() {
				defer al.summarizing.Delete(sessionKey)
				al.summarizeSession(sessionKey)
			}()
		}
	}
}

// GetStartupInfo returns information about loaded tools and skills for logging.
func (al *AgentLoop) GetStartupInfo() map[string]interface{} {
	info := make(map[string]interface{})

	// Tools info
	toolsList := al.tools.List()
	info["tools"] = map[string]interface{}{
		"count": len(toolsList),
		"names": toolsList,
	}

	// Skills info
	info["skills"] = al.contextBuilder.GetSkillsInfo()

	// Database info
	if al.dbProvider != nil && al.dbProvider.IsConnected() {
		info["database"] = "connected"
	} else {
		info["database"] = "not connected"
	}

	// Reasoning info
	info["reasoning"] = al.reasoning.enabled
	info["personality"] = al.personality.Name

	return info
}

// formatMessagesForLog formats messages for logging
func formatMessagesForLog(messages []providers.Message) string {
	if len(messages) == 0 {
		return "[]"
	}

	var result string
	result += "[\n"
	for i, msg := range messages {
		result += fmt.Sprintf("  [%d] Role: %s\n", i, msg.Role)
		if msg.ToolCalls != nil && len(msg.ToolCalls) > 0 {
			result += "  ToolCalls:\n"
			for _, tc := range msg.ToolCalls {
				result += fmt.Sprintf("    - ID: %s, Type: %s, Name: %s\n", tc.ID, tc.Type, tc.Name)
				if tc.Function != nil {
					result += fmt.Sprintf("      Arguments: %s\n", utils.Truncate(tc.Function.Arguments, 200))
				}
			}
		}
		if msg.Content != "" {
			content := utils.Truncate(msg.Content, 200)
			result += fmt.Sprintf("  Content: %s\n", content)
		}
		if msg.ToolCallID != "" {
			result += fmt.Sprintf("  ToolCallID: %s\n", msg.ToolCallID)
		}
		result += "\n"
	}
	result += "]"
	return result
}

// formatToolsForLog formats tool definitions for logging
func formatToolsForLog(tools []providers.ToolDefinition) string {
	if len(tools) == 0 {
		return "[]"
	}

	var result string
	result += "[\n"
	for i, tool := range tools {
		result += fmt.Sprintf("  [%d] Type: %s, Name: %s\n", i, tool.Type, tool.Function.Name)
		result += fmt.Sprintf("      Description: %s\n", tool.Function.Description)
		if len(tool.Function.Parameters) > 0 {
			result += fmt.Sprintf("      Parameters: %s\n", utils.Truncate(fmt.Sprintf("%v", tool.Function.Parameters), 200))
		}
	}
	result += "]"
	return result
}

// summarizeSession summarizes the conversation history for a session.
func (al *AgentLoop) summarizeSession(sessionKey string) {
	if strings.HasPrefix(sessionKey, "heartbeat:") {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	history := al.sessions.GetHistory(sessionKey)
	summary := al.sessions.GetSummary(sessionKey)

	if len(history) <= 4 {
		return
	}

	toSummarize := history[:len(history)-4]

	maxMessageTokens := al.contextWindow / 2
	validMessages := make([]providers.Message, 0)
	omitted := false

	for _, m := range toSummarize {
		if m.Role != "user" && m.Role != "assistant" {
			continue
		}
		msgTokens := len(m.Content) / 4
		if msgTokens > maxMessageTokens {
			omitted = true
			continue
		}
		validMessages = append(validMessages, m)
	}

	if len(validMessages) == 0 {
		return
	}

	var finalSummary string
	if len(validMessages) > 10 {
		mid := len(validMessages) / 2
		part1 := validMessages[:mid]
		part2 := validMessages[mid:]

		s1, _ := al.summarizeBatch(ctx, part1, "")
		s2, _ := al.summarizeBatch(ctx, part2, "")

		mergePrompt := fmt.Sprintf("Merge these two conversation summaries into one cohesive summary:\n\n1: %s\n\n2: %s", s1, s2)
		resp, err := al.provider.Chat(ctx, []providers.Message{{Role: "user", Content: mergePrompt}}, nil, al.model, map[string]interface{}{
			"max_tokens":  1024,
			"temperature": 0.3,
		})
		if err == nil {
			finalSummary = resp.Content
		} else {
			finalSummary = s1 + " " + s2
		}
	} else {
		finalSummary, _ = al.summarizeBatch(ctx, validMessages, summary)
	}

	if omitted && finalSummary != "" {
		finalSummary += "\n[Note: Some oversized messages were omitted from this summary for efficiency.]"
	}

	if finalSummary != "" {
		al.sessions.SetSummary(sessionKey, finalSummary)
		al.sessions.TruncateHistory(sessionKey, 4)
		al.sessions.Save(sessionKey)
		
		al.saveSessionToDB(ctx, sessionKey)
	}
}

// summarizeBatch summarizes a batch of messages.
func (al *AgentLoop) summarizeBatch(ctx context.Context, batch []providers.Message, existingSummary string) (string, error) {
	prompt := "Provide a concise summary of this conversation segment, preserving core context and key points.\n"
	if existingSummary != "" {
		prompt += "Existing context: " + existingSummary + "\n"
	}
	prompt += "\nCONVERSATION:\n"
	for _, m := range batch {
		prompt += fmt.Sprintf("%s: %s\n", m.Role, m.Content)
	}

	response, err := al.provider.Chat(ctx, []providers.Message{{Role: "user", Content: prompt}}, nil, al.model, map[string]interface{}{
		"max_tokens":  1024,
		"temperature": 0.3,
	})
	if err != nil {
		return "", err
	}
	return response.Content, nil
}

// estimateTokens estimates the number of tokens in a message list.
func (al *AgentLoop) estimateTokens(messages []providers.Message) int {
	total := 0
	for _, m := range messages {
		total += len(m.Content) / 4
	}
	return total
}

// isToolCallFormat verifica se o conteúdo é formato interno de tool call
func isToolCallFormat(content string) bool {
	if content == "" {
		return false
	}
	
	patterns := []string{
		"(message={",
		"(web_fetch={",
		"(search={",
		"(exec={",
		"(read_file={",
		"(write_file={",
		"(list_dir={",
		"(spawn={",
		"(subagent={",
		"(append_file={",
		"(edit_file={",
		"(i2c={",
		"(spi={",
	}
	
	for _, pattern := range patterns {
		if strings.Contains(content, pattern) {
			return true
		}
	}
	return false
}
