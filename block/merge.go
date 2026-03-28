package block

import (
	"fmt"

	pb "github.com/seoyhaein/api-protos/gen/go/datablock/ichthys"
	"github.com/seoyhaein/tori/protoio"
)

// GenerateFBs folderFiles 를 받아서 FileBlock 객체를 생성하고, 바이너리 protobuf 파일로 저장
func GenerateFBs(folderFiles [][]string) ([]*pb.FileBlock, error) {
	var fileBlocks []*pb.FileBlock

	for _, ff := range folderFiles {
		if len(ff) == 0 {
			continue
		}
		folderPath := ff[0]

		var fileNames []string
		if len(ff) > 1 {
			fileNames = ff[1:]
		}

		fb, err := GenerateFileBlock(folderPath, fileNames)
		if err != nil {
			return nil, fmt.Errorf("failed to generate file block for folder %s: %w", folderPath, err)
		}

		fileBlocks = append(fileBlocks, fb)
	}
	return fileBlocks, nil
}

// GenerateDataBlock fileblock 을 병합하여 datablcok 으로 저장
// outputFile 은 파일이어야 함. 파일이 존재할 경우는 체크 하지 않고 덮어씀.
func GenerateDataBlock(inputBlocks []*pb.FileBlock, outputFile string) error {
	dataBlock, err := MergeFileBlocksFromData(inputBlocks)
	if err != nil {
		return err
	}

	// DataBlock 저장
	if err := protoio.SaveMessage(outputFile, dataBlock, 0o777); err != nil {
		return fmt.Errorf("failed to save DataBlock: %w", err)
	}

	fmt.Printf("Successfully merged %d FileBlock files into %s\n", len(inputBlocks), outputFile)
	return nil
}
