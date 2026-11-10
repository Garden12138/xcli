//go:build darwin || linux

package modelstore

import (
	"github.com/Garden12138/xcli/internal/localstate"
	"os"
)

func Lock(p string, w bool) (*os.File, error) { return localstate.Lock(p, w) }
