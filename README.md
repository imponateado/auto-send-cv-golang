# API Go de Processamento de Texto e Documentos

Uma API REST desenvolvida em Go projetada com **Clean Architecture (Arquitetura Limpa)**. Esta API foi construída para receber um payload JSON contendo um texto grande não formatado e/ou o Base64 de um documento (PDF, DOCX, ODT, etc.), calculando estatísticas textuais e identificando metadados do arquivo decodificado.

## Arquitetura

O projeto segue uma estrutura de camadas limpa:
- **`cmd/server/`**: Ponto de entrada da aplicação. Configura o servidor HTTP com timeouts apropriados e desligamento gracioso (graceful shutdown).
- **`internal/config/`**: Gerenciamento de configurações por variáveis de ambiente. Carrega automaticamente arquivos `.env` locais em ambiente de desenvolvimento sem dependências externas.
- **`internal/domain/`**: Definições das entidades de negócio e interfaces (contratos) como `ProcessRequest`, `ProcessResult` e `FileMetadata`.
- **`internal/service/`**: Lógica de negócio (cálculo de bytes, contagem de itens, decodificação Base64 e detecção automática de MIME type).
- **`internal/handler/`**: Controladores HTTP (validação de JSON, validação condicional dos campos obrigatórios), middlewares globais (logging estruturado e recuperação de pânicos).
- **`internal/infra/`**: Implementações reais de infraestrutura de baixo nível para e-mail (SMTP) e WhatsApp (Meta Cloud API).

## Como Executar a Aplicação

### Pré-requisitos
- Go 1.22 ou superior instalado.

### Configurando Variáveis de Ambiente (.env)
A API suporta carregamento de variáveis através de arquivos `.env`. Copie o template padrão e preencha suas configurações:
```bash
cp .env.example .env
```
O arquivo `.env` gerado já é ignorado pelo Git por segurança.

### Iniciando o Servidor
Para iniciar a API localmente:
```bash
go run cmd/server/main.go
```
Por padrão, o servidor subirá na porta `8080`. Se desejar alterar a porta, utilize a variável de ambiente `PORT` ou altere o valor no arquivo `.env`.

## Como Testar a API

### Executando Testes Unitários
Para rodar os testes da aplicação:
```bash
go test -v ./...
```

### Endpoints Disponíveis

A API expõe os seguintes endpoints sob `/api/v1`:

| Método | Rota | Descrição |
|---|---|---|
| POST | `/api/v1/vacancies/clear` | Limpa o banco vetorial de vagas |
| POST | `/api/v1/vacancies` | Popula o banco vetorial com vagas (assíncrono, retorna `task_id`) |
| GET | `/api/v1/tasks/{id}` | Consulta o status de uma tarefa assíncrona |
| POST | `/api/v1/match` | Faz o match de um currículo contra as vagas cadastradas |
| POST | `/api/v1/credentials` | Registra credenciais OAuth2 (Google/Microsoft) de um candidato para envio de e-mail |
| DELETE | `/api/v1/credentials/{email}` | Remove as credenciais de um candidato |
| GET | `/api/v1/whatsapp/qr?phone=...` | Gera o QR Code para autenticação do WhatsApp (PNG) |
| GET | `/api/v1/whatsapp/status?phone=...` | Consulta o status da sessão do WhatsApp |
| POST | `/api/v1/whatsapp/disconnect?phone=...` | Desconecta a sessão do WhatsApp |
| GET | `/swagger/` | Documentação interativa (Swagger UI) |

### Fluxo Típico

**1. Popular o banco de vagas** (texto bruto separado por delimitador):
```bash
curl -X POST http://localhost:8080/api/v1/vacancies \
  -H "Content-Type: application/json" \
  -d '{
    "content": "Vaga 1: ...\nVaga 2: ...",
    "delimiter": "\n"
  }'
```
Retorno (`202 Accepted`):
```json
{
  "status": "accepted",
  "task_id": "b3f1...",
  "message": "Vacancies processing started in background"
}
```

