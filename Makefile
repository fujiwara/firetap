.PHONY: clean test
export CGO_ENABLED=0

firetap: go.* *.go
	go build -o $@ cmd/firetap/main.go

clean:
	rm -rf firetap layer.zip dist/

test:
	go test -v ./...

install:
	go install github.com/fujiwara/firetap/cmd/firetap

dist:
	goreleaser build --snapshot --rm-dist

extensions/firetap: firetap
	mkdir -p extensions
	cp firetap extensions/firetap

layer.zip: extensions/firetap
	zip -r layer.zip extensions/

publish-layer: layer.zip
	aws lambda publish-layer-version \
		--layer-name firetap \
		--zip-file fileb://layer.zip \
		--compatible-runtimes provided.al2023 provided.al2
