deps:
	#GOOSE_INSTALL=$(HOME)
	curl -fsSL https://raw.githubusercontent.com/pressly/goose/master/install.sh | sh

ifndef CONFIG_FILE
CONFIG_FILE:="config/config.yml"
endif

DEV_HOST=$(shell (cat $(CONFIG_FILE) | yq '.postgres.host'))
DEV_PORT=$(shell (cat $(CONFIG_FILE) | yq '.postgres.port'))
DEV_USER=$(shell (cat $(CONFIG_FILE) | yq '.postgres.user'))
DEV_PASS=$(shell (cat $(CONFIG_FILE) | yq '.postgres.password'))
DEV_DBNAME=$(shell (cat $(CONFIG_FILE) | yq '.postgres.database'))
DEV_SCHEMA=$(shell (cat $(CONFIG_FILE) | yq '.postgres.schema'))


ifndef DB_HOST
	DB_HOST:=$(DEV_HOST)
endif
ifndef DB_PORT
	DB_PORT:=$(DEV_PORT)
endif
ifndef DB_USER
	DB_USER:=$(DEV_USER)
endif
ifndef DB_PASS
	DB_PASS:=$(DEV_PASS)
endif
ifndef DB_DBNAME
	DB_NAME:=$(DEV_DBNAME)
endif
ifndef DB_SCHEMA
	DB_SCHEMA:=$(DEV_SCHEMA)
endif

lint:
	golangci-lint run

run:
	go run ./cmd/app/

migrate:
	goose -dir=migrations postgres "postgresql://$(DB_USER):$(DB_PASS)@$(DB_HOST):$(DB_PORT)/$(DB_NAME)?sslmode=require" status

migrate-status:
	goose -dir=migrations postgres "postgresql://$(DB_USER):$(DB_PASS)@$(DB_HOST):$(DB_PORT)/$(DB_NAME)?sslmode=require" status

migrate-up:
	goose -dir=migrations postgres "postgresql://$(DB_USER):$(DB_PASS)@$(DB_HOST):$(DB_PORT)/$(DB_NAME)?sslmode=require" up

migrate-down:
	goose -dir=migrations postgres "postgresql://$(DB_USER):$(DB_PASS)@$(DB_HOST):$(DB_PORT)/$(DB_NAME)?sslmode=require" down

migrate-new:
	goose -dir=migrations postgres "postgresql://$(DB_USER):$(DB_PASS)@$(DB_HOST):$(DB_PORT)/$(DB_NAME)?sslmode=require" create $(filter-out $@,$(MAKECMDGOALS)) sql


down:
	docker-compose down -v

up:
	docker-compose up -d

init:
	bash init-infra.sh

check:
	bash check-status.sh

smart-start: down up init check

%:
	@: