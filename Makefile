# Основные переменные
GO := go
GOFLAGS := -v
GOOS ?= $(shell $(GO) env GOOS)
GOARCH ?= $(shell $(GO) env GOARCH)
GOPATH ?= $(shell $(GO) env GOPATH)
GOBIN ?= $(GOPATH)/bin
GOCMD := GOOS=$(GOOS) GOARCH=$(GOARCH) $(GO)

# Пути к директориям
BIN_DIR := ./bin
CMD_DIR := ./cmd
PKG_DIR := ./pkg
TEST_DIR := ./test
SCRIPTS_DIR := ./scripts
CERTS_DIR := ./certs

# Имя приложения и пути к бинарникам
APP_NAME := streaming-server
SERVER_BINARY := $(BIN_DIR)/server
CLIENT_BINARY := $(BIN_DIR)/client

# Версия и сборка
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
COMMIT_HASH ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
PROJECT_NAME ?= $(shell basename $(CURDIR) | tr '[:upper:]' '[:lower:]')

# Docker переменные - автоматическое определение версии
DOCKER_COMPOSE := $(shell which docker-compose 2>/dev/null || echo "docker compose")
DOCKER_COMPOSE_DEV := $(DOCKER_COMPOSE) -f docker-compose.yml -f docker-compose.dev.yml

# Опции тестирования
BAIL ?= false
TEST_FLAGS := $(if $(filter true,$(BAIL)),-failfast,)

# Опции клиента
PROTOCOL ?= http
DEBUG ?= false
INTERACTIVE ?= true
METHOD ?= 
PARAMS ?= 
CLIENT_ARGS := $(if $(filter true,$(INTERACTIVE)),-interactive,) \
               $(if $(filter true,$(DEBUG)),-debug,) \
               $(if $(PROTOCOL),-protocol $(PROTOCOL),) \
               $(if $(METHOD),-method $(METHOD),) \
               $(if $(PARAMS),-params '$(PARAMS)',) \
               $(EXTRA_ARGS)

# Флаги линковщика
LDFLAGS := -ldflags "-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME) -X main.CommitHash=$(COMMIT_HASH)"

# Цели по умолчанию
.PHONY: all help
all: clean setup build test

# По умолчанию показываем справку
.DEFAULT_GOAL := help

# Проверка Docker Compose
.PHONY: check-docker-compose
check-docker-compose:
	@echo "      🐳 Проверка Docker Compose..."
	@if command -v docker-compose >/dev/null 2>&1; then \
		echo "      ✅ Найден docker-compose: $$(docker-compose --version)"; \
		$(eval DOCKER_COMPOSE := docker-compose) \
	elif docker compose version >/dev/null 2>&1; then \
		echo "      ✅ Найден docker compose: $$(docker compose version)"; \
		$(eval DOCKER_COMPOSE := docker compose) \
	else \
		echo "      ❌ Docker Compose не найден!"; \
		echo "      💡 Установите Docker Compose:"; \
		echo "         - Для новых версий Docker: уже включен"; \
		echo "         - Для старых версий: https://docs.docker.com/compose/install/"; \
		exit 1; \
	fi
	@$(eval DOCKER_COMPOSE_DEV := $(DOCKER_COMPOSE) -f docker-compose.yml -f docker-compose.dev.yml)

# Настройка проекта
.PHONY: setup
setup: deps make-scripts-executable install-lint
	@echo "      🔧 Настройка проекта..."
	@mkdir -p $(BIN_DIR)
	@$(SCRIPTS_DIR)/setup-deps.sh

