package worker

import (
	"context"
	"errors"
	"time"

	"github.com/earl/umbrel-dropbox-sync/internal/state"
)

const (
	DefaultMaxAttempts = 5
	DefaultBaseBackoff = time.Second
	DefaultMaxBackoff  = time.Minute
)

type Handler interface {
	HandleOp(context.Context, state.PendingOp) error
}

type HandlerFunc func(context.Context, state.PendingOp) error

func (f HandlerFunc) HandleOp(ctx context.Context, op state.PendingOp) error {
	return f(ctx, op)
}

type RetryAfter interface {
	RetryAfter() time.Duration
}

type RetryAfterError struct {
	After time.Duration
	Err   error
}

func (e RetryAfterError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return "retry after"
}

func (e RetryAfterError) Unwrap() error {
	return e.Err
}

func (e RetryAfterError) RetryAfter() time.Duration {
	return e.After
}

type Processor struct {
	Store       *state.Store
	Handler     Handler
	MaxAttempts int
	BaseBackoff time.Duration
	MaxBackoff  time.Duration
	Now         func() time.Time
}

type Result struct {
	Processed bool
	OpID      int64
	Completed bool
	Failed    bool
	RetryAt   time.Time
	Err       error
}

func (p Processor) ProcessOne(ctx context.Context) (Result, error) {
	if p.Store == nil {
		return Result{}, errors.New("worker processor missing store")
	}
	if p.Handler == nil {
		return Result{}, errors.New("worker processor missing handler")
	}
	now := p.now()
	op, err := p.Store.NextReadyPendingOp(now)
	if err != nil {
		return Result{}, err
	}
	if op == nil {
		return Result{}, nil
	}

	handleErr := p.Handler.HandleOp(ctx, *op)
	if handleErr == nil {
		if err := p.Store.CompleteOp(op.ID); err != nil {
			return Result{}, err
		}
		return Result{Processed: true, OpID: op.ID, Completed: true}, nil
	}

	attempt := op.Attempts + 1
	if attempt >= p.maxAttempts() {
		if err := p.Store.FailOp(op.ID, handleErr.Error()); err != nil {
			return Result{}, err
		}
		return Result{Processed: true, OpID: op.ID, Failed: true, Err: handleErr}, nil
	}

	retryAt := now.Add(p.retryDelay(op.Attempts, handleErr))
	if err := p.Store.RetryOp(op.ID, retryAt, handleErr.Error()); err != nil {
		return Result{}, err
	}
	return Result{Processed: true, OpID: op.ID, RetryAt: retryAt, Err: handleErr}, nil
}

func (p Processor) now() time.Time {
	if p.Now != nil {
		return p.Now().UTC()
	}
	return time.Now().UTC()
}

func (p Processor) maxAttempts() int {
	if p.MaxAttempts > 0 {
		return p.MaxAttempts
	}
	return DefaultMaxAttempts
}

func (p Processor) baseBackoff() time.Duration {
	if p.BaseBackoff > 0 {
		return p.BaseBackoff
	}
	return DefaultBaseBackoff
}

func (p Processor) maxBackoff() time.Duration {
	if p.MaxBackoff > 0 {
		return p.MaxBackoff
	}
	return DefaultMaxBackoff
}

func (p Processor) retryDelay(previousAttempts int, err error) time.Duration {
	var retryAfter RetryAfter
	if errors.As(err, &retryAfter) && retryAfter.RetryAfter() > 0 {
		return retryAfter.RetryAfter()
	}

	delay := p.baseBackoff()
	for i := 0; i < previousAttempts; i++ {
		delay *= 2
		if delay >= p.maxBackoff() {
			return p.maxBackoff()
		}
	}
	if delay > p.maxBackoff() {
		return p.maxBackoff()
	}
	return delay
}
