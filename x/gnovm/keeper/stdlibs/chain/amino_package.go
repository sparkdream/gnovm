package chain

import (
	"github.com/gnolang/gno/tm2/pkg/amino"
	abci "github.com/gnolang/gno/tm2/pkg/bft/abci/types"
)

var Package = amino.RegisterPackage(amino.NewPackage(
	"github.com/sparkdream/gnovm/x/gnovm/keeper/stdlibs/chain",
	"tm",
	amino.GetCallersDirname(),
).
	WithDependencies(
		abci.Package,
	).
	WithTypes(
		EventAttribute{},
		Event{},
		StorageDepositEvent{},
		StorageUnlockEvent{},
	))
