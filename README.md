# ClickUp Dynamic Excel API

API REST para geração de relatórios Excel dinâmicos a partir de dados do ClickUp.

## Características

- **Schema-on-Read**: Configuração dinâmica de campos via payload JSON
- **Paginação Automática**: Busca todas as tarefas independente do tamanho da lista
- **Rate Limiting**: Controle de concorrência para respeitar limites da API do ClickUp
- **Retry com Backoff**: Retry automático (3x) com espera de 30s em caso de timeout
- **Streaming**: Processamento em streaming para baixo consumo de memória (~300MB para 35k+ tasks) e envio binário direto (sem base64)
- **Webhook Assíncrono**: Processamento em background (até 90 min) com envio do resultado via webhook multipart/form-data
- **Tratamento de Tipos**: Conversão automática de dropdowns, datas, moedas, labels, etc.
- **Timezone**: Datas formatadas em `America/Sao_Paulo`
- **Structured Logging**: Logs em JSON com request tracing via `X-Request-ID`
- **Segurança**: Autenticação via Bearer Token

## Stack

- **Go 1.23+** com Gin Gonic
- **excelize** para geração de Excel
- **zerolog** para structured logging
- **Docker** com multistage build (~15MB)

## Docker Hub

```bash
docker pull crangelp/clickup-excel-api:latest
```

**Tags disponíveis:**
- `latest` - Versão mais recente
- `v1.4.3` - Fix tratamento de campos de data vazios no ClickUp
- `v1.4.2` - Fix bug paginação (last_page), resposta webhook simplificada
- `v1.4.1` - Parâmetro `include_closed` (default: false, apenas tasks abertas)
- `v1.4.0` - Structured logging, request tracing
- `v1.3.0` - Parâmetro opcional `subtasks` (default: apenas main tasks)
- `v1.2.0` - Streaming + baixo consumo de memória
- `v1.0.0` - Versão inicial

## Configuração

### Variáveis de Ambiente

Copie o arquivo `.env.example` para `.env`:

```bash
cp .env.example .env
```

Configure as variáveis:

| Variável | Descrição | Obrigatório | Default |
|----------|-----------|-------------|---------|
| `TOKEN_CLICKUP` | Token pessoal do ClickUp (pk_...) | ✅ | - |
| `TOKEN_API` | Token de autenticação da API | ✅ | - |
| `PORT` | Porta do servidor | ❌ | `8080` |
| `GIN_MODE` | Modo do Gin: debug/release | ❌ | `debug` |
| `LOG_LEVEL` | Nível de log: debug/info/warn/error | ❌ | `info` |
| `LOG_JSON` | Logs em formato JSON (true/false) | ❌ | `true` |
| `TZ` | Timezone para formatação de datas | ❌ | `America/Sao_Paulo` |

### Obtendo o Token do ClickUp

1. Acesse ClickUp > Settings > Apps
2. Clique em "Generate" na seção API Token
3. Copie o token (formato: `pk_xxxxxxxx_...`)

## Execução

### Docker (Recomendado)

```bash
docker-compose up -d
```

### Local

```bash
cd backend
go mod tidy
go run ./cmd/api
```

## API

### Health Check

```http
GET /health
```

**Resposta:**
```json
{"status": "ok"}
```

### Gerar Relatório (Síncrono)

```http
POST /api/v1/reports
Authorization: Bearer {TOKEN_API}
Content-Type: application/json
```

**Payload:**
```json
{
  "list_ids": ["901234567890", "901234567891"],
  "fields": [
    "name",
    "status",
    "assignees",
    "due_date",
    "a1b2c3d4-e5f6-7890-abcd-ef1234567890"
  ],
  "subtasks": false,
  "include_closed": false
}
```

| Parâmetro | Tipo | Obrigatório | Default | Descrição |
|-----------|------|-------------|---------|-----------|
| `list_ids` | array | ✅ | - | IDs das listas do ClickUp |
| `fields` | array | ✅ | - | Campos a incluir no relatório |
| `webhook_url` | string | ❌ | - | URL para envio assíncrono |
| `subtasks` | boolean | ❌ | `false` | Incluir subtasks no relatório |
| `include_closed` | boolean | ❌ | `false` | Incluir tasks finalizadas |

**Resposta:** Arquivo Excel binário

### Gerar Relatório (Assíncrono com Webhook)

```http
POST /api/v1/reports
Authorization: Bearer {TOKEN_API}
Content-Type: application/json
```

