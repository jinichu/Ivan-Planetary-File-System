.PHONY: build
build: test proj2

.PHONY: test
test: proto
	go test ./...

proj2: proto
	go build -v -o proj2 .

.PHONY: deps
deps:
	go get -u ./...
	go get -u google.golang.org/grpc
	go get -u github.com/gogo/protobuf/protoc-gen-gogoslick

.PHONY: proto
proto:
	protoc -I .. -I . --gogoslick_out=plugins=grpc:. serverpb/server.proto

