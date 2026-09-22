TLA_TOOLS_VERSION := 1.7.2
TLA_TOOLS_SHA1 := 7f21faa2cdae3189e7d5fadb4488f0dfcc658407
TLA_TOOLS_JAR := .cache/tla2tools-$(TLA_TOOLS_VERSION).jar
TLA_SPECS := Mutation ConfigClone MachineRead ClientWatch NotificationDelivery BackupRestore
TEST_MYSQL_DSN := configra:configra-test@tcp(127.0.0.1:33079)/configra?parseTime=true&charset=utf8mb4&collation=utf8mb4_0900_ai_ci
TEST_MYSQL_ROOT_DSN := root:configra-test-root@tcp(127.0.0.1:33079)/?parseTime=true&charset=utf8mb4&collation=utf8mb4_0900_ai_ci
TEST_NATS_URL := nats://127.0.0.1:42229
TEST_CLICKHOUSE_DSN := clickhouse://configra:configra-test@127.0.0.1:9009/configra?dial_timeout=5s&compress=lz4
TEST_CONTAINER_MYSQL_DSN := configra:configra-test@tcp(mysql:3306)/configra?parseTime=true&charset=utf8mb4&collation=utf8mb4_0900_ai_ci
TEST_DOCKER_NETWORK := configra-test_default
LOCAL_MYSQL_DSN := configra:configra-test@tcp(127.0.0.1:33079)/configra_local?parseTime=true&charset=utf8mb4&collation=utf8mb4_0900_ai_ci
LOCAL_CLICKHOUSE_DSN := clickhouse://configra:configra-test@127.0.0.1:9009/configra_local?dial_timeout=5s&compress=lz4
LOCAL_OIDC_SECRET := configra-local-client-secret-2026
LOCAL_PROJECT ?= configra-local
LOCAL_COMPOSE := docker compose --project-name '$(LOCAL_PROJECT)' -f deploy/compose.test.yaml -f deploy/compose.local.yaml
IMAGE ?= configra:test
VEGETA_VERSION := v12.13.0
VEGETA := .cache/vegeta-12.13.0/vegeta

.PHONY: web-build web-test
.PHONY: kubernetes-test kubernetes-image image-license-test
KUBERNETES_IMAGE ?= configra-kubernetes:test
kubernetes-test:
	GOWORK=off go -C kubernetes test -race ./...
	GOWORK=off go -C kubernetes vet ./...

kubernetes-image:
	docker build -f kubernetes/Dockerfile --tag '$(KUBERNETES_IMAGE)' .

# These filesystem/provenance checks do not start databases or Configra services.
image-license-test: image kubernetes-image
	node scripts/verify-image.mjs '$(IMAGE)' configra
	node scripts/verify-image.mjs '$(KUBERNETES_IMAGE)' configra-kubernetes

web-build:
	npm --prefix web ci --ignore-scripts
	npm --prefix web run build

web-test: web-build
	npm --prefix web run test:licenses
	npm --prefix web test

.PHONY: tla
tla: $(TLA_TOOLS_JAR)
	@set -e; for spec in $(TLA_SPECS); do \
		mkdir -p .cache/tlc/$$spec; \
		java -XX:+UseParallelGC -cp $(TLA_TOOLS_JAR) tlc2.TLC \
			-cleanup -metadir .cache/tlc/$$spec -workers auto \
			-config tla/$$spec.cfg tla/$$spec.tla; \
	done

$(TLA_TOOLS_JAR):
	@mkdir -p .cache
	curl -fsSL https://github.com/tlaplus/tlaplus/releases/download/v$(TLA_TOOLS_VERSION)/tla2tools.jar -o $@
	@printf '%s  %s\n' '$(TLA_TOOLS_SHA1)' '$@' | shasum -a 1 -c -

.PHONY: mysql-test-up dependencies-test-up mysql-test-down test-integration
mysql-test-up:
	docker compose -f deploy/compose.test.yaml up -d --wait mysql

dependencies-test-up:
	docker compose -f deploy/compose.test.yaml up -d --wait mysql nats clickhouse

