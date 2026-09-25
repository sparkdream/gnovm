package keeper_test

import (
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"

	"cosmossdk.io/log"
	storetypes "cosmossdk.io/store/types"
	addresscodec "github.com/cosmos/cosmos-sdk/codec/address"
	"github.com/cosmos/cosmos-sdk/testutil"
	sdk "github.com/cosmos/cosmos-sdk/types"
	moduletestutil "github.com/cosmos/cosmos-sdk/types/module/testutil"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	"github.com/sparkdream/gnovm/x/gnovm/keeper"
	module "github.com/sparkdream/gnovm/x/gnovm/module"
	"github.com/sparkdream/gnovm/x/gnovm/types"
)

// A node builds its VM on the first Gno tx after it starts, re-reading every
// stored package. That must not be charged to the tx: only a restarted node
// pays it (so gas would differ across validators), and a tx gas limit running
// out inside gno's package-iteration goroutine panics where nothing can
// recover it, taking the node down.
func TestVMInitializationIsNotChargedToTheTx(t *testing.T) {
	encCfg := moduletestutil.MakeTestEncodingConfig(module.AppModule{})
	addressCodec := addresscodec.NewBech32Codec(sdk.GetConfig().GetBech32AccountAddrPrefix())
	storeKey := storetypes.NewKVStoreKey("gnovm")
	memStoreKey := storetypes.NewMemoryStoreKey("memory:gnovm")
	ctx := testutil.DefaultContextWithDB(t, storeKey, storetypes.NewTransientStoreKey("transient_test")).Ctx.
		WithChainID("gnovm-test")
	newKeeper := func() keeper.Keeper {
		ctrl := gomock.NewController(t)
		return keeper.NewKeeper(
			log.NewTestLogger(t), storeKey, memStoreKey, encCfg.Codec, addressCodec,
			authtypes.NewModuleAddress(types.GovModuleName), NewMockAuthKeeper(ctrl), NewMockBankKeeper(ctrl),
		)
	}

	// genesis stores the stdlibs
	first := newKeeper()
	require.NoError(t, first.InitGenesis(ctx, types.GenesisState{Params: types.DefaultParams()}))

	// a restarted node: a fresh keeper over the same store, first Gno tx with
	// a gas limit far below what re-reading the packages costs
	restarted := newKeeper()
	meter := storetypes.NewGasMeter(10_000)
	txCtx := ctx.WithGasMeter(meter).WithExecMode(sdk.ExecModeFinalize)
	require.NotPanics(t, func() {
		_, err := restarted.BuildGnoContext(txCtx)
		require.NoError(t, err)
	})
	require.Zero(t, meter.GasConsumed(), "VM initialization was charged to the tx")
}
