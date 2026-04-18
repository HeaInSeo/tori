package block

import (
	"fmt"
	"sort"

	pb "github.com/seoyhaein/tori/protos/ichthys/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ConvertMapToFileBlock converts the grouped rules result into a FileBlock.
func ConvertMapToFileBlock(rows map[int]map[string]string, headers []string, blockID string) *pb.FileBlock {
	fb := &pb.FileBlock{
		BlockId:       blockID,
		ColumnHeaders: headers,
		Rows:          make([]*pb.Row, 0, len(rows)),
	}

	indices := make([]int, 0, len(rows))
	for idx := range rows {
		indices = append(indices, idx)
	}
	sort.Ints(indices)

	for _, idx := range indices {
		cols := rows[idx]
		row := &pb.Row{
			RowNumber: int32(idx),
			Cells:     make(map[string]string, len(cols)),
		}
		for colKey, value := range cols {
			row.Cells[colKey] = value
		}
		fb.Rows = append(fb.Rows, row)
	}

	return fb
}

// MergeFileBlocksFromData combines FileBlocks into one DataBlock.
func MergeFileBlocksFromData(inputBlocks []*pb.FileBlock) (*pb.DataBlock, error) {
	if len(inputBlocks) == 0 {
		return nil, fmt.Errorf("no input blocks provided")
	}

	return &pb.DataBlock{
		UpdatedAt: timestamppb.Now(),
		Blocks:    inputBlocks,
	}, nil
}
