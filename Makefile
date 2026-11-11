# Desktop development shell (not the runtime image).
GO_MOD_VOLUME   ?= opendivine-go-mod
GO_BUILD_VOLUME ?= opendivine-go-build
DEV_IMAGE       ?= opendivine-dev

DOCKER_SECURITY_OPTIONS := $(shell docker info --format '{{.SecurityOptions}}' 2>/dev/null)

# Rootless container root maps to the host user.
DOCKER_UID := $(if $(findstring name=rootless,$(DOCKER_SECURITY_OPTIONS)),0,$(shell id -u))
DOCKER_GID := $(if $(findstring name=rootless,$(DOCKER_SECURITY_OPTIONS)),0,$(shell id -g))

DOCKER_USER := \
	--user $(DOCKER_UID):$(DOCKER_GID) \
	--cap-drop ALL \
	--security-opt no-new-privileges

GO_CACHE_MOUNTS := \
	-v "$(GO_MOD_VOLUME):/cache/go-mod" \
	-v "$(GO_BUILD_VOLUME):/cache/go-build"

X11_DISPLAY_NUM := $(shell printf '%s' "$(DISPLAY)" | sed -n 's/^[^:]*:\([0-9][0-9]*\).*/\1/p')
X11_SOCKET      := /tmp/.X11-unix/X$(X11_DISPLAY_NUM)

GPU_ARGS := $(if $(wildcard /dev/dri),--device /dev/dri)

# Avoid $(if): --mount commas would split its arguments.
X11_ARGS :=

ifneq ($(X11_DISPLAY_NUM),)
ifneq ($(wildcard $(X11_SOCKET)),)
X11_ARGS := \
	-e DISPLAY=$(DISPLAY) \
	--mount type=bind,src=$(X11_SOCKET),dst=$(X11_SOCKET),readonly
endif
endif

PULSE_SOCKET ?= $(or $(XDG_RUNTIME_DIR),/run/user/$(shell id -u))/pulse/native
PULSE_COOKIE ?= $(or $(XDG_CONFIG_HOME),$(HOME)/.config)/pulse/cookie

AUDIO_ARGS :=
ifeq ($(shell test -S "$(PULSE_SOCKET)" && printf yes),yes)
AUDIO_ARGS := \
	--mount "type=bind,src=$(PULSE_SOCKET),dst=/run/pulse/native,readonly" \
	-e PULSE_SERVER=unix:/run/pulse/native
ifeq ($(shell test -f "$(PULSE_COOKIE)" && printf yes),yes)
AUDIO_ARGS += \
	--mount "type=bind,src=$(PULSE_COOKIE),dst=/run/pulse/cookie,readonly" \
	-e PULSE_COOKIE=/run/pulse/cookie
endif
endif

GAMEDATA_DIR := $(shell OPENDIVINE_GAMEDATA="$(OPENDIVINE_GAMEDATA)" go run ./cmd/gamedata-probe 2>/dev/null)

GAMEDATA_ARGS :=
ifneq ($(GAMEDATA_DIR),)
GAMEDATA_ARGS := \
	--mount "type=bind,src=$(GAMEDATA_DIR),dst=/gamedata,readonly" \
	-e OPENDIVINE_GAMEDATA=/gamedata \
	-e TEST_GAMEDATA_PATH=/gamedata
endif

.PHONY: dev-image
dev-image:
	docker build --target devcontainer -t $(DEV_IMAGE) .

# Initialize cache ownership without exposing the checkout to root.
.PHONY: shell
shell: dev-image
	@docker run --rm \
		--user 0:0 \
		--cap-drop ALL \
		--cap-add CHOWN \
		--security-opt no-new-privileges \
		$(GO_CACHE_MOUNTS) \
		$(DEV_IMAGE) chown $(DOCKER_UID):$(DOCKER_GID) /cache/go-mod /cache/go-build
	docker run --rm -it \
		$(DOCKER_USER) \
		$(GPU_ARGS) \
		$(X11_ARGS) \
		$(AUDIO_ARGS) \
		$(GAMEDATA_ARGS) \
		--tmpfs /tmp:exec \
		-e HOME=/tmp \
		-e GOPATH=/tmp/go \
		-e GOMODCACHE=/cache/go-mod \
		-e GOCACHE=/cache/go-build \
		-e CGO_ENABLED=1 \
		-v "$(CURDIR):$(CURDIR)" \
		$(GO_CACHE_MOUNTS) \
		-w "$(CURDIR)" \
		$(DEV_IMAGE) bash
