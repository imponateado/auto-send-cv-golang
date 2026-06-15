# API Go de Processamento de Texto com Delimitador

Uma API REST desenvolvida em Go projetada com **Clean Architecture (Arquitetura Limpa)**. Esta API foi construída para receber um payload JSON contendo um texto grande não formatado e um delimitador, calculando estatísticas como contagem de bytes e total de itens separados por esse delimitador.

## Arquitetura

O projeto segue uma estrutura de camadas limpa:
- **`cmd/server/`**: Ponto de entrada da aplicação. Configura o servidor HTTP com timeouts apropriados e desligamento gracioso (graceful shutdown).
- **`internal/config/`**: Gerenciamento de configurações por variáveis de ambiente.
- **`internal/domain/`**: Definições das entidades de negócio e interfaces (contratos) como `ProcessRequest` e `ProcessResult`.
- **`internal/service/`**: Lógica de negócio (cálculo de bytes, contagem de itens usando o delimitador especificado).
- **`internal/handler/`**: Controladores HTTP (validação de JSON, injeção de dependências), middlewares globais (logging estruturado e recuperação de pânicos).

## Como Executar a Aplicação

### Pré-requisitos
- Go 1.22 ou superior instalado.

### Iniciando o Servidor
Para iniciar a API localmente:
```bash
go run cmd/server/main.go
```
Por padrão, o servidor subirá na porta `8080`. Se desejar alterar a porta, utilize a variável de ambiente `PORT`:
```bash
PORT=9000 go run cmd/server/main.go
```

## Como Testar a API

### Executando Testes Unitários
Para rodar os testes da aplicação:
```bash
go test -v ./...
```

### Efetuando uma Requisição
Envie uma requisição HTTP POST para `/api/v1/process` contendo o JSON com as chaves `content` e `delimiter`.

Exemplo usando `curl` enviando JSON:
```bash
curl -X POST http://localhost:8080/api/v1/process \
  -H "Content-Type: application/json" \
  -d '{
    "content": "2026-06-15 - Leonardo: Olá\n2026-06-15 - Outro: Oi\n2026-06-15 - Leonardo: Como vai?",
    "delimiter": "\n"
  }'
```

Retorno esperado (JSON):
```json
{
  "bytes_processed": 92,
  "items_processed": 3,
  "items": [
    "2026-06-15 - Leonardo: Olá",
    "2026-06-15 - Outro: Oi",
    "2026-06-15 - Leonardo: Como vai?"
  ],
  "duration_ms": 0,
  "status": "success"
}
```
