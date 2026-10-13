# Linux needs the webkit2gtk-4.1 tag; macOS uses the system WebKit.
TAGS := $(if $(filter Linux,$(shell uname -s)),-tags webkit2_41,)

.PHONY: dev build test run

dev:    ## hot-reload app (also served at http://localhost:34115)
	wails dev $(TAGS)

build:  ## production binary in build/bin
	wails build $(TAGS) -clean

test:
	go test ./internal/...

run: build
	./build/bin/$(if $(filter Darwin,$(shell uname -s)),Maestro.app/Contents/MacOS/Maestro,maestro)
