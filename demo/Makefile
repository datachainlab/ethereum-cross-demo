
.PHONY: build
build:
	mkdir -p ./bin
	make -C ../relayer build && cp ../relayer/build/uly ./bin/
	make -C ../cli build && cp ../cli/build/erc20cli ./bin/

.PHONY: network
network:
	make -C chains/coordinator network
	make -C chains/participant network

.PHONY: network-down
network-down:
	make -C chains/coordinator network-down
	make -C chains/participant network-down
	make clean

.PHONY: clean
clean:
	rm -rf .erc20cli
	rm -rf .urelayer
	rm -rf ./bin
	rm -rf ./data
	rm -rf ./fixtures

.PHONY: demo
demo:
	@./scripts/fixture
	@./scripts/init-cli-config
	@./scripts/init-rly
	@./scripts/handshake
	@./scripts/scenario/atomicswap-success
	@./scripts/scenario/atomicswap-failure
	@echo "Demo completed. Please check the logs above for details."
