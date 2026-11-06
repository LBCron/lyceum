//go:build lyceumtest

package testhooks

import (
	crand "crypto/rand"
	"encoding/binary"
	"io"
	"math/rand/v2"
	"os"
	"strconv"
	"sync"
	"time"
)

// ActionPublished returns true when LYCEUM_ACTION_PUBLISHED is 1, false when it
// is set to anything else, else built.
func ActionPublished(built bool) bool {
	if v, ok := os.LookupEnv("LYCEUM_ACTION_PUBLISHED"); ok {
		return v == "1"
	}
	return built
}

// Now returns LYCEUM_NOW when set, else the current time in UTC.
func Now() time.Time {
	v := os.Getenv("LYCEUM_NOW")
	if v == "" {
		return time.Now().UTC()
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		panic("testhooks: LYCEUM_NOW is not RFC 3339: " + err.Error())
	}
	return t.UTC()
}

var (
	seedOnce sync.Once
	seeded   io.Reader // nil when LYCEUM_SEED is unset
)

// Rand returns a deterministic stream seeded from LYCEUM_SEED when set, else
// the cryptographic random source.
func Rand() io.Reader {
	seedOnce.Do(func() {
		v := os.Getenv("LYCEUM_SEED")
		if v == "" {
			return
		}
		n, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			panic("testhooks: LYCEUM_SEED is not an unsigned integer: " + err.Error())
		}
		var seed [32]byte
		binary.LittleEndian.PutUint64(seed[:], n)
		seeded = &lockedReader{r: rand.NewChaCha8(seed)}
	})
	if seeded == nil {
		return crand.Reader
	}
	return seeded
}

// lockedReader makes the seeded stream safe for concurrent use.
type lockedReader struct {
	mu sync.Mutex
	r  io.Reader
}

func (l *lockedReader) Read(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.r.Read(p)
}
