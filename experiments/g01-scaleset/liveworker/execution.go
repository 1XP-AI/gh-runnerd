package liveworker

import "context"

// PairedWorker is scoped to WithPairedExecution. All value copies share private
// lifetime state; it grants no authority against hostile Go callers.
type PairedWorker struct{ scope *pairedScope }
type pairedScope struct{}

// WithPairedExecution requires the caller's real controller lease/checker.
// The compiled contract starts fail-closed while its behavior is specified.
func (d *Driver) WithPairedExecution(ctx context.Context, in PairInput, checkController func(ControllerCheck) error, fn func(*PairedWorker) error) error {
	return ErrState
}
func (w *PairedWorker) Binding() PairBinding { return PairBinding{} }
func (w *PairedWorker) Bind(controllerIntent RecordRef) (PairReceipt, error) {
	return PairReceipt{}, ErrState
}
func (w *PairedWorker) Create(h HandoffReceipt, jit string) (ContainerReceipt, error) {
	return ContainerReceipt{}, ErrState
}
func (w *PairedWorker) Start(c ContainerReceipt, handoffResult RecordRef) (RecordRef, error) {
	return RecordRef{}, ErrState
}
func (w *PairedWorker) Observe() (LocalReceipt, error) { return LocalReceipt{}, ErrState }
func (w *PairedWorker) DeleteTerminal(d TerminalDecisionRef) (DeletionReceipt, error) {
	return DeletionReceipt{}, ErrState
}
