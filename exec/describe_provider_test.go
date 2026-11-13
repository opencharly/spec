package exec

// describe_provider_test.go — the pb→spec half of the StepContract passthrough: a host-replied
// StepContract must reach a plugin caller with EVERY field the host acted on, so a plugin routing
// on the contract sees the same value the host does. PluginPaths is the field
// opencharly/charly#1131 adds, and dropping it here would make a capability the plugin declared
// invisible to the code that reads the contract.

import (
	"context"
	"testing"

	"google.golang.org/grpc"

	"github.com/opencharly/spec/proto"
	"github.com/opencharly/spec/spec"
)

// describeClient answers ONLY DescribeProvider: the embedded (nil) interface satisfies the rest of
// the generated client surface, and a call this test does not expect panics loudly rather than
// silently returning a zero value.
type describeClient struct {
	proto.ExecutorServiceClient // promoted, nil: any unexpected RPC panics
	reply                       *proto.DescribeProviderReply
}

func (c describeClient) DescribeProvider(context.Context, *proto.DescribeProviderRequest, ...grpc.CallOption) (*proto.DescribeProviderReply, error) {
	return c.reply, nil
}

func TestDescribeProviderCarriesTheStepContractFlags(t *testing.T) {
	reply := &proto.DescribeProviderReply{
		Found: true,
		StepContract: &proto.StepContract{
			Scope:       spec.ScopeSystem.String(),
			Venue:       int32(spec.VenueHostNative),
			Gate:        string(spec.GateNone),
			Emits:       false,
			PluginPaths: true,
		},
	}
	ex := NewInProcExecutor(describeClient{reply: reply})

	found, contract, err := ex.DescribeProvider(context.Background(), "step", "plugin-tree")
	if err != nil {
		t.Fatalf("DescribeProvider: %v", err)
	}
	if !found || contract == nil {
		t.Fatalf("found=%v contract=%v, want the provider's declared contract", found, contract)
	}
	if !contract.PluginPaths {
		t.Fatal("PluginPaths was dropped in the pb→spec conversion: the caller cannot see the flag the host acted on")
	}
	if contract.Scope != spec.ScopeSystem || contract.Venue != spec.VenueHostNative {
		t.Errorf("the contract's other fields drifted: scope=%v venue=%v", contract.Scope, contract.Venue)
	}

	// A provider that declares no contract still reports none — the flag must not manufacture one.
	ex = NewInProcExecutor(describeClient{reply: &proto.DescribeProviderReply{Found: true}})
	if found, contract, err := ex.DescribeProvider(context.Background(), "step", "no-contract"); err != nil || !found || contract != nil {
		t.Fatalf("a provider with no StepContract: found=%v contract=%v err=%v", found, contract, err)
	}
}