**Payload:**
```json
{
  "list_ids": ["901234567890"],
  "fields": ["name", "status", "due_date"],
  "webhook_url": "https://seu-servidor.com/webhook",
  "subtasks": false,
  "include_closed": true
}
```

**Resposta imediata:**
```json
{
  "success": true
}
```

**Payload enviado para o webhook (sucesso) – multipart/form-data:**
- `success`: `"true"`
- `folder_name`: `"Nome da Pasta"`
- `total_tasks`: `"35000"`
- `total_lists`: `"5"`
- `file_mime`: `"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"`
- `file`: arquivo binário `relatorio_2025-12-09_00-15-00.xlsx`

**Payload enviado para o webhook (erro) – JSON:**
```json
{
  "success": false,
  "error": "timeout na requisição para o ClickUp"
}
```

### Campos Nativos Disponíveis

| Campo | Descrição |
|-------|-----------|
| `id` | ID da tarefa |
| `name` | Nome da tarefa |
| `description` | Descrição |
| `status` | Status atual |
| `priority` | Prioridade |
| `assignees` | Responsáveis |
| `tags` | Tags |
| `date_created` | Data de criação |
| `date_updated` | Data de atualização |
| `date_closed` | Data de fechamento |
| `due_date` | Data de vencimento |
| `start_date` | Data de início |
| `list` | Nome da lista |
| `folder` | Nome da pasta |
| `url` | URL da tarefa |

### Campos Personalizados

Use o UUID do campo personalizado do ClickUp. O sistema automaticamente:
- Resolve o nome da coluna a partir dos metadados
- Converte dropdowns para texto
- Formata datas para `dd/MM/yyyy`
- Formata moedas para `R$ X.XX`

## Erros

| Código | Descrição |
|--------|-----------|
| 400 | Payload inválido |
| 401 | Token inválido ou ausente |
| 404 | Lista não encontrada |
| 429 | Rate limit excedido |
| 500 | Erro interno |
| 504 | Timeout na API do ClickUp |

## Limites e Configurações

| Configuração | Valor |
|--------------|-------|
| ClickUp API | 10.000 requests/minuto |
| Rate Limiter | 2.000 requests/minuto (conservador) |
| Timeout por request | 60 segundos |
| Retry por página | 3 tentativas |
| Backoff entre retries | 30 segundos |
| Timeout processamento async | 90 minutos |
| Timeout webhook sucesso | 10 minutos |
| Timeout webhook erro | 5 minutos |

## Consumo de Memória

| Volume | Memória (v1.2.0+) |
|--------|-------------------|
| 10k tasks | ~200MB |
| 35k tasks | ~300MB |
| 100k tasks | ~400MB |

> A partir da v1.2.0, o processamento é feito via streaming, mantendo consumo de memória constante.

## Estrutura do Projeto

```
├── backend/
│   ├── cmd/api/
│   │   └── main.go          # Entry point
│   ├── internal/
│   │   ├── client/          # Cliente HTTP ClickUp
│   │   ├── config/          # Carregamento de configurações
│   │   ├── handler/         # HTTP handlers (Gin)
│   │   ├── logger/          # Structured logging (zerolog)
│   │   ├── middleware/      # Auth e request ID
│   │   ├── model/           # Structs e tipos
│   │   ├── repository/      # Storage temporário (disco)
│   │   └── service/         # Lógica de negócio
│   ├── Dockerfile
│   └── go.mod
├── api/
│   └── insomnia_collection.json
├── docker-compose.yml
├── stack.yaml               # Docker Swarm local
├── stack-prod.yaml          # Docker Swarm produção
└── .env.example
```

## Deploy com Docker Swarm

```yaml
version: "3.8"
services:
  clickup-api:
    image: crangelp/clickup-excel-api:latest
    environment:
      - TOKEN_CLICKUP=${TOKEN_CLICKUP}
      - TOKEN_API=${TOKEN_API}
      - GIN_MODE=release
    deploy:
      resources:
        limits:
          memory: 512M
      labels:
        - traefik.enable=true
        - traefik.http.routers.clickup-api.rule=Host(`api.exemplo.com`)
        - traefik.http.services.clickup-api.loadbalancer.server.port=8080
```

## Desenvolvimento

```bash
# Instalar dependências
cd backend && go mod tidy

# Rodar localmente
go run ./cmd/api

# Build
go build -o api ./cmd/api

# Build Docker
docker build -t clickup-excel-api .
```

## Licença

MIT
