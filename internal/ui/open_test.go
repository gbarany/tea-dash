package ui

import (
	"reflect"
	"testing"
)

func TestBrowserCommandRejectsUnsafeURLs(t *testing.T) {
	for _, raw := range []string{"", "-aCalculator", "file:///tmp/page.html", "javascript:alert(1)", "data:text/html,test", "https:relative", "//example.com/path", "https:///missing-host", "https://example.com/\n"} {
		t.Run(raw, func(t *testing.T) {
			for _, platform := range []string{"darwin", "windows", "linux"} {
				if _, err := browserCommand(raw, platform); err == nil {
					t.Errorf("%s accepted %q", platform, raw)
				}
			}
		})
	}
}

func TestBrowserCommandPreservesHTTPURLAsOneArgument(t *testing.T) {
	raw := "https://example.com/pull/1?q=a%20b&next=$(ignored)"
	for _, tc := range []struct {
		platform string
		want     []string
	}{
		{"darwin", []string{"open", raw}},
		{"windows", []string{"rundll32", "url.dll,FileProtocolHandler", raw}},
		{"linux", []string{"xdg-open", raw}},
	} {
		t.Run(tc.platform, func(t *testing.T) {
			cmd, err := browserCommand(raw, tc.platform)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(cmd.Args, tc.want) {
				t.Errorf("Args = %#v, want %#v", cmd.Args, tc.want)
			}
		})
	}
	if _, err := browserCommand("http://localhost:3000/", "linux"); err != nil {
		t.Fatal(err)
	}
}
