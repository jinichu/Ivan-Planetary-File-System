.PHONY: everything
everything: test build

.PHONY: build
build: proj2 ipfs

.PHONY: test
test: proto
	go test ./...

proj2: proto
	go build -v -o proj2 .

.PHONY: deps
deps:
	go get -u google.golang.org/grpc
	go get -u github.com/gogo/protobuf/protoc-gen-gogoslick
	go get -u github.com/grpc-ecosystem/grpc-gateway/protoc-gen-grpc-gateway
	go get -u github.com/grpc-ecosystem/grpc-gateway/protoc-gen-swagger
	go get -u github.com/golang/protobuf/protoc-gen-go
	go get -u -t ./...

.PHONY: proto
proto:
	protoc -I$(GOPATH)/src -I . -I/usr/local/include -I$(GOPATH)/src/github.com/grpc-ecosystem/grpc-gateway/third_party/googleapis --gogoslick_out=plugins=grpc:. --grpc-gateway_out=logtostderr=true:. serverpb/server.proto

ipfs: app/ipfs.go proj2
	go build -v -o ipfs app/ipfs.go
