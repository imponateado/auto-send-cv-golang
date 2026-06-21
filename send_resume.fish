#!/usr/bin/env fish

# Configurações
set API_URL "http://localhost:8080/api/v1/process"
set DEFAULT_DELIMITER "
---
"

# Ajuda / Uso
function usage
    echo "Uso: ./send_resume.fish <caminho_do_curriculo.pdf> <caminho_das_vagas.txt> [delimitador]"
    echo ""
    echo "Argumentos:"
    echo "  caminho_do_curriculo.pdf  Caminho para o arquivo de currículo (PDF, DOCX, etc)"
    echo "  caminho_das_vagas.txt     Caminho para o arquivo contendo as vagas"
    echo "  delimitador               Opcional. Delimitador usado para separar as vagas no arquivo de vagas."
    echo "                            Padrão: Quebra de linha seguida de 3 traços e quebra de linha"
    exit 1
end

# Se o usuário passar menos de 2 argumentos ou pedir help
set argc (count $argv)
if test $argc -lt 2; or test $argc -gt 3; or test "$argv[1]" = "-h"; or test "$argv[1]" = "--help"
    usage
end

set RESUME_PATH $argv[1]
set VACANCIES_PATH $argv[2]
set DELIMITER $DEFAULT_DELIMITER
if test $argc -eq 3
    set DELIMITER $argv[3]
end

# Validações de arquivos
if not test -f "$RESUME_PATH"
    echo "Erro: Arquivo do currículo não encontrado: $RESUME_PATH"
    exit 1
end

if not test -f "$VACANCIES_PATH"
    echo "Erro: Arquivo das vagas não encontrado: $VACANCIES_PATH"
    exit 1
end

# Codifica o PDF/Documento para Base64 de forma compatível
echo "-> Codificando currículo em Base64..."
if base64 -w 0 "$RESUME_PATH" > /dev/null 2>&1
    set BASE64_RESUME (base64 -w 0 "$RESUME_PATH" | string collect)
else
    # Fallback para macOS
    set BASE64_RESUME (base64 "$RESUME_PATH" | tr -d '\r\n' | string collect)
end

# Lê o conteúdo do arquivo de vagas
set VACANCIES_CONTENT (cat "$VACANCIES_PATH" | string collect)

echo "-> Montando o payload JSON de forma segura com Python..."
# Gera o JSON de forma segura usando Python 3 para evitar problemas de escape de strings
set JSON_PAYLOAD (python3 -c '
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
' "$VACANCIES_CONTENT" "$DELIMITER" "$BASE64_RESUME" | string collect)

if test $status -ne 0
    echo "Erro: Falha ao gerar o JSON do payload."
    exit 1
end

echo "-> Enviando requisição para $API_URL..."
echo ""

# Dispara a requisição usando curl e captura o status HTTP
set RESPONSE (curl -s -w "\nHTTP_STATUS:%{http_code}" -X POST "$API_URL" \
    -H "Content-Type: application/json" \
    -d "$JSON_PAYLOAD" | string collect)

# Separa o corpo e o status de retorno
set HTTP_STATUS (echo "$RESPONSE" | grep "HTTP_STATUS:" | cut -d':' -f2)
set BODY (echo "$RESPONSE" | grep -v "HTTP_STATUS:" | string collect)

echo "=== Resposta da API (Status HTTP: $HTTP_STATUS) ==="
# Se o jq estiver instalado, formata a saída bonita, senão exibe crua
if command -v jq >/dev/null 2>&1
    echo "$BODY" | jq .
else
    echo "$BODY"
end
echo "=================================================="
