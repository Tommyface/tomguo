package source

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Policy 是一个源在适配层的执行策略。
type Policy struct {
	Timeout  time.Duration
	Attempts int
	Backoff  time.Duration
	Interval time.Duration
}

func DefaultPolicy() Policy {
	return Policy{
		Timeout:  20 * time.Second,
		Attempts: 2,
		Backoff:  600 * time.Millisecond,
		Interval: 0,
	}
}

func (p Policy) normalized() Policy {
	if p.Timeout <= 0 {
		p.Timeout = DefaultPolicy().Timeout
	}
	if p.Attempts <= 0 {
		p.Attempts = 1
	}
	if p.Backoff < 0 {
		p.Backoff = 0
	}
	if p.Interval < 0 {
		p.Interval = 0
	}
	return p
}

// ErrorKind 区分可重试与不可重试的错误。
type ErrorKind int

const (
	KindTransient ErrorKind = iota
	KindPermanent
)

// Error 是适配层对源错误的分类包装。
type Error struct {
	Kind ErrorKind
	Op   string
	Err  error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Op == "" {
		return e.Err.Error()
	}
	return e.Op + ": " + e.Err.Error()
}

func (e *Error) Unwrap() error { return e.Err }

func Transient(op string, err error) error {
	if err == nil {
		return nil
	}
	return &Error{Kind: KindTransient, Op: op, Err: err}
}

func Permanent(op string, err error) error {
	if err == nil {
		return nil
	}
	return &Error{Kind: KindPermanent, Op: op, Err: err}
}

// IsTransient 判断错误是否可重试。未包装的错误按可重试处理，
// 以便宿主底层错误（网络、超时）不被静默丢弃。
func IsTransient(err error) bool {
	if err == nil {
		return false
	}
	var target *Error
	if errors.As(err, &target) {
		return target.Kind == KindTransient
	}
	return true
}

// Health 记录一个源的调用结果，供清单健康度与下架建议使用。
type Health struct {
	Success   int       `json:"success"`
	Failure   int       `json:"failure"`
	LastError string    `json:"lastError,omitempty"`
	LastAt    time.Time `json:"lastAt,omitempty"`
}

// SuccessRate 返回成功率；没有调用记录时返回 0。
func (h Health) SuccessRate() float64 {
	total := h.Success + h.Failure
	if total == 0 {
		return 0
	}
	return float64(h.Success) / float64(total)
}

type limiter struct {
	mu       sync.Mutex
	next     time.Time
	interval time.Duration
}

func (l *limiter) wait(ctx context.Context) error {
	if l.interval <= 0 {
		return nil
	}
	l.mu.Lock()
	now := time.Now()
	slot := l.next
	if slot.Before(now) {
		slot = now
	}
	l.next = slot.Add(l.interval)
	l.mu.Unlock()
	delay := time.Until(slot)
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// invoke 执行一次带超时、限速与重试的调用。
func invoke[T any](ctx context.Context, policy Policy, limiter *limiter, op string, fn func(context.Context) (T, error)) (T, error) {
	var zero T
	policy = policy.normalized()
	var lastErr error
	for attempt := 1; attempt <= policy.Attempts; attempt++ {
		if err := limiter.wait(ctx); err != nil {
			return zero, err
		}
		callCtx, cancel := context.WithTimeout(ctx, policy.Timeout)
		value, err := fn(callCtx)
		cancel()
		if err == nil {
			return value, nil
		}
		lastErr = err
		if ctx.Err() != nil {
			return zero, err
		}
		if !IsTransient(err) || attempt == policy.Attempts {
			break
		}
		if policy.Backoff > 0 {
			timer := time.NewTimer(policy.Backoff * time.Duration(attempt))
			select {
			case <-ctx.Done():
				timer.Stop()
				return zero, lastErr
			case <-timer.C:
			}
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("%s: 无结果", op)
	}
	return zero, lastErr
}
