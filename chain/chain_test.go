package chain_test

import (
	"context"
	"log/slog"
	"testing"

	"github.com/lynxland/appkit"
	"github.com/lynxland/appkit/chain"
	grpcapiauthn "github.com/lynxland/grpcapi/authn"
	grpcapiinterceptor "github.com/lynxland/grpcapi/interceptor"
	grpcapipolicy "github.com/lynxland/grpcapi/policy"
	"google.golang.org/grpc"
)

func dummyStage(order int) grpcapiinterceptor.Stage {
	return grpcapiinterceptor.Stage{
		Name:  "test",
		Order: order,
		Unary: func(_ context.Context, req any, _ *grpc.UnaryServerInfo, h grpc.UnaryHandler) (any, error) {
			return h(context.Background(), req)
		},
	}
}

func TestDefaultChainNoStages(t *testing.T) {
	set, err := grpcapipolicy.NewSet()
	if err != nil {
		t.Fatalf("NewSet: %v", err)
	}
	ch, err := chain.DefaultChain(slog.Default(), set, &appkit.Aggregated{}, grpcapiauthn.Options{})
	if err != nil {
		t.Fatalf("DefaultChain: %v", err)
	}
	if ch != nil {
		t.Error("chain should be nil when no stages are contributed (server.New builds its own default chain)")
	}
	ch, err = chain.DefaultChain(slog.Default(), set, nil, grpcapiauthn.Options{})
	if err != nil || ch != nil {
		t.Errorf("nil aggregated should give (nil, nil), got (%v, %v)", ch, err)
	}
}

func TestDefaultChainWithStages(t *testing.T) {
	set, err := grpcapipolicy.NewSet()
	if err != nil {
		t.Fatalf("NewSet: %v", err)
	}
	agg := &appkit.Aggregated{Stages: []grpcapiinterceptor.Stage{dummyStage(50)}}
	ch, err := chain.DefaultChain(slog.Default(), set, agg, grpcapiauthn.Options{})
	if err != nil {
		t.Fatalf("DefaultChain: %v", err)
	}
	if ch == nil {
		t.Fatal("chain should be built when stages are contributed")
	}
}
