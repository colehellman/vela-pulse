.PHONY: dev-up dev-down migrate migrate-status gateway-run scraper-run lint test

dev-up:
	docker compose -f infra/compose.yaml up -d

dev-down:
	docker compose -f infra/compose.yaml down -v

migrate:
	$(MAKE) -C gateway migrate

migrate-status:
	$(MAKE) -C gateway migrate-status

gateway-run:
	$(MAKE) -C gateway run

scraper-run:
	$(MAKE) -C scraper run

lint:
	$(MAKE) -C gateway lint
	$(MAKE) -C scraper lint

test:
	$(MAKE) -C gateway test
	$(MAKE) -C scraper test
