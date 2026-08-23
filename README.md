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

**Vagas e match:**

| Método | Rota | Descrição |
|---|---|---|
| POST | `/api/v1/vacancies/clear` | Limpa o banco vetorial de vagas |
| GET | `/api/v1/vacancies` | Lista todas as vagas atualmente no banco vetorial |
| DELETE | `/api/v1/vacancies/{id}` | Remove uma vaga específica pelo seu ID |
| POST | `/api/v1/match` | Faz o match de um currículo contra as vagas cadastradas |
| GET | `/api/v1/matches` | Lista o histórico de execuções de match (em memória) |
| GET | `/api/v1/matches/{id}` | Consulta um item do histórico de matches |
| DELETE | `/api/v1/matches/{id}` | Remove um item do histórico de matches |

**Credenciais de e-mail:**

| Método | Rota | Descrição |
|---|---|---|
| POST | `/api/v1/credentials` | Registra credenciais OAuth2 (Google/Microsoft) de um candidato para envio de e-mail |
| GET | `/api/v1/credentials` | Lista os candidatos com credenciais cadastradas (sem expor os segredos) |
| GET | `/api/v1/credentials/{email}` | Consulta as credenciais de um candidato (sem expor os segredos) |
| DELETE | `/api/v1/credentials/{email}` | Remove as credenciais de um candidato |

**Sessão do WhatsApp:**

| Método | Rota | Descrição |
|---|---|---|
| GET | `/api/v1/whatsapp/qr?phone=...` | Gera o QR Code para autenticação do WhatsApp (PNG) |
| GET | `/api/v1/whatsapp/status?phone=...` | Consulta o status da sessão do WhatsApp |
| GET | `/api/v1/whatsapp/connections` | Lista todo número que já completou o pareamento |
| POST | `/api/v1/whatsapp/disconnect?phone=...` | Desconecta a sessão do WhatsApp |

**Monitoramento de grupos:**

| Método | Rota | Descrição |
|---|---|---|
| GET | `/api/v1/whatsapp/groups?phone=...` | Lista os grupos de que o número é membro |
| GET | `/api/v1/whatsapp/groups/watched?phone=...` | Lista os grupos atualmente monitorados |
| PUT | `/api/v1/whatsapp/groups/watched?phone=...` | Substitui o conjunto de grupos monitorados |
| DELETE | `/api/v1/whatsapp/groups/watched?phone=...` | Para de monitorar todos os grupos do número |
| GET | `/api/v1/whatsapp/groups/watched-phones` | Lista os números com pelo menos um grupo monitorado |
| POST | `/api/v1/whatsapp/groups/flush` | Processa imediatamente o buffer de mensagens pendentes |
| GET | `/api/v1/whatsapp/groups/status` | Contagem de pendências e resultado do último flush |

**Outros:**

| Método | Rota | Descrição |
|---|---|---|
| GET | `/swagger/` | Documentação interativa (Swagger UI) |

### Fluxo Típico

**1. Conferir as vagas já indexadas no banco vetorial:**
```bash
curl http://localhost:8080/api/v1/vacancies
```
```json
[
  { "index": 0, "text": "Vaga 1: ..." },
  { "index": 1, "text": "Vaga 2: ..." }
]
```

**2. Rodar o match de um currículo** (PDF em Base64) contra as vagas já indexadas:
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
  "matches": [
    {
      "index": 0,
      "vacancy_id": "vac_3f2a...",
      "reason": "Candidato tem experiência em Go e Clean Architecture",
      "contact_type": "email",
      "contact_target": "rh@empresa.com"
    }
  ],
  "duration_ms": 4210,
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

## Frontend

O diretório [`frontend/`](frontend/) contém um cliente web estático — `index.html`, `styles.css` e `app.js`, sem framework, sem build step e sem dependências. Não é embutido no binário do Go; é servido separadamente. Como roda em outra origem, o backend já expõe CORS liberado (`internal/handler/handler.go`) para viabilizar as chamadas.

Cobre os fluxos principais em abas: Vagas (limpar, listar, remover), Match de currículo (upload de PDF), Credenciais de e-mail (OAuth2), WhatsApp (QR code, status, desconexão) e Grupos monitorados (seleção, flush manual, status do buffer).

### Rodando
Abrir o `frontend/index.html` direto no navegador funciona, mas o caminho recomendado é servir por HTTP, para evitar as restrições de `file://`:

```bash
cd frontend
python3 -m http.server 3000
```

Depois acesse `http://localhost:3000`.

O endereço da API é configurável **na própria página**, no campo no topo — não há variável de build. O valor fica salvo no `localStorage` do navegador e o default é `http://localhost:8080`. Para acessar de outro dispositivo na rede local, troque por `http://<ip-da-maquina>:8080`.
