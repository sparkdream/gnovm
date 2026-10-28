package keeper

import (
	goerrors "errors"
	"fmt"
	"reflect"
	"strings"
	"unsafe"

	"github.com/gnolang/gno/gno.land/pkg/sdk/vm"
	gno "github.com/gnolang/gno/gnovm/pkg/gnolang"
	"github.com/gnolang/gno/gnovm/stdlibs"
	"github.com/gnolang/gno/tm2/pkg/errors"
	gnosdk "github.com/gnolang/gno/tm2/pkg/sdk"
	"github.com/gnolang/gno/tm2/pkg/store"
	stypes "github.com/gnolang/gno/tm2/pkg/store/types"
)

// Query caps fixed inside vm.VMKeeper (gno.land/pkg/sdk/vm/keeper.go).
const (
	vmMaxGasQuery   = 3_000_000_000
	vmMaxAllocQuery = 1_500_000_000
)

// chainDomainParamPath and chainDomainDefault mirror the unexported VM
// param read by vm.VMKeeper.getChainDomainParam.
const (
	chainDomainParamPath = "vm:p:chain_domain"
	chainDomainDefault   = "gno.land"
)

// queryEvalBounded is vm.VMKeeper.QueryEval run under a caller-chosen gas
// limit instead of the VM's fixed 3B. The VM charges one gas per byte of
// peak allocation, so the limit bounds memory as well as computation.
//
// It mirrors vm.VMKeeper.queryEvalInternal line for line. The VM keeps its
// package store unexported and offers no way to lower the query caps, so the
// store is read by reflection (see vmGnoStore); TestQueryEvalBounded fails if
// a gno upgrade moves it.
func (k *Keeper) queryEvalBounded(ctx gnosdk.Context, pkgPath, expr string, maxGas int64) (res string, err error) {
	gnoStore, err := vmGnoStore(k.VMKeeper)
	if err != nil {
		return "", err
	}

	ctx = ctx.WithGasMeter(store.NewGasMeter(maxGas))
	alloc := gno.NewAllocator(vmMaxAllocQuery)
	// throwaway (never committed)
	gnostore := gnoStore.BeginTransaction(ctx.Store(k.storeKey), ctx.Store(k.memStoreKey), ctx.GasMeter())
	pv := gnostore.GetPackage(pkgPath, false)
	if pv == nil {
		return "", vm.ErrInvalidPkgPath(fmt.Sprintf("package not found: %s", pkgPath))
	}
	chainDomain := chainDomainDefault
	k.vmParams.GetString(ctx, chainDomainParamPath, &chainDomain)
	msgCtx := stdlibs.ExecContext{
		ChainID:     ctx.ChainID(),
		ChainDomain: chainDomain,
		Height:      ctx.BlockHeight(),
		Timestamp:   ctx.BlockTime().Unix(),
		Banker:      vm.NewSDKBanker(k.VMKeeper, ctx), // safe as long as ctx is a fork to be discarded.
		Params:      vm.NewSDKParams(k.vmParams, ctx),
		EventLogger: ctx.EventLogger(),
	}
	m := gno.NewMachineWithOptions(
		gno.MachineOptions{
			PkgPath:  pkgPath,
			Output:   k.VMKeeper.Output,
			Store:    gnostore,
			Context:  msgCtx,
			Alloc:    alloc,
			GasMeter: ctx.GasMeter(),
		})
	defer m.Release()
	defer recoverQuery(m, &err)
	xx, err := m.ParseExpr(expr)
	if err != nil {
		return "", err
	}
	rtvs := m.Eval(xx)
	parts := make([]string, len(rtvs))
	for i, rtv := range rtvs {
		parts[i] = rtv.String()
	}
	return strings.Join(parts, "\n"), nil
}

// vmGnoStore returns the VM's unexported package store.
func vmGnoStore(vmk *vm.VMKeeper) (gno.Store, error) {
	if vmk == nil {
		return nil, goerrors.New("gno VM is not initialized")
	}
	f := reflect.ValueOf(vmk).Elem().FieldByName("gnoStore")
	if !f.IsValid() {
		return nil, goerrors.New("gno VM has no gnoStore field")
	}
	s, ok := reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem().Interface().(gno.Store)
	if !ok || s == nil {
		return nil, goerrors.New("gno VM store is not initialized")
	}
	return s, nil
}

// recoverQuery mirrors the VM's doRecoverQuery: out of gas and VM panics
// become errors.
func recoverQuery(m *gno.Machine, e *error) {
	r := recover()
	if r == nil {
		return
	}
	if err, ok := r.(error); ok {
		var oog stypes.OutOfGasError
		if goerrors.As(err, &oog) {
			*e = oog
			return
		}
		var up gno.UnhandledPanicError
		if goerrors.As(err, &up) {
			*e = errors.Wrapf(
				errors.New(up.Descriptor),
				"VM panic: %s\nStacktrace:\n%s\n",
				up.Descriptor, m.ExceptionStacktrace(),
			)
			return
		}
	}
	*e = errors.Wrapf(
		fmt.Errorf("%v", r),
		"VM panic: %v\nStacktrace:\n%s\n",
		r, m.Stacktrace().String(),
	)
}
