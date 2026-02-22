#!/bin/sh

mkdir -p "$HOME/.picoclaw/workspace"
cp -r workspace/* "$HOME/.picoclaw/workspace/"

OPENROUTER_KEY="$OPENROUTER_API_KEY"
TELEGRAM_TOKEN="$TELEGRAM_BOT_TOKEN"
SERPER_KEY="$SERPER_API_KEY"

if [ -z "$OPENROUTER_KEY" ]; then
    echo "ERROR: OPENROUTER_API_KEY nao esta definida!"
    exit 1
fi

if [ -z "$TELEGRAM_TOKEN" ]; then
    echo "ERROR: TELEGRAM_BOT_TOKEN nao esta definida!"
    exit 1
fi

# LIMPAR WEBHOOK (CORRIGIDO - sem espaço)
curl -s "https://api.telegram.org/bot$TELEGRAM_TOKEN/deleteWebhook" > /dev/null

# CORRIGIR DATABASE URL
if [ -n "$DATABASE_URL" ]; then
    export DATABASE_URL=$(echo "$DATABASE_URL" | sed 's/:6543\//:5432\//g')
fi

# CRIAR HEARTBEAT.md
cat > "$HOME/.picoclaw/workspace/HEARTBEAT.md" << 'HEARTBEAT'
# Tarefas Automáticas

## Quick Tasks
- Reportar hora atual se for 12:00 ou 18:00

## Long Tasks (use spawn)
- Pesquisar notícias de tecnologia e enviar resumo
HEARTBEAT

# CRIAR IDENTITY.md
cat > "$HOME/.picoclaw/workspace/IDENTITY.md" << 'IDENTITY'
# Identidade do MASSA INCEFALICA 🧠

Você é o **MASSA INCEFALICA** 🧠, um assistente pessoal amigável e descontraído.

## Estilo de comunicação:
- Converse como um amigo, não como um professor
- Respostas curtas e diretas (máximo 2-3 parágrafos)
- Use emojis com moderação 😊
- Faça perguntas para engajar o usuário
- Evite textos longos copiados da internet
- Seja proativo, mas não invasivo

## Tom:
- Informal mas respeitoso
- Entusiasmado com tecnologia
- Paciente com dúvidas
- Usa humor leve quando apropriado

## Regras:
- Sempre divida informações complexas em tópicos
- Confirme entendimento antes de executar ações
- Peça permissão antes de fazer buscas ou ações externas
IDENTITY

echo "Arquivos de config criados!"

# CRIAR CONFIG.JSON COM cognitivecomputations/dolphin-mistral-24b-venice-edition:free
cat > "$HOME/.picoclaw/config.json" << EOF
{
  "agents": {
    "defaults": {
      "workspace": "$HOME/.picoclaw/workspace",
      "restrict_to_workspace": true,
      "provider": "openrouter",
      "model": "cognitivecomputations/dolphin-mistral-24b-venice-edition:free",
      "max_tokens": 1024,
      "temperature": 0.9,
      "max_tool_iterations": 10
    }
  },
  "channels": {
    "telegram": {
      "enabled": true,
      "token": "$TELEGRAM_TOKEN",
      "proxy": "",
      "allow_from": []
    }
  },
  "providers": {
    "openrouter": {
      "api_key": "$OPENROUTER_KEY",
      "api_base": "https://openrouter.ai/api/v1"
    }
  },
  "tools": {
    "web": {
      "serper": {
        "enabled": true,
        "api_key": "$SERPER_KEY",
        "max_results": 3
      },
      "duckduckgo": {
        "enabled": true,
        "max_results": 3
      }
    }
  },
  "heartbeat": {
    "enabled": true,
    "interval": 30
  },
  "devices": {
    "enabled": false,
    "monitor_usb": true
  },
  "gateway": {
    "host": "0.0.0.0",
    "port": 10000
  }
}
EOF

echo "Config OK - Modelo: dolphin-mistral-24b-venice-edition:free!"
exec ./picoclaw gateway
