BIN          := kc
PLATFORM_BIN  = $(BIN)$(if $(findstring windows,$(1)),.exe)
ARCH          = $(shell go env GOARCH)
OS            = $(shell go env GOOS)
ARCH_LIST    ?= 386 amd64
OS_LIST      ?= linux openbsd freebsd netbsd darwin windows
PLATFORMS     = $(foreach os,$(OS_LIST),$(foreach arch,$(ARCH_LIST),$(os)-$(arch)))
BRANCH        = $(shell git rev-parse --abbrev-ref HEAD)
VERSION       = $(shell $(call get-version))
VERSION_FILE := version.go
PREFIX       := /usr/local
BINDIR       := $(PREFIX)/bin
MANDIR       := $(PREFIX)/share/man
MAN_ROFF      = docs/MANUAL.roff
MAN_ADOC      = docs/MANUAL.adoc
LICENSE       = docs/LICENSE.md
CHANGELOG     = docs/CHANGELOG.md
DOCS          = $(MAN_ADOC) $(CHANGELOG) $(LICENSE)
STATICCHECK   = $(shell go env GOPATH)/bin/staticcheck

empty =
space = $(empty) $(empty)
comma = ,

.DEFAULT_GOAL := $(BIN)

$(BIN): | build/bin/$(OS)-$(ARCH)
	@cp -v $| $(call PLATFORM_BIN,$|)

.PHONY: version
version: $(VERSION_FILE)
	@echo $(VERSION)

.PHONY: set-version
set-version: $(VERSION_FILE)
	@{ \
		set -e; \
		old_version=$$($(call get-version)); \
		$(call set-version,$(VERSION)); \
		echo "version: $$old_version -> $(VERSION)"; \
	}

$(VERSION_FILE):
	@{ \
		echo 'package main'; \
		echo; \
		echo 'const Version = "dev"'; \
	} > $(VERSION_FILE)

.PHONY: test
test:
	@go test ./...

.PHONY: check
check: checks := inherit
check: checks += -ST1005 # incorrectly formatted error strings
check: checks := $(subst $(space),$(comma),$(strip $(checks)))
check:
	@go vet ./...
	@$(STATICCHECK) -checks $(checks) ./...

.PHONY: install
install: bin = $(DESTDIR)$(BINDIR)
install: man = $(DESTDIR)$(MANDIR)/man1
install: $(BIN) $(MAN_ROFF)
	@install -dm755 $(bin) $(man)
	@install -v -m755 $(BIN) -t $(bin)
	@install -v -m644 $(MAN_ROFF) $(man)/$(BIN).1

.PHONY: clean
clean:
	@rm -rf \
		$(BIN) \
		$(MAN_ROFF) \
		dist \
		build

$(MAN_ROFF): $(MAN_ADOC)
	@echo man: $@
	@perl -0p \
		-e 's/``(.+?)``/_\1_/msg;' \
		-e 's/`(.+?)`/*\1*/msg;' \
		-e 's/<<(.+?)>>/<<\1,\U\1>>/g;' \
		$(MAN_ADOC) \
	| asciidoctor \
		--backend manpage \
		--doctype manpage \
		--attribute version=$(VERSION) \
		--verbose \
		--out-file $(MAN_ROFF) \
		- # read from stdin

.SECONDEXPANSION:
$(PLATFORMS): | $(addprefix dist/$(BIN)-$$@-$(VERSION),.tar.gz .sha256)

dist: $(PLATFORMS)

dist/$(BIN)-%-$(VERSION).tar.gz: build/bin/% $(DOCS)
	@test -d dist || mkdir dist
	@tar czvf $@ \
		--transform 's,.*/,kc-$(VERSION)/,g' \
		--transform 's,$*,$(call PLATFORM_BIN,$*),' \
		--show-transformed $^ \
		| sed 's,^,archive: $(@F)/,'

%.sha256: %.tar.gz
	@(cd $(@D) && sha256sum $(^F) > $(@F))
	@(cd $(@D) && sha256sum --check --strict $(@F) | sed 's/^/sum: /')

build/bin/%: os = $(firstword $(call split,$(@F)))
build/bin/%: arch = $(lastword $(call split,$(@F)))
build/bin/%: *.go
	@echo compile: $@
	@GOOS=$(os) GOARCH=$(arch) go build -o $@

.PHONY: release
release: version = $$($(call get-version))
release: branch := next
release: check test clean $(BIN) $(VERSION_FILE)
	@git diff-index --quiet $(BRANCH) || { \
		echo 'git: commit all changes before proceeding'; \
		exit 1; \
	}
	@test "$(BRANCH)" = "$(branch)" || { \
		echo 'git (on $(BRANCH)): switch to "$(branch)" before proceeding'; \
		exit 1; \
	}
	@{ \
		set -e; \
		release=$$($(BIN) --release $(VERSION)); \
		$(MAKE) set-version VERSION=$$release; \
	}
	@$(MAKE) dist VERSION=$(version)
	@{ \
		set -e; \
		release=$(version); \
		git add $(VERSION_FILE) $(CHANGELOG); \
		git commit -m "release: $$release"; \
		git tag $$release; \
		git push --force --follow-tags; \
	}
	@{ \
		echo $(version); \
		$(BIN) --show | sed -n '2,$$p'; \
	} | hub release create \
			--file - \
			$$(for asset in dist/*; do echo -n "--attach $$asset "; done) \
			--commitish $(BRANCH) \
			$(version)

split = $(subst -, ,$(1))

define set-version
sed -r -i "s/^(const Version).+/\1 = \"$(1)\"/" $(VERSION_FILE)
endef

define get-version
awk -F '"' '/^const Version/ { print $$2 }' $(VERSION_FILE)
endef
