// Command nowrand prints testhooks.Now and 16 bytes from testhooks.Rand, one per line.
package main

import (
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/oritromax/lyceum/internal/testhooks"
)

func main() {
	b := make([]byte, 16)
	if _, err := io.ReadFull(testhooks.Rand(), b); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(testhooks.Now().Format(time.RFC3339Nano))
	fmt.Println(hex.EncodeToString(b))
}
