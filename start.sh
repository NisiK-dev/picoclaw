#!/bin/sh

mkdir -p "$HOME/.picoclaw/workspace"
cp -r workspace/* "$HOME/.picoclaw/workspace/"

OPENROUTER_KEY="$OPENROUTER_API_KEY"
TELEGRAM_TOKEN="$TELEGRAM_BOT_TOKEN"
SERPER_KEY="$SERPER_API_KEY"

echo "=== DEBUG ==="
echo "OPENROUTER_KEY definida: $(if [ -n "$OPENROUTER_KEY" ]; then echo SIM; else echo NAO; fi)"
echo "TELEGRAM_TOKEN definida: $(if [ -n "$TELEGRAM_TOKEN" ]; then echo SIM; else echo NAO; fi)"
echo "SERPER_KEY definida: $(if [ -n "$SERPER_KEY" ]; then echo SIM; else echo NAO; fi)"
echo "============="

if [ -z "$OPENROUTER_KEY" ]; then
    echo "ERROR: OPENROUTER_API_KEY nao esta definida!"
    exit 1
fi

if [ -z "$TELEGRAM_TOKEN" ]; then
    echo "ERROR: TELEGRAM_BOT_TOKEN nao esta definida!"
    exit 1
fi

# LIMPAR WEBHOOK DO TELEGRAM
echo "Limpando webhook do Telegram..."
curl -s "https://api.telegram.org/bot$TELEGRAM_TOKEN/deleteWebhook" > /dev/null
echo "OK"

if [ -n "$DATABASE_URL" ]; then
    export DATABASE_URL=$(echo "$DATABASE_URL" | sed 's/:6543\//:5432\//g')
    echo "Database OK"
fi

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
    "enabled": false,
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

echo "Config criado com sucesso!"
echo "- Serper: $(if [ -n "$SERPER_KEY" ]; then echo ATIVO; else echo DESATIVADO; fi)"
echo "- DuckDuckGo: ATIVO (backup)"

exec ./picoclaw gateway