.PHONY: local-dependencies-up local-runtime local-run local-seed local-screenshots local-stop
local-dependencies-up:
	$(LOCAL_COMPOSE) up -d --wait mysql nats clickhouse
	$(LOCAL_COMPOSE) exec -T mysql \
		mysql -uroot -pconfigra-test-root -e \
		"CREATE DATABASE IF NOT EXISTS configra_local CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci; CREATE DATABASE IF NOT EXISTS casdoor CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci; GRANT ALL PRIVILEGES ON configra_local.* TO 'configra'@'%'; GRANT ALL PRIVILEGES ON casdoor.* TO 'configra'@'%';"
	$(LOCAL_COMPOSE) exec -T clickhouse \
		clickhouse-client --user configra --password configra-test \
		--query 'CREATE DATABASE IF NOT EXISTS configra_local'
	$(LOCAL_COMPOSE) up -d --wait casdoor

local-runtime:
	@mkdir -p .cache/local-dev
	@test -s .cache/local-dev/master-key || openssl rand -base64 32 > .cache/local-dev/master-key
	@test -s .cache/local-dev/client-ca.crt -a -s .cache/local-dev/client-ca.key || \
		openssl req -x509 -newkey rsa:2048 -sha256 -nodes -days 3650 \
		-subj '/CN=Configra Local Client CA' -addext 'basicConstraints=critical,CA:TRUE' \
		-addext 'keyUsage=critical,keyCertSign,cRLSign' \
		-keyout .cache/local-dev/client-ca.key -out .cache/local-dev/client-ca.crt
	@test -s .cache/local-dev/server.crt -a -s .cache/local-dev/server.key || \
		openssl req -x509 -newkey rsa:2048 -sha256 -nodes -days 365 \
		-subj '/CN=localhost' -addext 'subjectAltName=DNS:localhost,IP:127.0.0.1' \
		-keyout .cache/local-dev/server.key -out .cache/local-dev/server.crt
	@test -s .cache/local-dev/client-edge.crt -a -s .cache/local-dev/client-edge.key || \
		(openssl req -new -newkey rsa:2048 -nodes -subj '/CN=edge-reader-01/O=Configra Local' \
			-keyout .cache/local-dev/client-edge.key -out .cache/local-dev/client-edge.csr && \
		 printf '[client]\nbasicConstraints=critical,CA:FALSE\nkeyUsage=critical,digitalSignature,keyEncipherment\nextendedKeyUsage=clientAuth\n' | \
		 openssl x509 -req -sha256 -days 825 -in .cache/local-dev/client-edge.csr \
			-CA .cache/local-dev/client-ca.crt -CAkey .cache/local-dev/client-ca.key -CAcreateserial \
			-extfile /dev/stdin -extensions client -out .cache/local-dev/client-edge.crt)
	@test -s .cache/local-dev/client-revoked.crt -a -s .cache/local-dev/client-revoked.key || \
		(openssl req -new -newkey rsa:2048 -nodes -subj '/CN=retired-reader-02/O=Configra Local' \
			-keyout .cache/local-dev/client-revoked.key -out .cache/local-dev/client-revoked.csr && \
		 printf '[client]\nbasicConstraints=critical,CA:FALSE\nkeyUsage=critical,digitalSignature,keyEncipherment\nextendedKeyUsage=clientAuth\n' | \
		 openssl x509 -req -sha256 -days 825 -in .cache/local-dev/client-revoked.csr \
			-CA .cache/local-dev/client-ca.crt -CAkey .cache/local-dev/client-ca.key \
			-extfile /dev/stdin -extensions client -out .cache/local-dev/client-revoked.crt)
	@chmod 600 .cache/local-dev/master-key .cache/local-dev/server.key .cache/local-dev/client-ca.key .cache/local-dev/client-*.key

local-run: local-dependencies-up local-runtime web-build
	CONFIGRA_MYSQL_DSN='$(LOCAL_MYSQL_DSN)' \
	CONFIGRA_CLICKHOUSE_DSN='$(LOCAL_CLICKHOUSE_DSN)' \
	CONFIGRA_OIDC_CLIENT_SECRET='$(LOCAL_OIDC_SECRET)' \
	go run ./cmd/configra management --config deploy/management.local.yaml

local-seed: local-runtime
	node web/scripts/local-ui.mjs seed

local-screenshots:
	node web/scripts/local-ui.mjs screenshots

local-stop:
	$(LOCAL_COMPOSE) stop casdoor

mysql-test-down:
	docker compose -f deploy/compose.test.yaml down -v

