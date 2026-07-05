#!/usr/bin/env bash
# =============================================================================
# send.sh — Envia requisição para a API de Processamento (Nova Arquitetura Assíncrona)
#
# Uso:
#   bash send.sh <comando> [argumentos]
#
# Comandos:
#   clear                               Limpa o banco de dados vetorial de vagas
#   populate <arquivo_vagas> [delim]    Cadastra e gera embeddings para as vagas (delim padrão: ---)
#   match <arquivo_curriculo>          Compara o currículo (PDF) contra o banco vetorial
#   run <arquivo_vagas> <curriculo>    Executa o fluxo completo (clear + populate + match)
#
# Exemplos:
#   bash send.sh clear
#   bash send.sh populate vagas.txt
#   bash send.sh populate vagas.txt "==="
#   bash send.sh match curriculo.pdf
#   bash send.sh run vagas.txt curriculo.pdf
# =============================================================================

BASE_URL="http://127.0.0.1:8080"

GREEN='\033[0;32m'
CYAN='\033[0;36m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

usage() {
  echo -e "${CYAN}Uso: bash send.sh <comando> [argumentos]${NC}"
  echo ""
  echo "Comandos disponíveis:"
  echo "  clear                               Limpa o banco de dados vetorial de vagas"
  echo "  populate <arquivo_vagas> [delim]    Cadastra e gera embeddings para as vagas (delim padrão: ---)"
  echo "  match <arquivo_curriculo>          Compara o currículo (PDF) contra o banco vetorial"
  echo "  run <arquivo_vagas> <curriculo>    Executa o fluxo completo (clear + populate + match)"
  echo ""
  echo "Exemplos:"
  echo "  bash send.sh clear"
  echo "  bash send.sh populate vagas.txt"
  echo "  bash send.sh populate vagas.txt \"===\""
  echo "  bash send.sh match curriculo.pdf"
  echo "  bash send.sh run vagas.txt curriculo.pdf"
  exit 0
}

if [ -z "$1" ]; then
  usage
fi

COMMAND="$1"
shift

TMP_DIR="./.tmp_send"
mkdir -p "$TMP_DIR"
cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

# Função de polling para acompanhar progresso das tarefas assíncronas do servidor
poll_task() {
  local TASK_ID="$1"
  if [ -z "$TASK_ID" ] || [ "$TASK_ID" = "null" ]; then
    echo -e "${RED}Erro: ID de tarefa inválido recebido.${NC}"
    exit 1
  fi

  echo -e "Tarefa registrada no servidor: ID = ${YELLOW}$TASK_ID${NC}"
  echo -n "Processando vagas em background "

  while true; do
    local STATUS_RESP
    STATUS_RESP=$(curl -s "$BASE_URL/api/v1/tasks/$TASK_ID")
    local STATUS
    STATUS=$(echo "$STATUS_RESP" | jq -r '.status // empty')

    if [ "$STATUS" = "completed" ]; then
      local ITEMS
      ITEMS=$(echo "$STATUS_RESP" | jq -r '.items_processed')
      echo -e "\n${GREEN}✓ Processamento concluído! Novas vagas indexadas: $ITEMS${NC}\n"
      return 0
    elif [ "$STATUS" = "failed" ]; then
      local ERR
      ERR=$(echo "$STATUS_RESP" | jq -r '.error')
      echo -e "\n${RED}✗ Erro no processamento da tarefa: $ERR${NC}\n"
      exit 1
    elif [ "$STATUS" = "processing" ]; then
      echo -n "."
      sleep 2
    else
      echo -e "\n${RED}Status desconhecido: '$STATUS'. Resposta:${NC}"
      echo "$STATUS_RESP"
      exit 1
    fi
  done
}

