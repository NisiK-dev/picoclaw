#!/bin/sh

# Criar diretórios necessários
mkdir -p "$HOME/.picoclaw/workspace"
cp -r workspace/* "$HOME/.picoclaw/workspace/"

# Pegar as variáveis de ambiente
OPENROUTER_KEY="$OPENROUTER_API_KEY"
TELEGRAM_TOKEN="$TELEGRAM_BOT_TOKEN"
TELEGRAM_USER_ID="$TELEGRAM_USER_ID"

# Verificar se a API key do OpenRouter está definida
if [ -z "$OPENROUTER_KEY" ]; then
    echo "ERROR: OPENROUTER_API_KEY nao esta definida!"
    exit 1
fi

# Verificar se o token do Telegram está definido
if [ -z "$TELEGRAM_TOKEN" ]; then
    echo "WARNING: TELEGRAM_BOT_TOKEN nao esta definida! O bot nao funcionara."
else
    echo "OK: TELEGRAM_BOT_TOKEN definido"
fi

# Corrigir a connection string do Supabase (mudar porta 6543 para 5432)
if [ -n "$DATABASE_URL" ]; then
    FIXED_DATABASE_URL=$(echo "$DATABASE_URL" | sed 's/:6543\//:5432\//g')
    export DATABASE_URL="$FIXED_DATABASE_URL"
    echo "Database URL corrigida para usar porta 5432"
fi

# Criar config.json dinamicamente
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
      "allow_from": [
        "$TELEGRAM_USER_ID"
      ]
    },
    "discord": {
      "enabled": false,
      "token": "",
      "allow_from": []
    },
    "maixcam": {
      "enabled": false,
      "host": "0.0.0.0",
      "port": 18790,
      "allow_from": []
    },
    "whatsapp": {
      "enabled": false,
      "bridge_url": "ws://localhost:3001",
      "allow_from": []
    },
    "feishu": {
      "enabled": false,
      "app_id": "",
      "app_secret": "",
      "encrypt_key": "",
      "verification_token": "",
      "allow_from": []
    },
    "dingtalk": {
      "enabled": false,
      "client_id": "",
      "client_secret": "",
      "allow_from": []
    },
    "slack": {
      "enabled": false,
      "bot_token": "",
      "app_token": "",
      "allow_from": []
    },
    "line": {
      "enabled": false,
      "channel_secret": "",
      "channel_access_token": "",
      "webhook_host": "0.0.0.0",
      "webhook_port": 18791,
      "webhook_path": "/webhook/line",
      "allow_from": []
    },
    "onebot": {
      "enabled": false,
      "ws_url": "ws://127.0.0.1:3001",
      "access_token": "",
      "reconnect_interval": 5,
      "group_trigger_prefix": [],
      "allow_from": []
    }
  },
  "providers": {
    "anthropic": {
      "api_key": "",
      "api_base": ""
    },
    "openai": {
      "api_key": "",
      "api_base": "",
      "web_search": true
    },
    "openrouter": {
      "api_key": "$OPENROUTER_KEY",
      "api_base": "https://openrouter.ai/api/v1"
    },
    "groq": {
      "api_key": "",
      "api_base": ""
    },
    "zhipu": {
      "api_key": "",
      "api_base": ""
    },
    "gemini": {
      "api_key": "",
      "api_base": ""
    },
    "vllm": {
      "api_key": "",
      "api_base": ""
    },
    "nvidia": {
      "api_key": "",
      "api_base": "",
      "proxy": ""
    },
    "moonshot": {
      "api_key": "",
      "api_base": ""
    },
    "ollama": {
      "api_key": "",
      "api_base": "http://localhost:11434/v1"
    }
  },
  "tools": {
    "web": {
      "brave": {
        "enabled": false,
        "api_key": "",
        "max_results": 5
      },
      "perplexity": {
        "enabled": false,
        "api_key": "",
        "max_results": 5
      }
    },
    "cron": {
      "exec_timeout_minutes": 5
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

echo "Config created successfully!"
echo "- OpenRouter: configurado"
echo "- Telegram: ativado"

# Iniciar o gateway
exec ./picoclaw gateway
