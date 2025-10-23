.PHONY: buf-push
buf-push:
	@if [ -z "$(VERSION)" ]; then \
		echo "Error: VERSION is required. Usage: make buf-push VERSION=v0.7.2"; \
		exit 1; \
	fi
	buf push --exclude-unnamed --label=$(VERSION)

.PHONY: buf-generate
buf-generate:
	buf generate