# Сделать скрипты исполняемыми
.PHONY: make-scripts-executable
make-scripts-executable:
	@echo "      🔑 Установка прав на исполнение скриптов..."
	@chmod +x $(SCRIPTS_DIR)/*.sh

# Проверка и автоматическое исправление зависимостей
.PHONY: check-and-fix-deps
check-and-fix-deps:
	@echo "      🔍 Проверка состояния зависимостей..."
	@if [ ! -f go.sum ]; then \
		echo "      ⚠️  go.sum отсутствует, создание..."; \
		$(MAKE) fix-checksums; \
	elif [ go.mod -nt go.sum ]; then \
		echo "      ⚠️  go.mod новее go.sum, обновление..."; \
		$(MAKE) fix-checksums; \
	elif ! $(GO) mod verify >/dev/null 2>&1; then \
		echo "      ⚠️  Проверка модулей не прошла, исправление..."; \
		$(MAKE) fix-checksums; \
	else \
		echo "      ✅ Зависимости в порядке"; \
		$(GO) mod download; \
	fi

# Установка зависимостей
.PHONY: deps
deps: check-and-fix-deps
	@echo "      📦 Зависимости готовы"

# Инициализация зависимостей
.PHONY: init-deps
init-deps: make-scripts-executable
	@echo "      🚀 Инициализация зависимостей..."
	@$(SCRIPTS_DIR)/init-deps.sh

# Проверка зависимостей
.PHONY: check-deps
check-deps: make-scripts-executable
	@echo "      🔍 Проверка зависимостей..."
	@$(SCRIPTS_DIR)/check-deps.sh

# Сборка проекта
.PHONY: build
build: check-and-fix-deps build-server build-client

# Сборка сервера
.PHONY: build-server
build-server:
	@echo "      🏗️ Сборка сервера..."
	@$(GOCMD) build $(GOFLAGS) $(LDFLAGS) -o $(SERVER_BINARY) $(CMD_DIR)/server/main.go

# Сборка клиента
.PHONY: build-client
build-client:
	@echo "      🏗️ Сборка клиента..."
	@$(GOCMD) build $(GOFLAGS) $(LDFLAGS) -o $(CLIENT_BINARY) $(CMD_DIR)/client/main.go

# Запуск сервера
.PHONY: run
run: check-and-fix-deps
	@echo "      🚀 Запуск сервера..."
	@if [ ! -f $(CERTS_DIR)/server.crt ] || [ ! -f $(CERTS_DIR)/server.key ]; then \
		echo "      🔐 Сертификаты не найдены, создание..."; \
		$(MAKE) certs; \
	fi
	@if [ -f $(SERVER_BINARY) ]; then \
		echo "      📦 Используется скомпилированный бинарник"; \
		TLS_CERT_FILE=$(CERTS_DIR)/server.crt TLS_KEY_FILE=$(CERTS_DIR)/server.key $(SERVER_BINARY); \
	else \
		echo "      🔄 Запуск из исходного кода"; \
		TLS_CERT_FILE=$(CERTS_DIR)/server.crt TLS_KEY_FILE=$(CERTS_DIR)/server.key $(GO) run $(CMD_DIR)/server/main.go; \
	fi

# Запуск клиента
.PHONY: run-client
run-client: check-and-fix-deps
	@echo "      🚀 Запуск клиента ($(PROTOCOL))..."
	@if [ -f $(CLIENT_BINARY) ]; then \
		echo "      📦 Используется скомпилированный бинарник"; \
		$(CLIENT_BINARY) $(CLIENT_ARGS); \
	else \
		echo "      🔄 Запуск из исходного кода"; \
		$(GO) run $(CMD_DIR)/client/main.go $(CLIENT_ARGS); \
	fi

# Запуск с air (горячая перезагрузка)
.PHONY: dev
dev:
	@echo "      🔄 Запуск в режиме разработки с горячей перезагрузкой..."
	@air -c air.toml

# Очистка
.PHONY: clean
clean:
	@echo "      🧹 Очистка..."
	@rm -rf $(BIN_DIR)
	@$(GO) clean -cache -testcache

# Генерация самоподписанных сертификатов
.PHONY: certs
certs: make-scripts-executable
	@echo "      🔐 Генерация самоподписанных сертификатов..."
	@$(SCRIPTS_DIR)/generate-certs.sh

# Проверка сертификатов
.PHONY: verify-certs
verify-certs:
	@echo "      🔍 Проверка сертификатов..."
	@if [ -f $(CERTS_DIR)/server.crt ] && [ -f $(CERTS_DIR)/server.key ]; then \
		echo "      📜 Проверка сертификата:"; \
		openssl x509 -in $(CERTS_DIR)/server.crt -text -noout | head -20; \
		echo "      🔑 Проверка приватного ключа:"; \
		openssl rsa -in $(CERTS_DIR)/server.key -check -noout; \
		echo "      🔗 Проверка соответствия ключа и сертификата:"; \
		if [ "$$(openssl x509 -noout -modulus -in $(CERTS_DIR)/server.crt | openssl md5)" = "$$(openssl rsa -noout -modulus -in $(CERTS_DIR)/server.key | openssl md5)" ]; then \
			echo "      ✅ Ключ и сертификат соответствуют друг другу"; \
		else \
			echo "      ❌ Ключ и сертификат НЕ соответствуют друг другу"; \
		fi; \
	else \
		echo "      ❌ Сертификаты не найдены. Запустите 'make certs'"; \
	fi

# Очистка сертификатов
.PHONY: clean-certs
clean-certs:
	@echo "      🧹 Очистка сертификатов..."
	@rm -rf $(CERTS_DIR)
	@echo "      ✅ Сертификаты удалены"

# Интеграционные тесты с сертификатами
.PHONY: test-integration-with-certs
test-integration-with-certs: certs test-integration
	@echo "      🧪 Интеграционные тесты с TLS сертификатами завершены"

# Запуск сервера с TLS
.PHONY: run-with-tls
run-with-tls: certs build-server
	@echo "      🚀 Запуск сервера с TLS сертификатами..."
	@echo "      📁 Используются сертификаты из $(CERTS_DIR)/"
	@TLS_CERT_FILE=$(CERTS_DIR)/server.crt TLS_KEY_FILE=$(CERTS_DIR)/server.key $(SERVER_BINARY)

# Очистка тестовых процессов и портов
.PHONY: clean-test-env
clean-test-env: make-scripts-executable
	@echo "      🧹 Очистка тестового окружения..."
	@$(SCRIPTS_DIR)/cleanup-test-processes.sh

# Тесты
.PHONY: test
test: clean-test-env test-unit test-integration

# Модульные тесты
.PHONY: test-unit
test-unit: check-and-fix-deps
	@echo "      🧪 Запуск модульных тестов..."
	@$(GO) test $(GOFLAGS) $(TEST_FLAGS) -race ./pkg/...

# Проверка тестовых зависимостей
.PHONY: check-test-deps
check-test-deps:
	@echo "      🔍 Проверка тестовых зависимостей..."
	@if ! $(GO) list -test ./test/integration/... >/dev/null 2>&1; then \
		echo "      ⚠️  Тестовые зависимости требуют обновления..."; \
		$(GO) mod tidy; \
		$(GO) get -t ./test/integration/...; \
		$(GO) mod verify; \
	fi

# Интеграционные тесты
.PHONY: test-integration
test-integration: check-and-fix-deps check-test-deps make-scripts-executable clean-test-env
	@echo "      🧪 Запуск интеграционных тестов..."
	@$(SCRIPTS_DIR)/run-integration-tests.sh $(if $(filter true,$(BAIL)),--bail,)

# Интеграционные тесты в тихом режиме
.PHONY: test-integration-quiet
test-integration-quiet: check-and-fix-deps check-test-deps make-scripts-executable clean-test-env
	@echo "      🧪 Запуск интеграционных тестов (тихом режиме)..."
	@$(SCRIPTS_DIR)/run-integration-tests.sh --quiet $(if $(filter true,$(BAIL)),--bail,)

# Безопасный запуск тестов
.PHONY: test-safe
test-safe: check-and-fix-deps make-scripts-executable clean-test-env
	@echo "      🧪 Безопасный запуск тестов..."
	@$(SCRIPTS_DIR)/run-tests-safe.sh $(if $(filter true,$(BAIL)),--bail,)

# Comprehensive tests
.PHONY: test-comprehensive
test-comprehensive: check-and-fix-deps check-test-deps make-scripts-executable certs clean-test-env
	@echo "      🧪 Запуск комплексного набора тестов..."
	@$(SCRIPTS_DIR)/run-comprehensive-tests.sh $(if $(filter true,$(BAIL)),--bail,)

# Установка golangci-lint
.PHONY: install-lint
install-lint:
	@echo "      📥 Установка golangci-lint..."
	@if ! command -v golangci-lint >/dev/null 2>&1; then \
		echo "      🔧 Установка golangci-lint..."; \
		curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(shell go env GOPATH)/bin v1.55.2; \
	else \
		echo "      ✅ golangci-lint уже установлен"; \
	fi

# Линтинг
.PHONY: lint
lint: check-and-fix-deps install-lint
	@echo "      🔍 Запуск линтера..."
	@golangci-lint run ./...

# Форматирование кода
.PHONY: fmt
fmt:
	@echo "      📝 Форматирование кода..."
	@$(GO) fmt ./...

# Исправление контрольных сумм
.PHONY: fix-checksums
fix-checksums:
	@echo "      🔧 Исправление контрольных сумм..."
	@rm -f go.sum
	@$(GO) mod tidy
	@$(GO) get -t ./...
	@$(GO) mod verify
	@echo "      ✅ Контрольные суммы исправлены"

# ==================== DOCKER COMMANDS ====================

# Сборка Docker образов
.PHONY: docker-build
docker-build: check-docker-compose check-and-fix-deps certs
	@echo "      🐳 Сборка Docker образов и запуск сервера с Kafka..."
	@echo "      📋 Создание переменных окружения..."
	@echo "      🔧 Используется: $(DOCKER_COMPOSE)"
	@export VERSION=$(VERSION) BUILD_TIME=$(BUILD_TIME) COMMIT_HASH=$(COMMIT_HASH) PROJECT_NAME=$(PROJECT_NAME) && \
	$(DOCKER_COMPOSE) build --build-arg VERSION=$$VERSION --build-arg BUILD_TIME=$$BUILD_TIME --build-arg COMMIT_HASH=$$COMMIT_HASH
	@echo "      🚀 Запуск сервера и Kafka..."
	@export VERSION=$(VERSION) BUILD_TIME=$(BUILD_TIME) COMMIT_HASH=$(COMMIT_HASH) PROJECT_NAME=$(PROJECT_NAME) && \
	$(DOCKER_COMPOSE) up -d
	@echo "      ✅ Сервер запущен! Доступные порты:"
	@echo "         HTTP:  http://localhost:8080"
	@echo "         HTTPS: https://localhost:8443"
	@echo "         TCP:   localhost:8081"
	@echo "         TLS:   localhost:8444"
	@echo "         WS:    ws://localhost:8082"
	@echo "         WSS:   wss://localhost:8445"
	@echo "      📊 Проверка статуса: make docker-status"

# Разработческая сборка с дополнительными сервисами
.PHONY: docker-build-dev
docker-build-dev: check-docker-compose check-and-fix-deps certs
	@echo "      🐳 Сборка Docker образов для разработки..."
	@export VERSION=$(VERSION) BUILD_TIME=$(BUILD_TIME) COMMIT_HASH=$(COMMIT_HASH) PROJECT_NAME=$(PROJECT_NAME) && \
	$(DOCKER_COMPOSE_DEV) build --build-arg VERSION=$$VERSION --build-arg BUILD_TIME=$$BUILD_TIME --build-arg COMMIT_HASH=$$COMMIT_HASH
	@echo "      🚀 Запуск в режиме разработки..."
	@export VERSION=$(VERSION) BUILD_TIME=$(BUILD_TIME) COMMIT_HASH=$(COMMIT_HASH) PROJECT_NAME=$(PROJECT_NAME) && \
	$(DOCKER_COMPOSE_DEV) up -d
	@echo "      ✅ Сервер запущен в режиме разработки!"
	@echo "         Kafka UI: http://localhost:8090"

# Запуск клиента в Docker
.PHONY: docker-client
docker-client: check-docker-compose
	@echo "      🐳 Запуск клиента в Docker..."
	@export PROJECT_NAME=$(PROJECT_NAME) && \
	$(DOCKER_COMPOSE_DEV) --profile client up streaming-client

# Статус Docker сервисов
.PHONY: docker-status
docker-status: check-docker-compose
	@echo "      📊 Статус Docker сервисов:"
	@export PROJECT_NAME=$(PROJECT_NAME) && \
	$(DOCKER_COMPOSE) ps
	@echo ""
	@echo "      🔍 Логи сервера (последние 10 строк):"
	@export PROJECT_NAME=$(PROJECT_NAME) && \
	$(DOCKER_COMPOSE) logs --tail=10 streaming-server

# Логи Docker сервисов
.PHONY: docker-logs
docker-logs: check-docker-compose
	@echo "      📋 Логи всех сервисов:"
	@export PROJECT_NAME=$(PROJECT_NAME) && \
	$(DOCKER_COMPOSE) logs -f

# Логи только сервера
.PHONY: docker-logs-server
docker-logs-server: check-docker-compose
	@echo "      📋 Логи сервера:"
	@export PROJECT_NAME=$(PROJECT_NAME) && \
	$(DOCKER_COMPOSE) logs -f streaming-server

# Логи Kafka
.PHONY: docker-logs-kafka
docker-logs-kafka: check-docker-compose
	@echo "      📋 Логи Kafka:"
	@export PROJECT_NAME=$(PROJECT_NAME) && \
	$(DOCKER_COMPOSE) logs -f kafka

# Остановка Docker сервисов
.PHONY: docker-stop
docker-stop: check-docker-compose
	@echo "      🛑 Остановка Docker сервисов..."
	@export PROJECT_NAME=$(PROJECT_NAME) && \
	$(DOCKER_COMPOSE) stop
	@echo "      ✅ Сервисы остановлены"

# Перезапуск Docker сервисов
.PHONY: docker-restart
docker-restart: check-docker-compose
	@echo "      🔄 Перезапуск Docker сервисов..."
	@export PROJECT_NAME=$(PROJECT_NAME) && \
	$(DOCKER_COMPOSE) restart
	@echo "      ✅ Сервисы перезапущены"

# Очистка Docker ресурсов
.PHONY: docker-clean
docker-clean: check-docker-compose
	@echo "      🧹 Очистка Docker ресурсов..."
	@export PROJECT_NAME=$(PROJECT_NAME) && \
	$(DOCKER_COMPOSE) down -v --remove-orphans
	@docker system prune -f
	@echo "      ✅ Docker ресурсы очищены"

# Полная очистка включая образы
.PHONY: docker-clean-all
docker-clean-all: check-docker-compose
	@echo "      🧹 Полная очистка Docker ресурсов..."
	@export PROJECT_NAME=$(PROJECT_NAME) && \
	$(DOCKER_COMPOSE) down -v --remove-orphans --rmi all
	@docker system prune -af --volumes
	@echo "      ✅ Все Docker ресурсы очищены"

# Мониторинг ресурсов
.PHONY: docker-stats
docker-stats:
	@echo "      📊 Мониторинг ресурсов Docker:"
	@docker stats

# Проверка здоровья сервисов
.PHONY: docker-health
docker-health: check-docker-compose
	@echo "      🏥 Проверка здоровья сервисов:"
	@export PROJECT_NAME=$(PROJECT_NAME) && \
	$(DOCKER_COMPOSE) ps --format "table {{.Name}}\t{{.Status}}\t{{.Ports}}"
	@echo ""
	@echo "      🔍 Детальная проверка:"
	@curl -s http://localhost:8080/health || echo "❌ HTTP недоступен"
	@echo "✅ HTTP проверка завершена"

# Docker тесты
.PHONY: docker-test
docker-test: check-and-fix-deps make-scripts-executable
	@echo "      🐳 Запуск тестов в Docker..."
	@export PROJECT_NAME=$(PROJECT_NAME) && \
	$(SCRIPTS_DIR)/docker-test.sh

# ==================== DOCUMENTATION COMMANDS ====================

# Генерация документации
.PHONY: docs
docs: check-and-fix-deps
	@echo "      📚 Генерация документации..."
	@mkdir -p docs
	@$(GO) doc -all ./pkg/... > docs/api.md
	@echo "      ✅ Документация сгенерирована в docs/api.md"

# Запуск простого HTTP сервера для документации
.PHONY: docs-serve
docs-serve: docs-html docs-css
	@echo "      🌐 Запуск HTTP сервера для документации..."
	@echo "      🚀 Документация доступна по адресу: http://localhost:8000"
	@echo "      📚 Нажмите Ctrl+C для остановки"
	@cd docs && python3 -m http.server 8000 2>/dev/null || python -m SimpleHTTPServer 8000

# Генерация README для каждого пакета
.PHONY: docs-readme
docs-readme: check-and-fix-deps
	@echo "      📝 Генерация README файлов для пакетов..."
	@for dir in $$(find pkg -type d -mindepth 1); do \
		if [ -n "$$(find $$dir -maxdepth 1 -name '*.go' -not -name '*_test.go')" ]; then \
			echo "      📄 Создание README для $$dir..."; \
			pkg_name=$$(basename $$dir); \
			echo "# Package $$pkg_name" > $$dir/README.md; \
			echo "" >> $$dir/README.md; \
			echo "## Описание" >> $$dir/README.md; \
			echo "" >> $$dir/README.md; \
			go doc -all ./$$dir 2>/dev/null | head -20 >> $$dir/README.md || echo "Документация пакета $$pkg_name" >> $$dir/README.md; \
			echo "" >> $$dir/README.md; \
			echo "## Использование" >> $$dir/README.md; \
			echo "" >> $$dir/README.md; \
			echo '\`\`\`go' >> $$dir/README.md; \
			echo "import \"streaming-server/$$dir\"" >> $$dir/README.md; \
			echo '\`\`\`' >> $$dir/README.md; \
		fi; \
	done
	@echo "      ✅ README файлы созданы для всех пакетов"

# Генерация полной документации
.PHONY: docs-full
docs-full: docs docs-html docs-css docs-readme
	@echo "      📚 Полная документация сгенерирована!"
	@echo "      📁 Файлы документации:"
	@echo "         - docs/api.md (Markdown)"
	@echo "         - docs/api.html (HTML)"
	@echo "         - docs/style.css (CSS стили)"
	@echo "         - pkg/*/README.md (README для каждого пакета)"
	@echo "      🌐 Запустите 'make docs-serve' для просмотра в браузере"

