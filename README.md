# API Go de Match Automático de Vagas

Uma API REST em Go, com **Clean Architecture**, que monitora grupos de WhatsApp em busca de vagas de emprego, indexa cada mensagem como uma vaga e faz o match automático contra o currículo de um candidato — disparando a candidatura por e-mail ou WhatsApp quando uma LLM confirma a compatibilidade.

## Como funciona

1. Um watcher escuta os grupos de WhatsApp monitorados e acumula as mensagens em memória.
2. Após 10 minutos de silêncio no grupo, o lote inteiro é processado: cada mensagem vira uma vaga, com embedding gerado localmente pelo Ollama. Vaga postada como imagem entra pela legenda e pelo texto do print, transcrito por OCR no Gemini.
3. No `POST /api/v1/match`, o currículo é vetorizado e comparado por similaridade de cosseno contra as vagas do dia.
4. As vagas que passam do threshold vão para uma LLM (Gemini ou DeepSeek), que confirma os matches e extrai o canal de contato.
5. Para cada match confirmado, a candidatura é disparada com o currículo em anexo.

As vagas são descartadas no primeiro flush de cada novo dia.

## Arquitetura

- **`cmd/server/`**: Ponto de entrada. Configura o servidor HTTP com timeouts e desligamento gracioso.
- **`internal/config/`**: Configuração por variáveis de ambiente, com carregamento de `.env` sem dependências externas.
- **`internal/domain/`**: Entidades e contratos — `Vacancy`, `Match`, `Orchestrator`, `VectorStore`, `GroupWatchRepository`.
- **`internal/service/`**: Regra de negócio — o `orchestrator` (indexação e match) e o `GroupWatcher` (buffer e debounce das mensagens de grupo).
- **`internal/handler/`**: Controladores HTTP, middlewares de logging, recuperação de pânico e CORS.
- **`internal/infra/`**: Implementações concretas — SQLite (credenciais, grupos, mensagens e vagas), whatsmeow (sessão do WhatsApp), Gemini/DeepSeek (LLM), Ollama (embeddings), OAuth2 (Gmail e Microsoft Graph) e extração de texto de PDF. O OCR das vagas em imagem reusa o cliente do Gemini, sem dependência nova.

### Armazenamento

Tudo vive em SQLite, em dois arquivos: `./db/api.db` (credenciais, grupos monitorados, arquivo de mensagens e vagas com seus embeddings) e `./db/whatsapp.db` (sessão do whatsmeow).

Os embeddings ficam como `BLOB` e a busca por similaridade é força bruta em Go — varredura linear com cosseno sobre todas as vagas. Para a escala do projeto (centenas de vagas, limpas diariamente) isso custa milissegundos, e dispensa um banco vetorial dedicado.

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
| POST | `/api/v1/vacancies/clear` | Limpa a tabela de vagas |
| GET | `/api/v1/vacancies` | Lista todas as vagas atualmente armazenadas |
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

**1. Conferir as vagas já indexadas:**
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
      "contact_target": "rh@empresa.com",
      "status": "Enfileirado"
    }
  ],
  "duration_ms": 4210,
  "status": "success"
}
```

> **A resposta não confirma envio.** Os disparos vão para uma fila espaçada em 20 a 40 segundos por canal, para não parecer robô — uma rodada com muitos matches leva minutos. O `status` de cada match nasce `"Enfileirado"` e vira `"Sucesso (enviado)"` ou uma mensagem de erro conforme a fila drena. Para acompanhar:
>
> ```bash
> curl http://localhost:8080/api/v1/matches          # pega o id da execução
> curl http://localhost:8080/api/v1/matches/<id>     # status atualizado
> ```
>
> A fila vive em memória: um restart do servidor descarta os envios pendentes.
>
> O arquivo de execução (`log_<timestamp>.txt`, gravado no diretório de onde o binário roda) é o registro que sobrevive: cada match entra como `Enfileirado` e ganha um bloco `ENVIO CONCLUÍDO` com o desfecho quando a fila drena.

> **Para candidatura por e-mail funcionar, o candidato precisa ter credenciais OAuth2 cadastradas** via `POST /api/v1/credentials` (veja a seção abaixo), e o `candidate_email` da requisição precisa bater com o e-mail cadastrado. Sem isso, todo match cujo contato é e-mail aparece como `Não disparado` — e como boa parte das vagas pede currículo por e-mail, isso costuma ser a maioria deles.

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
