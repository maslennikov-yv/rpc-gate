#!/bin/bash

# Скрипт для генерации самоподписанных сертификатов для тестирования
# Использование: ./scripts/generate-certs.sh [output_dir]

set -e

# Цвета для вывода
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Функции для логирования
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Определяем директорию для сертификатов
CERTS_DIR=${1:-"./certs"}
CERT_FILE="$CERTS_DIR/server.crt"
KEY_FILE="$CERTS_DIR/server.key"

log_info "🔐 Генерация самоподписанных сертификатов для тестирования..."

# Создаем директорию если её нет
mkdir -p "$CERTS_DIR"

# Проверяем, существуют ли уже сертификаты
if [ -f "$CERT_FILE" ] && [ -f "$KEY_FILE" ]; then
    log_warning "Сертификаты уже существуют в $CERTS_DIR/"
    
    # Проверяем срок действия сертификата
    if openssl x509 -checkend 86400 -noout -in "$CERT_FILE" >/dev/null 2>&1; then
        log_info "Существующий сертификат действителен более 24 часов"
        
        # Спрашиваем пользователя, хочет ли он пересоздать сертификаты
        read -p "Пересоздать сертификаты? (y/N): " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            log_info "Используются существующие сертификаты"
            exit 0
        fi
    else
        log_warning "Существующий сертификат истекает в течение 24 часов, пересоздаем..."
    fi
fi

# Проверяем наличие OpenSSL
if ! command -v openssl &> /dev/null; then
    log_error "OpenSSL не найден. Установите OpenSSL для генерации сертификатов."
    exit 1
fi

log_info "📝 Создание приватного ключа..."
openssl genrsa -out "$KEY_FILE" 2048

log_info "📜 Создание сертификата..."

# Создаем конфигурационный файл для сертификата
cat > "$CERTS_DIR/cert.conf" << EOF
[req]
default_bits = 2048
prompt = no
default_md = sha256
distinguished_name = dn
req_extensions = v3_req

[dn]
C=US
ST=Test State
L=Test City
O=Test Organization
OU=Test Unit
CN=localhost

[v3_req]
basicConstraints = CA:FALSE
keyUsage = nonRepudiation, digitalSignature, keyEncipherment
subjectAltName = @alt_names

[alt_names]
DNS.1 = localhost
DNS.2 = *.localhost
DNS.3 = 127.0.0.1
IP.1 = 127.0.0.1
IP.2 = ::1
EOF

# Генерируем сертификат
openssl req -new -x509 -key "$KEY_FILE" -out "$CERT_FILE" -days 365 \
    -config "$CERTS_DIR/cert.conf" -extensions v3_req

# Удаляем временный конфигурационный файл
rm "$CERTS_DIR/cert.conf"

log_success "✅ Сертификаты созданы в $CERTS_DIR/"

# Показываем информацию о сертификате
log_info "📋 Информация о сертификате:"
echo "   Файл сертификата: $CERT_FILE"
echo "   Файл ключа: $KEY_FILE"
echo ""

# Выводим детали сертификата
log_info "📜 Детали сертификата:"
openssl x509 -in "$CERT_FILE" -text -noout | grep -A 1 "Subject:"
openssl x509 -in "$CERT_FILE" -text -noout | grep -A 10 "Subject Alternative Name:" || log_warning "Subject Alternative Name не найден"

# Проверяем срок действия
log_info "⏰ Срок действия:"
openssl x509 -in "$CERT_FILE" -noout -dates

# Проверяем соответствие ключа и сертификата
log_info "🔗 Проверка соответствия ключа и сертификата..."
CERT_MODULUS=$(openssl x509 -noout -modulus -in "$CERT_FILE" | openssl md5)
KEY_MODULUS=$(openssl rsa -noout -modulus -in "$KEY_FILE" | openssl md5)

if [ "$CERT_MODULUS" = "$KEY_MODULUS" ]; then
    log_success "✅ Ключ и сертификат соответствуют друг другу"
else
    log_error "❌ Ключ и сертификат НЕ соответствуют друг другу"
    exit 1
fi

log_info "💡 Использование:"
echo "   Для запуска сервера с TLS: make run-with-tls"
echo "   Для тестирования HTTPS: curl -k https://localhost:8443/rpc"
echo "   Переменные окружения:"
echo "     TLS_CERT_FILE=$CERT_FILE"
echo "     TLS_KEY_FILE=$KEY_FILE"

log_success "🎉 Генерация сертификатов завершена успешно!"
