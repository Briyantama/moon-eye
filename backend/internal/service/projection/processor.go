package projection

import "context"

// Processor applies a single change event to a read model. Implementations must be idempotent.
type Processor interface {
	Process(ctx context.Context, event ChangeEventRow) error
}