case "$COMMAND" in
  clear)
    echo -e "\n${CYAN}========================================${NC}"
    echo -e "${CYAN}  Limpando o banco de dados vetorial...${NC}"
    echo -e "${CYAN}========================================${NC}"
    curl -s -X POST "$BASE_URL/api/v1/vacancies/clear" | jq . 2>/dev/null || cat
    echo -e "\n${GREEN}✓ Banco vetorial de vagas limpo com sucesso!${NC}\n"
    ;;

  populate)
    TEXTO_FILE="$1"
    DELIMITER="${2:----}"

    if [ -z "$TEXTO_FILE" ]; then
      echo -e "${RED}Erro: especifique o arquivo de vagas. Ex: bash send.sh populate vagas.txt${NC}"
      exit 1
    fi
    if [ ! -f "$TEXTO_FILE" ]; then
      echo -e "${RED}Erro: arquivo de texto não encontrado: $TEXTO_FILE${NC}"
      exit 1
    fi

    echo -e "\n${CYAN}=================================================${NC}"
    echo -e "${CYAN}  Populando vagas no Banco vetorial local...   ${NC}"
    echo -e "${CYAN}=================================================${NC}"
    echo -e "  Vagas: ${YELLOW}$TEXTO_FILE${NC} (delimitador: \"$DELIMITER\")"
    echo ""

    PAYLOAD_VACANCIES="$TMP_DIR/payload_vacancies.json"
    jq -n \
      --rawfile content "$TEXTO_FILE" \
      --arg delimiter "$DELIMITER" \
      '{content: $content, delimiter: $delimiter}' > "$PAYLOAD_VACANCIES"

    RESPONSE=$(curl -s -X POST "$BASE_URL/api/v1/vacancies" \
      -H "Content-Type: application/json" \
      -d @"$PAYLOAD_VACANCIES")

    TASK_ID=$(echo "$RESPONSE" | jq -r '.task_id // empty')
    if [ -z "$TASK_ID" ] || [ "$TASK_ID" = "null" ]; then
      echo -e "${RED}Erro ao iniciar tarefa de população:${NC}"
      echo "$RESPONSE" | jq . 2>/dev/null || echo "$RESPONSE"
      exit 1
    fi

    poll_task "$TASK_ID"
    ;;

  match)
    PDF_FILE="$1"

    if [ -z "$PDF_FILE" ]; then
      echo -e "${RED}Erro: especifique o arquivo do currículo. Ex: bash send.sh match curriculo.pdf${NC}"
      exit 1
    fi
    if [ ! -f "$PDF_FILE" ]; then
      echo -e "${RED}Erro: arquivo PDF não encontrado: $PDF_FILE${NC}"
      exit 1
    fi

    echo -e "\n${CYAN}========================================${NC}"
    echo -e "${CYAN}  Rodando o Match do Currículo...       ${NC}"
    echo -e "${CYAN}========================================${NC}"
    echo -e "  Currículo: ${YELLOW}$PDF_FILE${NC}"
    echo ""

    base64 -w 0 "$PDF_FILE" > "$TMP_DIR/pdf.b64"
    PAYLOAD_MATCH="$TMP_DIR/payload_match.json"
    jq -n \
      --rawfile file_base64 "$TMP_DIR/pdf.b64" \
      '{file_base64: $file_base64}' > "$PAYLOAD_MATCH"

    curl -s -X POST "$BASE_URL/api/v1/match" \
      -H "Content-Type: application/json" \
      -d @"$PAYLOAD_MATCH" \
      | jq . 2>/dev/null || cat
    echo -e "\n${GREEN}✓ Busca vetorial e triagem concluídas!${NC}\n"
    ;;

  run)
    TEXTO_FILE="$1"
    PDF_FILE="$2"
    DELIMITER="${3:----}"

    if [ -z "$TEXTO_FILE" ] || [ -z "$PDF_FILE" ]; then
      echo -e "${RED}Erro: especifique ambos arquivos. Ex: bash send.sh run vagas.txt curriculo.pdf${NC}"
      exit 1
    fi
    if [ ! -f "$TEXTO_FILE" ]; then
      echo -e "${RED}Erro: arquivo de vagas não encontrado: $TEXTO_FILE${NC}"
      exit 1
    fi
    if [ ! -f "$PDF_FILE" ]; then
      echo -e "${RED}Erro: arquivo do currículo não encontrado: $PDF_FILE${NC}"
      exit 1
    fi

    # 1. Clear database
    echo -e "\n${CYAN}[1/3] Limpando o banco de dados vetorial...${NC}"
    curl -s -X POST "$BASE_URL/api/v1/vacancies/clear" | jq . 2>/dev/null || cat

    # 2. Populate
    echo -e "\n${CYAN}[2/3] Iniciando cadastro e indexação de vagas...${NC}"
    PAYLOAD_VACANCIES="$TMP_DIR/payload_vacancies.json"
    jq -n \
      --rawfile content "$TEXTO_FILE" \
      --arg delimiter "$DELIMITER" \
      '{content: $content, delimiter: $delimiter}' > "$PAYLOAD_VACANCIES"
    
    RESPONSE=$(curl -s -X POST "$BASE_URL/api/v1/vacancies" \
      -H "Content-Type: application/json" \
      -d @"$PAYLOAD_VACANCIES")

    TASK_ID=$(echo "$RESPONSE" | jq -r '.task_id // empty')
    if [ -z "$TASK_ID" ] || [ "$TASK_ID" = "null" ]; then
      echo -e "${RED}Erro ao iniciar tarefa de população:${NC}"
      echo "$RESPONSE" | jq . 2>/dev/null || echo "$RESPONSE"
      exit 1
    fi

    poll_task "$TASK_ID"

    # 3. Match
    echo -e "\n${CYAN}[3/3] Executando o match do currículo...${NC}"
    base64 -w 0 "$PDF_FILE" > "$TMP_DIR/pdf.b64"
    PAYLOAD_MATCH="$TMP_DIR/payload_match.json"
    jq -n \
      --rawfile file_base64 "$TMP_DIR/pdf.b64" \
      '{file_base64: $file_base64}' > "$PAYLOAD_MATCH"
    curl -s -X POST "$BASE_URL/api/v1/match" \
      -H "Content-Type: application/json" \
      -d @"$PAYLOAD_MATCH" \
      | jq . 2>/dev/null || cat

    echo -e "\n${GREEN}✓ Fluxo completo concluído!${NC}\n"
    ;;

  *)
    echo -e "${RED}Comando inválido: $COMMAND${NC}"
    usage
    ;;
esac
