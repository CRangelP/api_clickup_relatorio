# ClickUp Dynamic Excel API

API REST para geração de relatórios Excel dinâmicos a partir de dados do ClickUp.

## Características

- **Schema-on-Read**: Configuração dinâmica de campos via payload JSON
- **Paginação Automática**: Busca todas as tarefas independente do tamanho da lista
- **Rate Limiting**: Controle de concorrência para respeitar limites da API do ClickUp
- **Tratamento de Tipos**: Conversão automática de dropdowns, datas, moedas, etc.
- **Timezone**: Datas formatadas em `America/Sao_Paulo`
- **Segurança**: Autenticação via Bearer Token

## Stack

- **Go 1.23+** com Gin Gonic
- **excelize** para geração de Excel
- **Docker** com multistage build

## Configuração

### Variáveis de Ambiente

Copie o arquivo `.env.example` para `.env`:

```bash
cp .env.example .env
```

Configure as variáveis:

| Variável | Descrição | Obrigatório |
|----------|-----------|-------------|
| `TOKEN_CLICKUP` | Token pessoal do ClickUp (pk_...) | ✅ |
| `TOKEN_API` | Token de autenticação da API | ✅ |
| `PORT` | Porta do servidor (default: 8080) | ❌ |
| `GIN_MODE` | Modo do Gin: debug/release | ❌ |

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

### Gerar Relatório

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
  ]
}
```

**Resposta:** Arquivo Excel binário

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

## Limites

- **ClickUp API**: 10.000 requests/minuto
- **Concorrência**: Máximo 5 requests simultâneos
- **Rate Limiter**: 100 requests/minuto (conservador)
- **Timeout**: 30 segundos por request

## Estrutura do Projeto

```
backend/
├── cmd/api/
│   └── main.go              # Entry point
├── internal/
│   ├── config/              # Configurações
│   ├── middleware/          # Middleware de auth
│   ├── handler/             # HTTP handlers
│   ├── service/             # Lógica de negócio
│   ├── client/              # Cliente ClickUp
│   └── model/               # Structs
├── Dockerfile
└── go.mod
```

## Desenvolvimento

```bash
# Instalar dependências
cd backend && go mod tidy

# Rodar testes
go test ./...

# Build
go build -o api ./cmd/api
```

## Licença

MIT
