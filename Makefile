# ==== paths ===============================================================

ROOT_DIR := $(abspath $(dir $(lastword $(MAKEFILE_LIST))))
DEMO_DIR := $(ROOT_DIR)/demo
COORD_DIR := $(DEMO_DIR)/chains/coordinator
PART_DIR  := $(DEMO_DIR)/chains/participant

# ==== commands ============================================================

DOCKER_COMPOSE ?= docker-compose
NPM ?= npm

# ==== phony targets =======================================================

.PHONY: up up-coordinator up-participant down down-coordinator down-participant \
        deploy1 deploy2 deploy all logs-coordinator logs-participant slither

# --------------------------------------------------------------------------
# コンテナ起動系
# --------------------------------------------------------------------------

up: up-coordinator up-participant
	@echo "✅ all chains are up"

up-coordinator:
	cd $(COORD_DIR) && $(DOCKER_COMPOSE) up -d

up-participant:
	cd $(PART_DIR) && $(DOCKER_COMPOSE) up -d

down: down-coordinator down-participant
	@echo "🛑 all chains are down"

down-coordinator:
	- cd $(COORD_DIR) && $(DOCKER_COMPOSE) down

down-participant:
	- cd $(PART_DIR) && $(DOCKER_COMPOSE) down

logs-coordinator:
	cd $(COORD_DIR) && $(DOCKER_COMPOSE) logs -f

logs-participant:
	cd $(PART_DIR) && $(DOCKER_COMPOSE) logs -f

# --------------------------------------------------------------------------
# デプロイ系 (npm run deploy1 / deploy2 をラップ)
# --------------------------------------------------------------------------

deploy1:
	cd $(ROOT_DIR) && $(NPM) run deploy1

deploy2:
	cd $(ROOT_DIR) && $(NPM) run deploy2

deploy: deploy1 deploy2
	@echo "✅ both coordinator & participant deployed"

# --------------------------------------------------------------------------
# フルセット (コンテナ起動 + 両チェーンデプロイ)
# --------------------------------------------------------------------------

all: up deploy
	@echo "🎉 env ready (chains up & contracts deployed)"

slither:
	slither .
