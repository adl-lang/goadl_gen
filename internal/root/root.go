package root

import (
	"encoding/json"
	"log"
	"os"
)

type RootObj struct {
	Cfg        string `help:"Config file in json format (NOTE file entries take precedence over command-line flags & env)" json:"-"`
	DumpConfig bool   `help:"Dump the config to stdout and exits" json:"-"`
}

func (rt RootObj) Config(in interface{}) {
	if rt.Cfg != "" {
		fd, err := os.Open(rt.Cfg)
		// config is in its own func
		// this defer fire correctly
		//
		// won't fire if dump is used as os.Exit terminates program
		defer func() {
			fd.Close()
		}()
		if err != nil {
			log.Fatalf("error opening file %s %v", rt.Cfg, err)
		}
		dec := json.NewDecoder(fd)
		err = dec.Decode(in)
		if err != nil {
			log.Fatalf("json error %v", err)
		}
	}
	if rt.DumpConfig {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		err := enc.Encode(in)
		if err != nil {
			log.Fatalf("json encoding error %v", err)
		}
		os.Exit(0)
	}
}
