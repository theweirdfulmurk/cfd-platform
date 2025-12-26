.PHONY: help install build run test clean cluster-up cluster-down deploy

help: ## Показать эту справку
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

install: ## Установить зависимости
	@echo "🔧 Установка зависимостей..."
	@./setup-macos.sh

build: ## Собрать бэкенд
	@echo "🔨 Сборка бэкенда..."
	@cd backend && go build -o cfd-platform-backend .

run: ## Запустить локально
	@echo "🚀 Запуск бэкенда..."
	@cd backend && go run main.go

test: ## Запустить тесты
	@echo "🧪 Запуск тестов..."
	@cd backend && go test -v ./...

clean: ## Очистить собранные файлы
	@echo "🧹 Очистка..."
	@rm -f backend/cfd-platform-backend
	@rm -rf data/inputs/*
	@rm -rf data/results/*

cluster-up: ## Создать Kubernetes кластер
	@echo "☸️  Создание кластера..."
	@kind create cluster --name cfd-platform --config k8s/cluster-config.yaml

cluster-down: ## Удалить Kubernetes кластер
	@echo "🗑️  Удаление кластера..."
	@kind delete cluster --name cfd-platform

deploy: ## Развернуть в Kubernetes
	@echo "📦 Развертывание..."
	@kubectl apply -f k8s/deployment.yaml
	@echo "✅ Развернуто в namespace cfd-platform"

status: ## Показать статус
	@echo "📊 Статус системы:"
	@echo "\nКластер:"
	@kubectl cluster-info --context kind-cfd-platform
	@echo "\nПоды:"
	@kubectl get pods -n cfd-platform
	@echo "\nЗадания:"
	@kubectl get jobs -n cfd-platform

logs: ## Показать логи
	@kubectl logs -n cfd-platform -l app=cfd-platform --tail=50 --follow

docker-build: ## Собрать Docker образ
	@echo "🐳 Сборка Docker образа..."
	@cd backend && docker build -t cfd-platform-backend:latest -f ../docker/Dockerfile.backend .

docker-load: docker-build ## Загрузить образ в kind
	@echo "📤 Загрузка образа в kind..."
	@kind load docker-image cfd-platform-backend:latest --name cfd-platform

pull-solvers: ## Загрузить образы солверов
	@echo "⬇️  Загрузка образов солверов..."
	@docker pull openfoam/openfoam10-paraview56
	@docker pull unifem/openfoam-ccx
	@kind load docker-image openfoam/openfoam10-paraview56 --name cfd-platform
	@kind load docker-image unifem/openfoam-ccx --name cfd-platform

dev: ## Запустить в режиме разработки
	@echo "💻 Режим разработки..."
	@cd backend && go run main.go

all: install cluster-up build deploy ## Полная установка и развертывание
	@echo "✅ Система готова к работе!"
	@echo "Откройте http://localhost:8080"
