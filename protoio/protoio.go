package protoio

import (
	"fmt"
	"os"

	pb "github.com/seoyhaein/api-protos/gen/go/datablock/ichthys"
	"google.golang.org/protobuf/proto"
)

func loadMessage(filePath string, message proto.Message) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}
	if err := proto.Unmarshal(data, message); err != nil {
		return fmt.Errorf("failed to unmarshal protobuf message: %w", err)
	}
	return nil
}

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
	db := &pb.DataBlock{}
	if err := loadMessage(filePath, db); err != nil {
		return nil, fmt.Errorf("failed to load pb.DataBlock: %w", err)
	}
	return db, nil
}
