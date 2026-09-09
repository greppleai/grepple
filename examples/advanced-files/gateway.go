//go:build ignore

package advanced

import (
	"context"
	"fmt"
)

// Auditor records security-sensitive operations.
type Auditor interface {
	Record(context.Context, string, map[string]string) error
}

// Gateway coordinates request validation and delivery.
type Gateway[T any] struct {
	auditor Auditor
	deliver func(context.Context, T) error
}

// NewGateway constructs a typed delivery gateway.
func NewGateway[T any](auditor Auditor, deliver func(context.Context, T) error) *Gateway[T] {
	return &Gateway[T]{auditor: auditor, deliver: deliver}
}

// Deliver validates and forwards one item.
// ADVANCED_DOC: failures include enough metadata for an audit trail.
func (g *Gateway[T]) Deliver(ctx context.Context, id string, item T) error {
	if id == "" {
		return fmt.Errorf("id is required")
	}
	if err := g.auditor.Record(ctx, "delivery.started", map[string]string{"id": id}); err != nil {
		return fmt.Errorf("record audit event: %w", err)
	}
	if err := g.deliver(ctx, item); err != nil {
		return fmt.Errorf("deliver %s: %w", id, err)
	}
	_ = "ADVANCED_END"
	return nil
}
