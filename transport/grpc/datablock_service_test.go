package grpc

import (
	"context"
	"testing"

	pb "github.com/seoyhaein/tori/protos/ichthys/v1"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type stubDataBlockService struct {
	data *pb.DataBlock
	err  error
}

func (s stubDataBlockService) GetDataBlock(context.Context, *timestamppb.Timestamp) (*pb.DataBlock, error) {
	return s.data, s.err
}

func (s stubDataBlockService) SaveFolders(context.Context) error {
	return nil
}

func (s stubDataBlockService) SyncFolders(context.Context) (bool, error) {
	return false, nil
}

func TestNewFetchDataBlockResponseNilBuildsNoUpdatePayload(t *testing.T) {
	resp := newFetchDataBlockResponse(nil)
	if resp == nil {
		t.Fatalf("expected non-nil response")
	}
	if resp.GetNoUpdate() == nil {
		t.Fatalf("expected no_update payload")
	}
	if resp.GetDataBlock() != nil {
		t.Fatalf("expected data_block payload to be empty")
	}
	if _, ok := resp.Payload.(*pb.FetchDataBlockResponse_NoUpdate); !ok {
		t.Fatalf("expected no_update oneof payload, got %T", resp.Payload)
	}
	if resp.GetNoUpdate().ProtoReflect().Descriptor().FullName() != (&emptypb.Empty{}).ProtoReflect().Descriptor().FullName() {
		t.Fatalf("expected emptypb.Empty payload shape")
	}
}

func TestNewFetchDataBlockResponseDataBuildsDataBlockPayload(t *testing.T) {
	data := &pb.DataBlock{
		UpdatedAt: timestamppb.Now(),
		Blocks: []*pb.FileBlock{
			{BlockId: "block-1"},
		},
	}

	resp := newFetchDataBlockResponse(data)
	if resp == nil {
		t.Fatalf("expected non-nil response")
	}
	if resp.GetDataBlock() == nil {
		t.Fatalf("expected data_block payload")
	}
	if resp.GetNoUpdate() != nil {
		t.Fatalf("expected no_update payload to be empty")
	}
	if _, ok := resp.Payload.(*pb.FetchDataBlockResponse_DataBlock); !ok {
		t.Fatalf("expected data_block oneof payload, got %T", resp.Payload)
	}
	if resp.GetDataBlock() != data {
		t.Fatalf("expected same data pointer to be forwarded through seam")
	}
	if resp.GetDataBlock().GetBlocks()[0].GetBlockId() != "block-1" {
		t.Fatalf("unexpected block id: %q", resp.GetDataBlock().GetBlocks()[0].GetBlockId())
	}
}

func TestFetchDataBlockReturnsNoUpdateWhenServiceReturnsNil(t *testing.T) {
	server := NewDataBlockServer(stubDataBlockService{})

	resp, err := server.FetchDataBlock(context.Background(), &pb.FetchDataBlockRequest{})
	if err != nil {
		t.Fatalf("FetchDataBlock error: %v", err)
	}
	if resp.GetNoUpdate() == nil {
		t.Fatalf("expected no_update payload")
	}
	if resp.GetDataBlock() != nil {
		t.Fatalf("expected data_block payload to be empty")
	}
}

func TestFetchDataBlockReturnsDataBlockPayload(t *testing.T) {
	data := &pb.DataBlock{
		UpdatedAt: timestamppb.Now(),
		Blocks: []*pb.FileBlock{
			{BlockId: "block-1"},
		},
	}
	server := NewDataBlockServer(stubDataBlockService{data: data})

	resp, err := server.FetchDataBlock(context.Background(), &pb.FetchDataBlockRequest{})
	if err != nil {
		t.Fatalf("FetchDataBlock error: %v", err)
	}
	if resp.GetDataBlock() == nil {
		t.Fatalf("expected data_block payload")
	}
	if resp.GetDataBlock().GetBlocks()[0].GetBlockId() != "block-1" {
		t.Fatalf("unexpected block id: %q", resp.GetDataBlock().GetBlocks()[0].GetBlockId())
	}
	if resp.GetNoUpdate() != nil {
		t.Fatalf("expected no_update payload to be empty")
	}
}