**2. Acompanhar o processamento em background:**
```bash
curl http://localhost:8080/api/v1/tasks/b3f1...
```
```json
{
  "id": "b3f1...",
  "status": "completed",
  "items_processed": 10
}
```

**3. Rodar o match de um currículo** (PDF em Base64) contra as vagas já indexadas:
```bash
curl -X POST http://localhost:8080/api/v1/match \
  -H "Content-Type: application/json" \
  -d '{
    "file_base64": "JVBERi0xLjQK",
    "candidate_email": "candidato@example.com",
    "candidate_phone": "5511999999999"
  }'
```
Retorno esperado (JSON):
```json
{
  "bytes_processed": 9,
  "items_processed": 1,
  "items": ["..."],
  "file": {
    "size_in_bytes": 9,
    "mime_type": "application/pdf",
    "status": "decoded"
  },
  "matches": [],
  "duration_ms": 0,
  "status": "success"
}
```

### Credenciais de E-mail (OAuth2) e WhatsApp

Antes de disparar e-mails automáticos em nome do candidato, registre as credenciais OAuth2 (`google` ou `microsoft`) obtidas via consentimento do usuário:
```bash
curl -X POST http://localhost:8080/api/v1/credentials \
  -H "Content-Type: application/json" \
  -d '{
    "email": "candidato@example.com",
    "provider": "google",
    "refresh_token": "...",
    "client_id": "...",
    "client_secret": "..."
  }'
```
As credenciais ficam armazenadas em `./db/api.db` (SQLite) e são usadas pelo `internal/infra/email/oauth.go` para renovar o access token na hora do envio.

Para WhatsApp, autentique um número escaneando o QR Code retornado por `GET /api/v1/whatsapp/qr?phone=<numero>` (sessão gerenciada via `whatsmeow`, persistida em `./db/whatsapp.db`).

---

## Integrações de Disparo

O orquenstrador (`internal/service/orchestrator.go`) usa essas integrações para notificar candidatos automaticamente quando um match é confirmado pela LLM:

### 1. Envio de E-mail (OAuth2)
Implementado em [`internal/infra/email/oauth.go`](internal/infra/email/oauth.go) seguindo o contrato `EmailService` ([`internal/domain/email.go`](internal/domain/email.go)).
Autentica via OAuth2 (Google ou Microsoft) usando o refresh token do candidato, armazenado no SQLite através de `POST /api/v1/credentials`.

### 2. Envio de WhatsApp (whatsmeow)
Implementado em [`internal/infra/whatsapp/whatsmeow.go`](internal/infra/whatsapp/whatsmeow.go) seguindo o contrato `WhatsAppService` ([`internal/domain/whatsapp.go`](internal/domain/whatsapp.go)).
Usa a biblioteca [whatsmeow](https://github.com/tulir/whatsmeow) para manter uma sessão real do WhatsApp Web por número, persistida em `./db/whatsapp.db`.

---

## Frontend (Flutter)

O diretório [`frontend/`](frontend/) contém um app Flutter (Web e Android) que consome esta API — é um projeto separado, não embutido no binário do Go. Como roda em origem/app diferente da API, o backend já expõe CORS liberado (`internal/handler/handler.go`) para viabilizar as chamadas.

Cobre os 4 fluxos principais em abas: Vagas (clear/populate + acompanhamento da tarefa assíncrona), Match de currículo (upload de PDF), Credenciais de e-mail (OAuth2) e WhatsApp (QR code, status, desconexão).

### Rodando em desenvolvimento
```bash
cd frontend
flutter run -d chrome --dart-define=API_BASE=http://localhost:8080   # Web
flutter run -d android --dart-define=API_BASE=http://<ip-da-api>:8080  # Android (emulador/device)
```
`API_BASE` aponta para onde a API Go está rodando (default: `http://localhost:8080`). Em um device Android físico, use o IP da máquina na rede local, não `localhost`.

### Gerando os builds finais
```bash
cd frontend
flutter build web                                    # gera frontend/build/web
flutter build apk --dart-define=API_BASE=http://<ip-da-api>:8080   # gera o .apk em frontend/build/app/outputs/flutter-apk
```
