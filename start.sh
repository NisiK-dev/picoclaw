#!/bin/sh

# Criar diretórios necessários
mkdir -p "$HOME/.picoclaw/workspace"
cp -r workspace/* "$HOME/.picoclaw/workspace/"

# Pegar as variáveis de ambiente
OPENROUTER_KEY="$OPENROUTER_API_KEY"
TELEGRAM_TOKEN="$TELEGRAM_BOT_TOKEN"
TELEGRAM_USER_ID="$TELEGRAM_USER_ID"
SERPER_KEY="$SERPER_API_KEY"

echo "=== DEBUG ==="
echo "TELEGRAM_BOT_TOKEN length: ${#TELEGRAM_TOKEN}"
echo "TELEGRAM_USER_ID: $TELEGRAM_USER_ID"
echo "OPENROUTER_KEY length: ${#OPENROUTER_KEY}"
echo "============="

# Verificar se a API key do OpenRouter está definida
if [ -z "$OPENROUTER_KEY" ]; then
    echo "ERROR: OPENROUTER_API_KEY nao esta definida!"
    exit 1
fi

# Verificar se o token do Telegram está definido
if [ -z "$TELEGRAM_TOKEN" ]; then
    echo "ERROR: TELEGRAM_BOT_TOKEN nao esta definida!"
    exit 1
else
    echo "OK: TELEGRAM_BOT_TOKEN definido"
fi

# Corrigir a connection string do Supabase
if [ -n "$DATABASE_URL" ]; then
    FIXED_DATABASE_URL=$(echo "$DATABASE_URL" | sed 's/:6543\//:5432\//g')
    export DATABASE_URL="$FIXED_DATABASE_URL"
    echo "Database URL corrigida"
fi

# Criar config.json
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
      "super": {
        "enabled": true,
        "api_key": "$SERPER_KEY",
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

echo "Config created!"
echo "Token no config: $(grep -o '"token": "[^"]*' $HOME/.picoclaw/config.json | head -1)"

# Iniciar
exec ./picoclaw gateway
