package protoio

import (
	"fmt"
	"os"

	pb "github.com/seoyhaein/api-protos/gen/go/datablock/ichthys"
	"google.golang.org/protobuf/proto"
)

// SaveMessage serializes a protobuf message and writes it to disk.
func SaveMessage(filePath string, message proto.Message, perm os.FileMode) error {
	data, err := proto.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to serialize data: %w", err)
	}
	if err := os.WriteFile(filePath, data, perm); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}
	return nil
}

// LoadDataBlock loads a DataBlock protobuf message from disk.
func LoadDataBlock(filePath string) (*pb.DataBlock, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	db := &pb.DataBlock{}
	if err := proto.Unmarshal(data, db); err != nil {
		return nil, fmt.Errorf("failed to unmarshal pb.DataBlock: %w", err)
	}
	return db, nil
}
