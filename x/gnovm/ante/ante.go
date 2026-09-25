package ante

import (
	"fmt"

	storetypes "cosmossdk.io/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/sparkdream/gnovm/x/gnovm/types"
)

func NewAnteHandler() sdk.AnteDecorator {
	return &gnoAnteHandler{}
}

type gnoAnteHandler struct{}

func (gnoAnteHandler) AnteHandle(ctx sdk.Context, tx sdk.Tx, simulate bool, next sdk.AnteHandler) (sdk.Context, error) {
	msgs := tx.GetMsgs()

	gnoVMCount := 0
	nonGnoVMCount := 0

	for _, msg := range msgs {
		switch {
		case sdk.MsgTypeURL(msg) == sdk.MsgTypeURL(&types.MsgRun{}) ||
			sdk.MsgTypeURL(msg) == sdk.MsgTypeURL(&types.MsgAddPackage{}) ||
			sdk.MsgTypeURL(msg) == sdk.MsgTypeURL(&types.MsgCall{}):
			gnoVMCount++
		default:
			nonGnoVMCount++
		}
	}

	// Reject transactions that mix GnoVM and non-GnoVM messages
	if gnoVMCount > 0 && nonGnoVMCount > 0 {
		return ctx, fmt.Errorf("cannot mix GnoVM messages with non-GnoVM messages in the same transaction")
	}

	// Pass ctx on unchanged unless a Gno simulation needs the infinite
	// meter. It must never be a zero sdk.Context: baseapp treats a zero
	// context returned by the ante chain as "keep the pre-ante one", which
	// drops the gas meter SetUpContextDecorator installed (so message
	// execution ran unbounded by the tx's gas limit, and gas_wanted
	// reported the infinite meter's limit) along with every value any
	// decorator set.
	if gnoVMCount > 0 && simulate {
		ctx = ctx.WithGasMeter(storetypes.NewInfiniteGasMeter())
	}

	return next(ctx, tx, simulate)
}
