# Path to the protoc compiler
PROTOC_PATH:=$(shell command -v protoc)

# Path to the Protocol Buffers folder
PROTO_PATH=../protos

# Directory to save the generated files
OUT_DIR=$(PROTO_PATH)

# Include path for protobuf well-known types.
PROTO_WELL_KNOWN_INCLUDE:=$(shell find /home/heain/.nuget/packages/grpc.tools /opt/go/pkg/mod -path '*/google/protobuf/timestamp.proto' 2>/dev/null | head -n 1 | awk '{sub("/google/protobuf/timestamp.proto",""); print; exit}')

# Path to the protoc-gen-go plugin
PROTOC_GEN_GO_PATH:=$(shell command -v protoc-gen-go)
PROTOC_GEN_GO_GRPC_PATH:=$(shell command -v protoc-gen-go-grpc)

# Find all .proto files in the PROTO_PATH recursively
PROTO_FILES=$(shell find $(PROTO_PATH) -type f -name '*.proto')

# Default target
all: generate

# Generate Protocol Buffers code
generate: $(PROTO_FILES)
	@for file in $^; do \
		$(PROTOC_PATH) --proto_path=$(PROTO_PATH) --proto_path=$(PROTO_WELL_KNOWN_INCLUDE) --plugin=$(PROTOC_GEN_GO_PATH) --go_out=$(OUT_DIR) --go_opt=paths=source_relative $$file; \
		$(PROTOC_PATH) --proto_path=$(PROTO_PATH) --proto_path=$(PROTO_WELL_KNOWN_INCLUDE) --plugin=$(PROTOC_GEN_GO_GRPC_PATH) --go-grpc_out=$(OUT_DIR) --go-grpc_opt=paths=source_relative $$file; \
	done

# Clean generated files
clean:
	find $(OUT_DIR) -type f \( -name '*.pb.go' -o -name '*_grpc.pb.go' \) -delete

.PHONY: all generate clean
