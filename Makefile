BIN          := kc
PLATFORM_BIN  = $(BIN)$(if $(findstring windows,$(1)),.exe)
ARCH          = $(shell go env GOARCH)
OS            = $(shell go env GOOS)
ARCHLIST     ?= x86 x64
OSLIST       ?= linux openbsd freebsd netbsd macos windows
PLATFORMS    := $(foreach os,$(OSLIST),$(foreach arch,$(ARCHLIST),$(os)-$(arch)))
BRANCH        = $(shell git rev-parse --abbrev-ref HEAD)
COMMIT        = $(shell git rev-parse --short HEAD)
TODAY         = $(shell date '+%Y-%m-%d')
PREFIX       ?= /usr/local
BINDIR       := $(PREFIX)/bin
MANDIR       := $(PREFIX)/share/man
MANROFF      := docs/MANUAL.roff
MANADOC      := docs/MANUAL.adoc
LICENSE      := docs/LICENSE.md
CHANGELOG    := docs/CHANGELOG.md
DOCS         := $(MANADOC) $(CHANGELOG) $(LICENSE)
BASH          = $(shell which bash)
STATICCHECK   = $(shell go env GOPATH)/bin/staticcheck

ifdef CI
VERSION := ci
else
VERSION = $(shell perl -n -e 'if (/\#\#\s\[(.+)\]/) { print $$1; exit }' $(CHANGELOG))
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
install: $(BIN) $(MANROFF).gz
	install -Dm755 $(BIN) -t $(DESTDIR)$(BINDIR)
	install -Dm644 $(MANROFF).gz $(DESTDIR)$(MANDIR)/man1/$(BIN).1.gz

$(BIN): build/$(OS)-$(ARCH)
	cp $< $(call PLATFORM_BIN,$<)

$(MANROFF).gz: $(MANROFF); gzip -c $< > $@

$(MANROFF): $(MANADOC) $(CHANGELOG)
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
release: system-install pristine clean check test dist
	@$(BIN) --release $(VERSION)
	@echo '-----'
	@$(call release-message)
	@echo '-----'
	@$(call confirm,Push $(VERSION)?,\
		git checkout $(CHANGELOG); \
		echo 'release $(VERSION) cancelled')
	git checkout -b release/$(VERSION)
	$(MAKE) $(MANROFF)
	git add $(CHANGELOG) $(MANROFF)
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
	@$(call confirm,Publish $(VERSION)?,\
		git checkout -; \
		git push --delete origin $(VERSION) release/$(VERSION); \
		git branch --delete --force release/$(VERISON); \
		git tag --delete $(VERSION); \
		hub release delete $(VERSION); \
		echo 'release $(VERSION) cancelled; back on $(BRANCH)')
	git checkout next && git merge release/$(VERSION)
	git checkout master && git merge next
	git push --delete origin release/$(VERSION)
	git branch --delete --force release/$(VERSION)
	git push origin :
	hub release edit --draft=false --message "" $(VERSION)
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

build/%: ldflags  = -X main.Version=$(VERSION)
build/%: SHELL   := $(BASH)
build/%: GOOS     = $(shell $(call canonic_os,$(firstword $(call split,$(@F)))))
build/%: GOARCH   = $(shell $(call canonic_arch,$(lastword $(call split,$(@F)))))
build/%: *.go | build.dir; go build -ldflags "$(ldflags)" -o $@

.PHONY: pristine
pristine:
	@git diff-index --quiet $(BRANCH) || { \
		echo 'git: commit all changes before proceeding'; \
		exit 1; \
	}

PHONY: system-install
system-install:
	@command -v $(BIN) || { \
		echo '$(BIN) is not installed on this system. Aborting...'; \
		exit 1; \
	}

.PHONY: clean
clean:
	@rm -rf \
		$(BIN) \
		$(MANROFF).gz \
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
$(call require-bash,confirm); \
read -s -n 1 -p "$(1) [yN] " yn; echo $$yn; \
if [[ $${yn,,} != y ]]; then \
	$(if $(2),$(strip $(2));) \
	echo 'Aborting...'; \
	exit 1; \
fi
endef

define require-bash
test $(SHELL) = $(BASH) || { echo "'$(1)' requires bash"; exit 1; }
endef

define release-message
{ echo $(VERSION); echo; $(BIN) --show $(VERSION) | tail -n +3; }
endef
