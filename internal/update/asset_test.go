package update

import (
	"runtime"
	"testing"
)

func TestAssetFor(t *testing.T) {
	tests := []struct {
		name         string
		goos, goarch string
		want         string
	}{
		{name: "windows", goos: "windows", goarch: "amd64", want: "proxytoolbox.exe"},
		{name: "apple silicon", goos: "darwin", goarch: "arm64", want: "proxytoolbox-mac-AppleSiliconCPU"},
		{name: "intel mac", goos: "darwin", goarch: "amd64", want: "proxytoolbox-macOS-IntelCPU"},

		// No asset is published for these, so the updater must disable itself
		// rather than download something for the wrong platform.
		{name: "linux", goos: "linux", goarch: "amd64", want: ""},
		{name: "windows arm", goos: "windows", goarch: "arm64", want: ""},
		{name: "freebsd", goos: "freebsd", goarch: "amd64", want: ""},
		{name: "empty", goos: "", goarch: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := assetFor(tt.goos, tt.goarch); got != tt.want {
				t.Errorf("assetFor(%q, %q) = %q, want %q", tt.goos, tt.goarch, got, tt.want)
			}
		})
	}
}

// TestAssetName_MatchesThisPlatform checks the wiring from runtime constants,
// which the table above cannot: assetFor could be correct while AssetName
// passes it the wrong arguments.
func TestAssetName_MatchesThisPlatform(t *testing.T) {
	want := assetFor(runtime.GOOS, runtime.GOARCH)
	if got := AssetName(); got != want {
		t.Errorf("AssetName() = %q, want %q for %s/%s", got, want, runtime.GOOS, runtime.GOARCH)
	}
}
