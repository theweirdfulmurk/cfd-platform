.PHONY: build run test docker-build kind-load deploy clean

# Go commands
build:
	go build -o bin/server ./cmd/server

run:
	go run ./cmd/server

test:
	go test -v ./...

# Docker commands
docker-build:
	docker build -t cfd-platform-backend:latest .

# Kind commands
kind-create:
	kind create cluster --config k8s/cluster-config.yaml

kind-load:
	kind load docker-image cfd-platform-backend:latest --name cfd-platform

# Kubernetes commands
deploy:
	kubectl apply -f k8s/deployment.yaml

delete:
	kubectl delete -f k8s/deployment.yaml

logs:
	kubectl logs -n cfd-platform -l app=cfd-platform-backend --tail=100 -f

# Development
dev: docker-build kind-load deploy
	@echo "Backend deployed at http://localhost:8082"

# Full setup from scratch
setup: kind-create dev

# Clean
clean:
	rm -rf bin/
	kind delete cluster --name cfd-platform