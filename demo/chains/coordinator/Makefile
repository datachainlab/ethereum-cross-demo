.PHONY: network
network: 
	docker compose up -d
	make deploy

.PHONY: deploy
deploy:
	npm run deploy-coordinator

.PHONY: network-down
network-down:
	docker compose down -v --remove-orphans
