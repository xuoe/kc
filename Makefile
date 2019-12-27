BIN          := kc
PLATFORM_BIN  = $(BIN)$(if $(findstring windows,$(1)),.exe)
ARCH          = $(shell go env GOARCH)
OS            = $(shell go env GOOS)
ARCHLIST     ?= x86 x64
OSLIST       ?= linux openbsd freebsd netbsd macos windows
PLATFORMS    := $(foreach os,$(OSLIST),$(foreach arch,$(ARCHLIST),$(os)-$(arch)))
BRANCH       := $(shell git rev-parse --abbrev-ref HEAD)
COMMIT       := $(shell git rev-parse --short HEAD)
TODAY        := $(shell date '+%Y-%m-%d')
PREFIX       ?= /usr/local
BINDIR       := $(PREFIX)/bin
MANDIR       := $(PREFIX)/share/man
MANROFF      := docs/MANUAL.roff
MANADOC      := docs/MANUAL.adoc
LICENSE      := docs/LICENSE.md
CHANGELOG    := docs/CHANGELOG.md
DOCS         := $(MANADOC) $(CHANGELOG) $(LICENSE)
BASH         := $(shell which bash)
STATICCHECK   = $(shell go env GOPATH)/bin/staticcheck

ifdef CI
VERSION = ci
else
VERSION = $(shell ./$(BIN) --list | head -1)
endif

empty =
space = $(empty) $(empty)
comma = ,

.DEFAULT_GOAL := $(BIN)

.PHONY: version
version: ; @echo $(VERSION)

.PHONY: test
test: ; go test ./...

.PHONY: check
check: checks := inherit
check: checks += -ST1005 # incorrectly formatted error strings
check: checks := $(subst $(space),$(comma),$(strip $(checks)))
check:
	go vet ./...
	$(STATICCHECK) -checks $(checks) ./...

.PHONY: install
install: $(BIN) $(MANROFF)
	install -Dm755 $(BIN) -t $(DESTDIR)$(BINDIR)
	install -Dm644 $(MANROFF) $(DESTDIR)$(MANDIR)/$(BIN).1

$(BIN): build/$(OS)-$(ARCH)
	cp $< $(call PLATFORM_BIN,$<)

$(MANROFF): $(MANADOC)
	perl -0p \
		-e 's/``(.+?)``/_\1_/msg;' \
		-e 's/`(.+?)`/*\1*/msg;' \
		-e 's/<<(.+?)>>/<<\1,\U\1>>/g;' \
		$(MANADOC) \
	| asciidoctor \
		--backend manpage \
		--doctype manpage \
		--attribute version=$(VERSION) \
		--verbose \
		--out-file $(MANROFF) \
		- # read from stdin

.PHONY: release
release: private SHELL := $(BASH)
release: pristine clean check test dist $(BIN)
	@./$(BIN) --release $(VERSION) > /dev/null
	@echo '-----'
	@$(call release-message)
	@echo '-----'
	@$(call confirm,Push $(VERSION)?)
	git checkout -b release/$(VERSION)
	git add $(CHANGELOG)
	git commit -m "release: $(VERSION)"
	git tag --annotate -m "release $(VERSION)" $(VERSION)
	git push --follow-tags origin release/$(VERSION)
	$(call release-message) | hub release create \
		--draft \
		--browse \
		--file - \
		$$(echo dist/* | sed 's,dist/,--attach &,g') \
		--commitish release/$(VERSION) \
		$(VERSION)
	@$(call confirm,Publish $(VERSION)?)
	git checkout next && git merge release/$(VERSION)
	git checkout master && git merge next
	git push --delete origin release/$(VERSION)
	git branch --delete --force release/$(VERSION)
	git push origin :
	hub release edit --draft=false $(VERSION)
	@echo publish $(VERSION): OK


DISTFILES = $(DOCS) *.go
$(DISTFILES): ;

dist: dist.dir $(PLATFORMS)

.PHONY: $(PLATFORMS)
.SECONDEXPANSION:
$(PLATFORMS): | $(addprefix dist/$(BIN)-$(VERSION)-$$@,.tar.gz .sha256)

dist/$(BIN)-$(VERSION)-%.tar.gz: build/% $(DOCS)
	tar czvf $@ \
		--transform 's,.*/,kc-$(VERSION)/,' \
		--transform 's,$*,$(call PLATFORM_BIN,$*),' \
		--show-transformed $^

%.sha256: %.tar.gz
	cd $(@D) && { \
		sha256sum $(^F) > $(@F); \
		sha256sum --check --strict $(@F); \
	}

build/%: SHELL := $(BASH)
build/%: ldflags := -X main.buildDate=$(TODAY)
build/%: ldflags += -X main.buildCommit=$(COMMIT)
build/%: ldflags += -X main.buildVersion=$(VERSION)
build/%: os = $(shell $(call canonic_os,$(firstword $(call split,$(@F)))))
build/%: arch = $(shell $(call canonic_arch,$(lastword $(call split,$(@F)))))
build/%: *.go | build.dir
	GOOS=$(os) GOARCH=$(arch) go build -ldflags "$(ldflags)" -o $@

.PHONY: pristine
pristine:
	@git diff-index --quiet $(BRANCH) || { \
		echo 'git: commit all changes before proceeding'; \
		exit 1; \
	}

.PHONY: clean
clean:
	@rm -rf \
		$(BIN) \
		$(MANROFF) \
		dist \
		build

SUBDIRS = dist.dir build.dir
.PHONY: $(SUBDIRS)
$(SUBDIRS): %.dir: ; @test -d $* || mkdir $*

split = $(subst -, ,$(1))

define canonic_arch
    case $(1) in
        x86) echo 386 ;;
        x64) echo amd64 ;;
    esac
endef

define canonic_os
    case $(1) in
        macos) echo darwin ;;
            *) echo $(1) ;;
    esac
endef

define confirm
read -s -n 1 -p "$(1) [yN] " yn && \
if [[ $${yn,,} != y ]]; then \
	echo 'Exiting by choice...'; \
	exit 1; \
fi
endef

define release-message
{ echo $(VERSION); echo; ./$(BIN) --show $(VERSION) | tail -n +3; }
endef
