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

# LIMPAR WEBHOOK
curl -s "https://api.telegram.org/bot$TELEGRAM_TOKEN/deleteWebhook" > /dev/null

# CORRIGIR DATABASE URL
if [ -n "$DATABASE_URL" ]; then
    export DATABASE_URL=$(echo "$DATABASE_URL" | sed 's/:6543\//:5432\//g')
fi

# CRIAR HEARTBEAT.md
cat > "$HOME/.picoclaw/workspace/HEARTBEAT.md" << 'HEARTBEAT'
# Tarefas Automáticas

## Quick Tasks
- Reportar hora atual se for 12:00 (almoço) ou 18:00 (fim de expediente)

## Long Tasks (use spawn)
- Pesquisar últimas notícias de tecnologia e enviar resumo com 3 títulos principais
- Verificar previsão do tempo para amanhã e enviar alerta se chover
HEARTBEAT

echo "HEARTBEAT criado!"

# CRIAR CONFIG.JSON
cat > "$HOME/.picoclaw/config.json" << EOF
{
  "agents": {
    "defaults": {
      "workspace": "$HOME/.picoclaw/workspace",
      "restrict_to_workspace": true,
      "provider": "openrouter",
      "model": "openrouter/free",
      "max_tokens": 8192,
      "temperature": 0.7,
      "max_tool_iterations": 20
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
        "max_results": 5
      },
      "duckduckgo": {
        "enabled": true,
        "max_results": 5
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

echo "Config OK - Heartbeat ativo (30 min)!"
exec ./picoclaw gateway
