package update

import "runtime"

// assets maps a platform to the release asset built for it.
//
// The names match what the README tells people to download by hand, and must
// keep matching: a manual download and an auto-update have to produce the same
// file, or "which version am I on?" stops having one answer.
//
// A platform absent from this map has no published asset. Returning empty
// disables the updater there rather than guessing — downloading the .exe onto
// a Mac would replace a working binary with one that cannot run, and the
// original would already have been renamed aside.
var assets = map[string]string{
	"windows/amd64": "proxytoolbox.exe",
	"darwin/arm64":  "proxytoolbox-mac-AppleSiliconCPU",
	"darwin/amd64":  "proxytoolbox-macOS-IntelCPU",
}

// assetFor returns the asset name for a GOOS/GOARCH pair, or "" if none is
// published.
func assetFor(goos, goarch string) string {
	return assets[goos+"/"+goarch]
}

// AssetName returns the asset this binary should install, or "" if the
// platform has no release build.
func AssetName() string {
	return assetFor(runtime.GOOS, runtime.GOARCH)
}
