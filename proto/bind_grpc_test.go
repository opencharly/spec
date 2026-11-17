package proto_test

// bind_grpc_test.go — the METHOD half of the P09 unit 1 proof.
//
// bind_wire_test.go proves the payload survives Marshal/Unmarshal; it never touches
// ProviderClient.Bind, ProviderServer.Bind or _Provider_Bind_Handler. That gap was the review's
// finding: the generated method SURFACE — the thing this change adds — could be deleted and the
// payload test would still pass. So this file calls the method over a real gRPC connection.
//
// In-process over bufconn: no port, no external service, nothing to skip. R8 is satisfied without a
// skip because the service under test is one this test starts.
//
// What it asserts, and why the one-field reply depends on it:
//  1. the call completes and the REPLY carries the binding (not merely a nil error);
//  2. `unbind` returns Empty;
//  3. a failure crosses as the gRPC STATUS — the channel BindReply has no field for, which is the
//     whole reason it mirrors InvokeReply's single-field shape.

import (
	"context"
	"encoding/json"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	pb "github.com/opencharly/spec/proto"
	"github.com/opencharly/spec/spec"
)

// bindServer is a stub type owner: its Bind decodes the payload and echoes the provide into the
// binding, so the round trip is visible in the reply.
type bindServer struct {
	pb.UnimplementedProviderServer
	failWith error
}

func (s *bindServer) Bind(_ context.Context, in *pb.BindRequest) (*pb.BindReply, error) {
	if s.failWith != nil {
		return nil, s.failWith
	}
	var input spec.BindInput
	if err := json.Unmarshal(in.GetInputJson(), &input); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "decode #BindInput: %v", err)
	}
	out, _ := json.Marshal(spec.BindOutput{Binding: spec.Opaque{
		"bound_by":   string(input.Provided_by),
		"deployment": string(input.Deployment),
	}})
	return &pb.BindReply{OutputJson: out}, nil
}

func (s *bindServer) Unbind(_ context.Context, _ *pb.UnbindRequest) (*pb.Empty, error) {
	return &pb.Empty{}, nil
}

// dialBindServer starts the stub on a bufconn listener and returns a client for it.
func dialBindServer(t *testing.T, srv *bindServer) pb.ProviderClient {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	gs := grpc.NewServer()
	pb.RegisterProviderServer(gs, srv)
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(gs.Stop)

	// grpc v1.61 in this module predates grpc.NewClient (added in 1.63), so DialContext + WithBlock
	// is the spelling available; both are the same in-process connection to the bufconn listener.
	conn, err := grpc.DialContext(context.Background(), "bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		t.Fatalf("dial the in-process server: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return pb.NewProviderClient(conn)
}

// TestBindOverARealConnection is the method-surface proof: the call runs, and the REPLY is read.
func TestBindOverARealConnection(t *testing.T) {
	client := dialBindServer(t, &bindServer{})
	payload := mustJSON(t, goodBindInput())

	reply, err := client.Bind(context.Background(), &pb.BindRequest{InputJson: payload})
	if err != nil {
		t.Fatalf("Bind over gRPC: %v", err)
	}
	var out spec.BindOutput
	if err := json.Unmarshal(reply.GetOutputJson(), &out); err != nil {
		t.Fatalf("decode #BindOutput from the reply: %v", err)
	}
	if out.Binding["bound_by"] != "podman" {
		t.Fatalf("the reply's binding = %+v, want the provide the request carried (podman) — the test must read the REPLY, not merely complete the call", out.Binding)
	}
	if out.Binding["deployment"] != "bed" {
		t.Fatalf("the reply's binding did not carry the deployment: %+v", out.Binding)
	}

	empty, err := client.Unbind(context.Background(), &pb.UnbindRequest{InputJson: payload})
	if err != nil {
		t.Fatalf("Unbind over gRPC: %v", err)
	}
	if empty == nil {
		t.Fatal("Unbind returned a nil Empty — the generated method is not wired through the handler")
	}
}

// TestBindFailureCrossesAsTheStatus is the assertion the one-field BindReply depends on: there is
// no error field in the reply, so a failure MUST arrive as the gRPC status. If this ever stops
// being true, BindReply needs the field — and this test is where that is decided.
func TestBindFailureCrossesAsTheStatus(t *testing.T) {
	want := status.Error(codes.FailedPrecondition, "the store is locked")
	client := dialBindServer(t, &bindServer{failWith: want})

	_, err := client.Bind(context.Background(), &pb.BindRequest{InputJson: mustJSON(t, goodBindInput())})
	if err == nil {
		t.Fatal("a failing Bind returned no error — the reply has no error field, so the status IS the channel")
	}
	if got := status.Code(err); got != codes.FailedPrecondition {
		t.Fatalf("the failure crossed with code %v, want FailedPrecondition — the status must survive the handler", got)
	}
}