test-integration: dependencies-test-up
	CONFIGRA_TEST_MYSQL_DSN='$(TEST_MYSQL_DSN)' \
	CONFIGRA_TEST_MYSQL_ROOT_DSN='$(TEST_MYSQL_ROOT_DSN)' \
	CONFIGRA_TEST_NATS_URL='$(TEST_NATS_URL)' \
	CONFIGRA_TEST_CLICKHOUSE_DSN='$(TEST_CLICKHOUSE_DSN)' \
	CONFIGRA_TEST_DOCKER_NETWORK='$(TEST_DOCKER_NETWORK)' \
	go test -tags=integration -race -count=1 ./...
	CONFIGRA_TEST_MYSQL_ROOT_DSN='$(TEST_MYSQL_ROOT_DSN)' \
	CONFIGRA_TEST_NATS_URL='$(TEST_NATS_URL)' \
	CONFIGRA_TEST_CLICKHOUSE_DSN='$(TEST_CLICKHOUSE_DSN)' \
	go -C e2e test -tags=integration -race -count=1 ./...

.PHONY: backup-test
backup-test: dependencies-test-up
	CONFIGRA_TEST_MYSQL_ROOT_DSN='$(TEST_MYSQL_ROOT_DSN)' \
	CONFIGRA_TEST_CLICKHOUSE_DSN='$(TEST_CLICKHOUSE_DSN)' \
	CONFIGRA_TEST_DOCKER_NETWORK='$(TEST_DOCKER_NETWORK)' \
	go test -tags=integration -race -count=1 \
		./internal/storage/mysqlstore ./internal/logstore \
		-run 'Test(MySQLBackup|ClickHouseNativeBackup)'

.PHONY: image image-test service-test kubernetes-service-test
image:
	docker build --pull --tag '$(IMAGE)' .

image-test: dependencies-test-up image
	CONFIGRA_TEST_IMAGE='$(IMAGE)' \
	CONFIGRA_TEST_MYSQL_DSN='$(TEST_MYSQL_DSN)' \
	CONFIGRA_TEST_CONTAINER_MYSQL_DSN='$(TEST_CONTAINER_MYSQL_DSN)' \
	CONFIGRA_TEST_DOCKER_NETWORK='$(TEST_DOCKER_NETWORK)' \
	go test -tags=integration -race -count=1 ./internal/app \
		-run TestProductionAPIImageStartsReadyStopsCleanlyAndRejectsWrongMasterKey

# Owns a fresh, uniquely named disposable stack; never reuses a deployment.
service-test: image
	node scripts/service-smoke.mjs '$(IMAGE)'

kubernetes-service-test: image kubernetes-image
	node scripts/service-smoke.mjs '$(IMAGE)' --kubernetes '$(KUBERNETES_IMAGE)'

$(VEGETA):
	@mkdir -p '$(dir $(VEGETA))'
	GOTOOLCHAIN=go1.26.7 GOBIN='$(abspath $(dir $(VEGETA)))' \
		go install github.com/tsenart/vegeta/v12@$(VEGETA_VERSION)

.PHONY: load-smoke load-test
load-smoke: LOAD_DURATION := 15s
load-test: LOAD_DURATION := 10m
load-smoke load-test: dependencies-test-up image $(VEGETA)
	CONFIGRA_LOAD_DURATION='$(LOAD_DURATION)' \
	CONFIGRA_TEST_IMAGE='$(IMAGE)' \
	CONFIGRA_TEST_MYSQL_DSN='$(TEST_MYSQL_DSN)' \
	CONFIGRA_TEST_MYSQL_ROOT_DSN='$(TEST_MYSQL_ROOT_DSN)' \
	CONFIGRA_TEST_CONTAINER_MYSQL_DSN='$(TEST_CONTAINER_MYSQL_DSN)' \
	CONFIGRA_TEST_NATS_URL='$(TEST_NATS_URL)' \
	CONFIGRA_TEST_CLICKHOUSE_DSN='$(TEST_CLICKHOUSE_DSN)' \
	CONFIGRA_TEST_DOCKER_NETWORK='$(TEST_DOCKER_NETWORK)' \
	CONFIGRA_TEST_VEGETA='$(abspath $(VEGETA))' \
	go -C e2e test -tags='integration load' -count=1 -timeout=15m \
		-run TestProductionImageSustainsConfiguredLoadWithoutLeak
