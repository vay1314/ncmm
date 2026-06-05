package main

import (
	"github.com/3899/ncmm/internal/ncmm"
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
