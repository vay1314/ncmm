package main

import (
	"github.com/chaunsin/netease-cloud-music/internal/ncmm"
)

var (
	Version   = "1.0.0"
	Commit    = "none"
	BuildTime = "now"
)

func main() {
	c := ncmm.New()
	c.Version(Version, BuildTime, Commit)
	c.Execute()
}
