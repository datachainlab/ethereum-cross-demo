.PHONY: build
build:
	go build -o build/erc20cli .

.PHONY: protoc
protoc:
	@echo "Generating Protobuf files"
	docker run -v $(CURDIR):/workspace --workdir /workspace tendermintdev/sdk-proto-gen:v0.3 sh ./scripts/protocgen.sh
