.PHONY: clean test

firetap: go.* *.go
	go build -o $@ cmd/firetap/main.go

clean:
	rm -rf firetap dist/

test:
	go test -v ./...

install:
	go install github.com/fujiwara/firetap/cmd/firetap

dist:
	goreleaser build --snapshot --rm-dist
