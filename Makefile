.PHONY: buf-push
buf-push:
	@if [ -z "$(VERSION)" ]; then \
		echo "Error: VERSION is required. Usage: make buf-push VERSION=v0.7.2"; \
		exit 1; \
	fi
	cd protorelay
	buf generate && buf push --exclude-unnamed --label=main,$(VERSION)
	cd testdata
	buf dep update && buf generate
	cd ../..
