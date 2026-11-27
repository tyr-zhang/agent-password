//go:build darwin

package platform

import (
	"strings"
	"testing"
)

func TestLaunchAgentRunsForegroundDaemon(t *testing.T) {
	plist := launchAgentPlist("com.tyrzhang.agent-pwd", "/tmp/agent-pwd", "/tmp/agent-pwd.log")
	if !strings.Contains(plist, "<string>--foreground</string>") {
		t.Fatalf("launchd plist would detach and be restarted in a loop:\n%s", plist)
	}
	if !strings.Contains(plist, "<key>KeepAlive</key><true/>") {
		t.Fatal("launchd plist lost KeepAlive")
	}
}
