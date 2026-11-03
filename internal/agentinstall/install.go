// Package agentinstall provides the self-contained Linux Agent installer.
package agentinstall

import _ "embed"

//go:embed install.sh
var installer []byte

// Script returns a separate copy so callers cannot change the embedded installer.
func Script() []byte { return append([]byte(nil), installer...) }
