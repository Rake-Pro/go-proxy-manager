package ui

import (
	"strings"
	"testing"
)

// The stream-host editor renders no control for the optional TCP and UDP
// limits, and every save is a whole-object PUT, so the builder must carry each
// of them forward from the loaded object or a UI save wipes it.
func TestStreamEditorCarriesLimitsForward(t *testing.T) {
	js := loadAppJS(t)
	for _, want := range []string{
		"if (o.idleTimeout) body.idleTimeout = o.idleTimeout;",
		"if (o.maxConnsPerSource) body.maxConnsPerSource = o.maxConnsPerSource;",
		"if (proto !== 'tcp' && o.newSessionsPerMinute) body.newSessionsPerMinute = o.newSessionsPerMinute;",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("the stream-host save no longer carries a limit forward: missing %q", want)
		}
	}
}
