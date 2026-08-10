package quorum

import (
	"errors"
	"fmt"
)

// Config holds N/R/W quorum thresholds.
type Config struct {
	N int // replication factor (placement set size)
	W int // write quorum
	R int // read quorum
}

// DefaultsForN returns W=2, R=2 when N>=3, otherwise W=N, R=N.
func DefaultsForN(n int) (w, r int) {
	if n >= 3 {
		return 2, 2
	}
	return n, n
}

// Validate checks quorum configuration for the default overlap mode.
func (c Config) Validate() error {
	if c.N < 1 {
		return errors.New("replication factor N must be >= 1")
	}
	if c.W < 1 || c.W > c.N {
		return fmt.Errorf("write quorum W=%d must satisfy 1 <= W <= N=%d", c.W, c.N)
	}
	if c.R < 1 || c.R > c.N {
		return fmt.Errorf("read quorum R=%d must satisfy 1 <= R <= N=%d", c.R, c.N)
	}
	if c.W+c.R <= c.N {
		return fmt.Errorf("quorum overlap requires W+R > N (got W=%d R=%d N=%d)", c.W, c.R, c.N)
	}
	return nil
}

// WriteSatisfied reports whether enough write acknowledgements arrived.
func (c Config) WriteSatisfied(acks int) bool {
	return acks >= c.W
}

// ReadSatisfied reports whether enough read responses arrived.
func (c Config) ReadSatisfied(responses int) bool {
	return responses >= c.R
}

// Enabled reports whether quorum semantics apply (multi-replica).
func (c Config) Enabled() bool {
	return c.N > 1
}
