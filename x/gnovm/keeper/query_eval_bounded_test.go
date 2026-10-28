package keeper_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/gnolang/gno/gnovm/pkg/gnolang"
	"github.com/stretchr/testify/require"

	storetypes "cosmossdk.io/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	"github.com/sparkdream/gnovm/x/gnovm/keeper"
	"github.com/sparkdream/gnovm/x/gnovm/types"
)

// max_gas lowers the VM's query cap for one Eval: a cheap expression gives
// the same result as an unbounded Eval, an expensive one runs out of gas at
// the requested limit instead of the VM's 3B. This also fails if a gno
// upgrade moves the VM store the bounded path reads by reflection.
func TestQueryEvalBounded(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(&f.keeper)
	require.NoError(t, f.keeper.InitGenesis(f.ctx, types.GenesisState{Params: types.DefaultParams()}))

	creatorBytes := f.keeper.GetAuthority()
	creatorStr, err := f.addressCodec.BytesToString(creatorBytes)
	require.NoError(t, err)
	mpkg, err := ReadMemPackageFromDir(filepath.Join("testdata", "counter"))
	require.NoError(t, err)
	pkgBz, err := json.Marshal(mpkg)
	require.NoError(t, err)

	send, _ := sdk.ParseCoinsNormalized("1000stake")
	deposit, _ := sdk.ParseCoinsNormalized("2949stake")
	maxDeposit, _ := sdk.ParseCoinsNormalized("5000stake")
	f.authKeeper.EXPECT().GetAccount(f.ctx, creatorBytes).
		Return(authtypes.NewBaseAccountWithAddress(creatorBytes)).AnyTimes()
	f.bankKeeper.EXPECT().SendCoins(f.ctx, creatorBytes, gnolang.DerivePkgCryptoAddr(mpkg.Path).Bytes(), send)
	f.bankKeeper.EXPECT().SendCoins(f.ctx, creatorBytes, gnolang.DeriveStorageDepositCryptoAddr(mpkg.Path).Bytes(), deposit)
	_, err = ms.AddPackage(f.ctx, types.NewMsgAddPackage(creatorStr, send, maxDeposit, pkgBz))
	require.NoError(t, err)

	qs := keeper.NewQueryServerImpl(&f.keeper)
	queryCtx := sdk.UnwrapSDKContext(f.ctx).WithExecMode(sdk.ExecModeCheck).WithGasMeter(storetypes.NewInfiniteGasMeter())
	eval := func(expr string, maxGas uint64) (string, error) {
		res, err := qs.Eval(queryCtx, &types.QueryEvalRequest{PkgPath: mpkg.Path, Expr: expr, MaxGas: maxGas})
		if err != nil {
			return "", err
		}
		return res.Result, nil
	}

	unbounded, err := eval(`Render("")`, 0)
	require.NoError(t, err)
	bounded, err := eval(`Render("")`, 10_000_000)
	require.NoError(t, err)
	require.Equal(t, unbounded, bounded)
	require.Equal(t, `("0" string)`, bounded)

	const spin = `func() int { s := 0; for i := 0; i < 2000000; i++ { s += i }; return s }()`
	_, err = eval(spin, 10_000_000)
	require.ErrorContains(t, err, "out of gas")
	res, err := eval(spin, 0)
	require.NoError(t, err, "the VM's own cap still applies without max_gas")
	require.Contains(t, res, "1999999000000")

	// a value at or above the VM cap is ignored
	_, err = eval(spin, 3_000_000_000)
	require.NoError(t, err)

	// a large allocation is charged as gas, so max_gas bounds memory too
	_, err = eval(`len(make([]byte, 50000000))`, 10_000_000)
	require.ErrorContains(t, err, "out of gas")
}
