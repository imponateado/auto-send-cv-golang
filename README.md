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

### Efetuando uma Requisição
Envie uma requisição HTTP POST para `/api/v1/process` contendo o JSON com as chaves:
- `content`: Texto para processamento (opcional se enviar `file_base64`).
- `delimiter`: Delimitador para divisão do texto (obrigatório se enviar `content`).
- `file_base64`: String codificada em Base64 do arquivo de documento (opcional se enviar `content`).

Exemplo usando `curl` enviando JSON com texto e arquivo Base64:
```bash
curl -X POST http://localhost:8080/api/v1/process \
  -H "Content-Type: application/json" \
  -d '{
    "content": "Linha 1\nLinha 2",
    "delimiter": "\n",
    "file_base64": "JVBERi0xLjQK"
  }'
```

Retorno esperado (JSON):
```json
{
  "bytes_processed": 15,
  "items_processed": 2,
  "items": [
    "Linha 1",
    "Linha 2"
  ],
  "file": {
    "size_in_bytes": 9,
    "mime_type": "application/pdf",
    "status": "decoded"
  },
  "duration_ms": 0,
  "status": "success"
}
```

---

## Integrações Disponíveis (Para uso futuro)

O projeto inclui contratos de domínio e implementações de infraestrutura isoladas para duas integrações de envio de mensagens:

### 1. Envio de E-mail (SMTP)
Implementado em [smtp.go](file:///home/leonardo/Temp/api/internal/infra/email/smtp.go) seguindo o contrato `EmailService` ([email.go](file:///home/leonardo/Temp/api/internal/domain/email.go)).
Permite disparar e-mails utilizando servidores SMTP padrão (ex: SendGrid, Mailgun, SES, Gmail) através do pacote nativo `net/smtp`.

### 2. Envio de WhatsApp (Meta Cloud API)
Implementado em [client.go](file:///home/leonardo/Temp/api/internal/infra/whatsapp/client.go) seguindo o contrato `WhatsAppService` ([whatsapp.go](file:///home/leonardo/Temp/api/internal/domain/whatsapp.go)).
Permite disparar mensagens HTTP POST diretamente para os endpoints oficiais da WhatsApp Cloud API da Meta.
