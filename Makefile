NAME ?= gateway-dns

deploy: build
	kind load docker-image ${NAME}:latest --name dev
build: test
	docker build . -t ${NAME}
test: fmt
	go test ./...
fmt: vet
	gofmt -d .
vet:
	go vet ./...

setup:
	git config core.hooksPath .githooks

