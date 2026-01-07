ABIGEN ?= abigen

.PHONY: build
build:
	go build -o build/erc20cli .

.PHONY: protoc
protoc:
	@echo "Generating Protobuf files"
	docker run -v $(CURDIR):/workspace --workdir /workspace tendermintdev/sdk-proto-gen:v0.3 sh ./scripts/protocgen.sh

.PHONY: abi
abi:
ifdef SOURCE
	$(eval TARGET := $(shell echo ${SOURCE} | tr A-Z a-z))
	@mkdir -p ./build/abi ./contract/$(TARGET)
	@forge build >/dev/null
	@test -f ../out/${SOURCE}.sol/${SOURCE}.json || (echo "ABI JSON not found: ../out/${SOURCE}.sol/${SOURCE}.json"; exit 1)
	@jq -c '.abi' ../out/${SOURCE}.sol/${SOURCE}.json > ./build/abi/${SOURCE}.abi
	@$(ABIGEN) --abi ./build/abi/${SOURCE}.abi --pkg $(TARGET) --out ./contract/$(TARGET)/$(TARGET).go
else
	@echo "'SOURCE={ContractName}' is required, e.g. make abi SOURCE=CrossSimpleModule"
	@exit 1
endif
