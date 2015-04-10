.PHONY: cmd docker

cmd:
	godep go build -o build/tugboat ./cmd/tugboat

docker:
	docker build --no-cache -t remind101/tugboat .
