package lib

import (
	"strings"
	"testing"
)

func TestShouldOfferUpdate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		current string
		latest  string
		want    bool
	}{
		{name: "same version", current: "v0.1.2", latest: "v0.1.2", want: false},
		{name: "newer semver", current: "v0.1.2", latest: "v0.1.3", want: true},
		{name: "older latest", current: "v0.1.3", latest: "v0.1.2", want: false},
		{name: "non semver current build", current: "20260401-dev", latest: "v0.1.3", want: false},
		{name: "non semver fallback", current: "v-next", latest: "v-next-2", want: true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := ShouldOfferUpdate(tc.current, tc.latest); got != tc.want {
				t.Fatalf("ShouldOfferUpdate(%q, %q) = %v, want %v", tc.current, tc.latest, got, tc.want)
			}
		})
	}
}

func TestSelectReleaseAsset(t *testing.T) {
	t.Parallel()

	release := &ReleaseInfo{
		Assets: []ReleaseAsset{
			{Name: "readcli-darwin-amd64-v0.1.2.tar.gz"},
			{Name: "readcli-darwin-arm64-v0.1.2.tar.gz"},
			{Name: "readcli-linux-amd64-v0.1.2.tar.gz"},
		},
	}

	asset := SelectReleaseAsset(release, "darwin", "arm64")
	if asset == nil || asset.Name != "readcli-darwin-arm64-v0.1.2.tar.gz" {
		t.Fatalf("unexpected asset: %+v", asset)
	}

	if missing := SelectReleaseAsset(release, "linux", "arm64"); missing != nil {
		t.Fatalf("expected no asset, got %+v", missing)
	}
}

func TestUpdateInstallErrorIncludesTempDir(t *testing.T) {
	t.Parallel()

	err := (&UpdateInstallError{
		Message: "覆盖当前二进制失败",
		TempDir: "/tmp/readcli-update-123",
	}).Error()

	if !strings.Contains(err, "/tmp/readcli-update-123") {
		t.Fatalf("error string does not include temp dir: %q", err)
	}
}
