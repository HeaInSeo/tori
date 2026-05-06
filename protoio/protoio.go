package protoio

import (
	"fmt"
	"os"
	"path/filepath"

	pb "github.com/seoyhaein/tori/protos/ichthys/v1"
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

// SaveMessage serializes a protobuf message and writes it to disk atomically:
// data is written to a temp file in the same directory, then renamed over the target.
func SaveMessage(filePath string, message proto.Message, perm os.FileMode) error {
	data, err := proto.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to serialize data: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(filePath), ".pb-tmp-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}

	ok := false
	defer func() {
		if !ok {
			os.Remove(tmp.Name())
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("failed to write protobuf data: %w", err)
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return fmt.Errorf("failed to set file permissions: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}
	if err := os.Rename(tmp.Name(), filePath); err != nil {
		return fmt.Errorf("failed to rename temp file to %s: %w", filePath, err)
	}
	ok = true
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
