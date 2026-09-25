package ante_test

import (
	"testing"

	storetypes "cosmossdk.io/store/types"
	"github.com/cosmos/cosmos-sdk/testutil"
	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/stretchr/testify/require"
	protov2 "google.golang.org/protobuf/proto"

	"github.com/sparkdream/gnovm/x/gnovm/ante"
	"github.com/sparkdream/gnovm/x/gnovm/types"
)

type fakeTx struct{ msgs []sdk.Msg }

func (t fakeTx) GetMsgs() []sdk.Msg                    { return t.msgs }
func (t fakeTx) GetMsgsV2() ([]protov2.Message, error) { return nil, nil }

type ctxKey struct{}

// run passes tx through the decorator from a context that carries a
// limited gas meter and a value, and returns the context next received.
func run(t *testing.T, tx sdk.Tx, simulate bool) (sdk.Context, sdk.Context, error) {
	t.Helper()
	key := storetypes.NewKVStoreKey("gnovm_ante_" + t.Name())
	ctx := testutil.DefaultContext(key, storetypes.NewTransientStoreKey("t_"+t.Name())).
		WithGasMeter(storetypes.NewGasMeter(200_000)).
		WithValue(ctxKey{}, "set by an earlier decorator")
	var got sdk.Context
	_, err := ante.NewAnteHandler().AnteHandle(ctx, tx, simulate, func(c sdk.Context, _ sdk.Tx, _ bool) (sdk.Context, error) {
		got = c
		return c, nil
	})
	return ctx, got, err
}

// A non-Gno tx must reach the rest of the chain with the context it came
// in with. Handing next a zero context made baseapp fall back to its
// pre-ante context: the tx's gas meter (and every ante-set value) was lost,
// so message execution ran unbounded by the tx's gas limit.
func TestNonGnoTxKeepsItsContext(t *testing.T) {
	for _, simulate := range []bool{false, true} {
		in, got, err := run(t, fakeTx{msgs: []sdk.Msg{&banktypes.MsgSend{}}}, simulate)
		require.NoError(t, err)
		require.False(t, got.IsZero())
		require.Equal(t, uint64(200_000), got.GasMeter().Limit())
		require.Same(t, in.GasMeter(), got.GasMeter())
		require.Equal(t, "set by an earlier decorator", got.Value(ctxKey{}))
	}
}

// A Gno tx keeps the tx's gas meter when executed; only a simulation runs
// on an infinite one.
func TestGnoTxGasMeter(t *testing.T) {
	tx := fakeTx{msgs: []sdk.Msg{&types.MsgCall{}}}

	in, got, err := run(t, tx, false)
	require.NoError(t, err)
	require.Same(t, in.GasMeter(), got.GasMeter())
	require.Equal(t, "set by an earlier decorator", got.Value(ctxKey{}))

	_, got, err = run(t, tx, true)
	require.NoError(t, err)
	require.False(t, got.IsZero())
	require.Equal(t, uint64(storetypes.Gas(1<<64-1)), got.GasMeter().Limit())
	require.Equal(t, "set by an earlier decorator", got.Value(ctxKey{}))
}

func TestMixedTxRejected(t *testing.T) {
	_, got, err := run(t, fakeTx{msgs: []sdk.Msg{&types.MsgCall{}, &banktypes.MsgSend{}}}, false)
	require.ErrorContains(t, err, "cannot mix GnoVM messages")
	require.True(t, got.IsZero()) // next never ran
}
