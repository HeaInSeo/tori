package block

import (
	"sort"

	pb "github.com/HeaInSeo/tori/protos/ichthys/v1"
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
			RowNumber: int32(idx), //nolint:gosec // idx is a row index bounded by the input file's row count, never near int32 max
			Cells:     make(map[string]string, len(cols)),
		}
		for colKey, value := range cols {
			row.Cells[colKey] = value
		}
		fb.Rows = append(fb.Rows, row)
	}

	return fb
}

// MergeFileBlocksFromData combines FileBlocks into one DataBlock. An empty input
// yields a valid empty DataBlock (Blocks == nil) rather than an error: an empty
// accepted inventory is a representable state, and the acceptance-boundary
// reconcile path (db.regenerateProjectionFromDB) must be able to project it.
// Otherwise removing the last tracked folder would fail projection generation
// after the DB mutation already committed, wedging acceptance in a pending state.
func MergeFileBlocksFromData(inputBlocks []*pb.FileBlock) (*pb.DataBlock, error) {
	return &pb.DataBlock{
		UpdatedAt: timestamppb.Now(),
		Blocks:    inputBlocks,
	}, nil
}
