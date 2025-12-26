.PHONY: help install build run clean cluster-up cluster-down deploy

help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

install: 
	@echo "Установка зависимостей..."
	@./setup-macos.sh

build:
	@echo "Сборка бэкенда..."
	@cd backend && go build -o cfd-platform-backend .

run: 
	@echo "Запуск бэкенда..."
	@cd backend && go run main.go

clean: 
	@echo "Очистка..."
	@rm -f backend/cfd-platform-backend
	@rm -rf data/inputs/*
	@rm -rf data/results/*

cluster-up: 
	@echo "Создание кластера..."
	@kind create cluster --name cfd-platform --config k8s/cluster-config.yaml

cluster-down:
	@echo "Удаление кластера..."
	@kind delete cluster --name cfd-platform

deploy: 
	@echo "Развертывание..."
	@kubectl apply -f k8s/deployment.yaml
	@echo "Развернуто в namespace cfd-platform"

status: 
	@echo "Статус системы:"
	@echo "\nКластер:"
	@kubectl cluster-info --context kind-cfd-platform
	@echo "\nПоды:"
	@kubectl get pods -n cfd-platform
	@echo "\nЗадания:"
	@kubectl get jobs -n cfd-platform

logs:
	@kubectl logs -n cfd-platform -l app=cfd-platform --tail=50 --follow

docker-build:
	@echo "Сборка Docker образа..."
	@cd backend && docker build -t cfd-platform-backend:latest -f ../docker/Dockerfile.backend .

docker-load: docker-build 
	@echo "Загрузка образа в kind..."
	@kind load docker-image cfd-platform-backend:latest --name cfd-platform

pull-solvers:
	@echo "Загрузка образов солверов..."
	@docker pull openfoam/openfoam10-paraview56
	@docker pull unifem/openfoam-ccx
	@kind load docker-image openfoam/openfoam10-paraview56 --name cfd-platform
	@kind load docker-image unifem/openfoam-ccx --name cfd-platform

dev: 
	@echo "💻 Режим разработки..."
	@cd backend && go run main.go

all: install cluster-up build deploy
	@echo "Система готова к работе!"
	@echo "Откройте http://localhost:8082"
