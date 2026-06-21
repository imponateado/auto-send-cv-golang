#!/bin/bash

# Configurações
API_URL="http://localhost:8080/api/v1/process"
DEFAULT_DELIMITER=$'\n---\n'

# Ajuda / Uso
usage() {
    echo "Uso: $0 <caminho_do_curriculo.pdf> <caminho_das_vagas.txt> [delimitador]"
    echo ""
    echo "Argumentos:"
    echo "  caminho_do_curriculo.pdf  Caminho para o arquivo de currículo (PDF, DOCX, etc)"
    echo "  caminho_das_vagas.txt     Caminho para o arquivo contendo as vagas"
    echo "  delimitador               Opcional. Delimitador usado para separar as vagas no arquivo de vagas."
    echo "                            Padrão: Quebra de linha seguida de 3 traços e quebra de linha (\n---\n)"
    exit 1
}

# Se o usuário passar menos de 2 argumentos ou pedir help
if [ "$#" -lt 2 ] || [ "$#" -gt 3 ] || [ "$1" == "-h" ] || [ "$1" == "--help" ]; then
    usage
fi

RESUME_PATH="$1"
VACANCIES_PATH="$2"
DELIMITER="${3:-$DEFAULT_DELIMITER}"

# Validações de arquivos
if [ ! -f "$RESUME_PATH" ]; then
    echo "Erro: Arquivo do currículo não encontrado: $RESUME_PATH"
    exit 1
fi

if [ ! -f "$VACANCIES_PATH" ]; then
    echo "Erro: Arquivo das vagas não encontrado: $VACANCIES_PATH"
    exit 1
fi

# Codifica o PDF/Documento para Base64 de forma compatível (Linux / macOS)
echo "-> Codificando currículo em Base64..."
if ! BASE64_RESUME=$(base64 -w 0 "$RESUME_PATH" 2>/dev/null); then
    # Fallback para macOS
    BASE64_RESUME=$(base64 "$RESUME_PATH" | tr -d '\r\n')
fi

# Lê o conteúdo do arquivo de vagas
VACANCIES_CONTENT=$(cat "$VACANCIES_PATH")

echo "-> Montando o payload JSON de forma segura com Python..."
# Gera o JSON de forma segura usando Python 3 para evitar problemas de escape de strings no bash
JSON_PAYLOAD=$(python3 -c '
import sys, json
content = sys.argv[1]
delimiter = sys.argv[2]
file_base64 = sys.argv[3]
payload = {
    "content": content,
    "delimiter": delimiter,
    "file_base64": file_base64
}
print(json.dumps(payload))
' "$VACANCIES_CONTENT" "$DELIMITER" "$BASE64_RESUME")

if [ $? -ne 0 ]; then
    echo "Erro: Falha ao gerar o JSON do payload."
    exit 1
fi

echo "-> Enviando requisição para $API_URL..."
echo ""

# Dispara a requisição usando curl e captura o status HTTP
RESPONSE=$(curl -s -w "\nHTTP_STATUS:%{http_code}" -X POST "$API_URL" \
    -H "Content-Type: application/json" \
    -d "$JSON_PAYLOAD")

# Separa o corpo e o status de retorno
HTTP_STATUS=$(echo "$RESPONSE" | grep "HTTP_STATUS:" | cut -d':' -f2)
BODY=$(echo "$RESPONSE" | grep -v "HTTP_STATUS:")

echo "=== Resposta da API (Status HTTP: $HTTP_STATUS) ==="
# Se o jq estiver instalado, formata a saída bonita, senão exibe crua
if command -v jq >/dev/null 2>&1; then
    echo "$BODY" | jq .
else
    echo "$BODY"
fi
echo "=================================================="
