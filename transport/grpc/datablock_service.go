package grpc

import (
	"context"

	pb "github.com/seoyhaein/api-protos/gen/go/datablock/ichthys"
	appservice "github.com/seoyhaein/tori/service"
)

// DataBlockServer is the gRPC transport adapter for the app service contract.
// It owns protobuf request/response translation only.
type DataBlockServer struct {
	pb.UnimplementedDataBlockServiceServer
	core appservice.DataBlockService
}

func NewDataBlockServer(core appservice.DataBlockService) pb.DataBlockServiceServer {
	return &DataBlockServer{core: core}
}

func (s *DataBlockServer) FetchDataBlock(ctx context.Context, req *pb.FetchDataBlockRequest) (*pb.FetchDataBlockResponse, error) {
	data, err := s.core.GetDataBlock(ctx, req.GetIfModifiedSince())
	if err != nil {
		return nil, err
	}

	return newFetchDataBlockResponse(data), nil
}
