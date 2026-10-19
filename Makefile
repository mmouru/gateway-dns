NAME ?= gateway-dns

deploy: build
	kind load docker-image ${NAME}:latest --name dev
build: fmt
	docker build . -t ${NAME}
fmt: vet
	go fmt ./...
vet:
	go vet ./...


