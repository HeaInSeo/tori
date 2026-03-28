package grpc

import (
	pb "github.com/seoyhaein/api-protos/gen/go/datablock/ichthys"
	"google.golang.org/protobuf/types/known/emptypb"
)

// newFetchDataBlockResponse keeps gRPC payload assembly inside an adapter-local seam.
func newFetchDataBlockResponse(data *pb.DataBlock) *pb.FetchDataBlockResponse {
	if data == nil {
		return &pb.FetchDataBlockResponse{
			Payload: &pb.FetchDataBlockResponse_NoUpdate{
				NoUpdate: &emptypb.Empty{},
			},
		}
	}

	return &pb.FetchDataBlockResponse{
		Payload: &pb.FetchDataBlockResponse_DataBlock{
			DataBlock: data,
		},
	}
}
