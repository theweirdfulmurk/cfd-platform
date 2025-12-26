.PHONY: install dev build docker-build kind-load deploy clean

install:
	npm install

dev:
	npm run dev

build:
	npm run build

docker-build:
	docker build -t cfd-platform-frontend:latest .

kind-load:
	kind load docker-image cfd-platform-frontend:latest --name cfd-platform

deploy:
	kubectl apply -f k8s/frontend.yaml

delete:
	kubectl delete -f k8s/frontend.yaml

prod: docker-build kind-load deploy

clean:
	rm -rf node_modules dist