# Очистка документации
.PHONY: docs-clean
docs-clean:
	@echo "      🧹 Очистка документации..."
	@rm -rf docs/
	@find pkg -name "README.md" -delete
	@echo "      ✅ Документация очищена"

# Помощь
.PHONY: help
help:
	@echo "Доступные команды:"
	@echo ""
	@echo "  🏗️  Сборка и запуск:"
	@echo "  make setup              - Настройка проекта"
	@echo "  make build              - Сборка сервера и клиента"
	@echo "  make run                - Запуск сервера (автоматически создает сертификаты)"
	@echo "  make run-client         - Запуск клиента с параметрами по умолчанию"
	@echo "  make run-client PROTOCOL=ws    - Запуск WebSocket клиента"
	@echo "  make run-client PROTOCOL=tcp   - Запуск TCP клиента"
	@echo "  make run-client PROTOCOL=https - Запуск HTTPS клиента"
	@echo "  make run-client DEBUG=true     - Запуск клиента в режиме отладки"
	@echo "  make run-client METHOD=status  - Запуск клиента с методом status"
	@echo "  make run-with-tls       - Запуск сервера с TLS сертификатами"
	@echo "  make dev                - Запуск с горячей перезагрузкой"
	@echo ""
	@echo "  🐳 Docker команды:"
	@echo "  make docker-build       - Сборка и запуск сервера с Kafka (только порты сервера)"
	@echo "  make docker-build-dev   - Сборка для разработки (с Kafka UI)"
	@echo "  make docker-client      - Запуск клиента в Docker"
	@echo "  make docker-status      - Статус Docker сервисов"
	@echo "  make docker-logs        - Логи всех сервисов"
	@echo "  make docker-logs-server - Логи только сервера"
	@echo "  make docker-logs-kafka  - Логи Kafka"
	@echo "  make docker-stop        - Остановка сервисов"
	@echo "  make docker-restart     - Перезапуск сервисов"
	@echo "  make docker-clean       - Очистка Docker ресурсов"
	@echo "  make docker-clean-all   - Полная очистка Docker"
	@echo "  make docker-stats       - Мониторинг ресурсов"
	@echo "  make docker-health      - Проверка здоровья сервисов"
	@echo "  make docker-test        - Запуск тестов в Docker"
	@echo ""
	@echo "  🧪 Тестирование:"
	@echo "  make test               - Запуск всех тестов"
	@echo "  make test-unit          - Запуск модульных тестов"
	@echo "  make test-integration   - Запуск интеграционных тестов"
	@echo "  make test-integration-quiet - Запуск интеграционных тестов в тихом режиме"
	@echo "  make test-integration-with-certs - Интеграционные тесты с TLS сертификатами"
	@echo "  make test-safe          - Безопасный запуск тестов"
	@echo "  make test-comprehensive - Запуск комплексного набора тестов"
	@echo "  make test BAIL=true       - Запуск всех тестов с остановкой при первой ошибке"
	@echo "  make test-unit BAIL=true  - Запуск модульных тестов с остановкой при первой ошибке"
	@echo "  make test-integration BAIL=true - Запуск интеграционных тестов с остановкой при первой ошибке"
	@echo ""
	@echo "  🔐 TLS сертификаты:"
	@echo "  make certs              - Генерация самоподписанных сертификатов"
	@echo "  make verify-certs       - Проверка сертификатов"
	@echo "  make clean-certs        - Удаление сертификатов"
	@echo ""
	@echo "  🧹 Очистка и обслуживание:"
	@echo "  make clean              - Очистка"
	@echo "  make clean-test-env     - Очистка тестового окружения"
	@echo "  make install-lint       - Установка golangci-lint"
	@echo "  make lint               - Запуск линтера"
	@echo "  make fmt                - Форматирование кода"
	@echo "  make fix-checksums      - Исправление контрольных сумм"
	@echo ""
	@echo "  📚 Документация:"
	@echo "  make docs               - Генерация Markdown документации"
	@echo "  make docs-serve         - Запуск HTTP сервера для документации"
	@echo "  make help               - Показать эту справку"
