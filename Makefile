GOBIN = $(shell go env GOPATH)/bin

default: test

test:
	go test -race -shuffle=on -v ./...

$(GOBIN)/staticcheck:
	go install honnef.co/go/tools/cmd/staticcheck@latest

staticcheck: $(GOBIN)/staticcheck
	$(GOBIN)/staticcheck ./...

.PHONY: test staticcheck