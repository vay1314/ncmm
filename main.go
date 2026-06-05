package main

import (
	"github.com/chaunsin/netease-cloud-music/internal/ncmctl"
)

var (
	Version   = "1.0.0"
	Commit    = "none"
	BuildTime = "now"
)

func main() {
	c := ncmctl.New()
	c.Version(Version, BuildTime, Commit)
	c.Execute()
}
