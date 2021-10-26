package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/jdeng/goheif"
	"github.com/rwcarlsen/goexif/exif"
)

func main() {
	flag.Usage = func() {
		os.Stderr.WriteString(`fix-image-time dir1 [dir2 dir3...]

Print out all files in the directory that also exist in any other of the
directories.
`)
	}
	flag.Parse()
	if flag.NArg() < 1 {
		os.Stderr.WriteString("please provide at least one directory.\n\n")
		flag.Usage()
		os.Exit(2)
	}
	for i := range flag.Args() {
		dir := flag.Arg(i)
		count := 0
		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			if count > 100 {
				return fs.SkipDir
			}
			ext := filepath.Ext(path)
			switch strings.ToLower(ext) {
			case ".heic":
				fi, err := os.Open(path)
				if err != nil {
					return err
				}
				exifBytes, err := goheif.ExtractExif(fi)
				if err != nil {
					return err
				}
				exif, err := exif.Decode(bytes.NewReader(exifBytes))
				if err != nil {
					return err
				}
				tag, err := exif.DateTime()
				if err != nil {
					return err
				}
				statT, ok := info.Sys().(*syscall.Stat_t)
				if !ok {
					return fmt.Errorf("could not cast %#v to Stat_t", info.Sys())
				}
				ctime := time.Unix(int64(statT.Ctimespec.Sec), int64(statT.Ctimespec.Nsec))
				fmt.Printf("%s created at %v\n", path, ctime)
				fmt.Printf("exif dt: %v\n", tag)
				if err := fi.Close(); err != nil {
					return err
				}
				count++
			}
			if strings.ToLower(ext) != ".jpg" {
				return nil
			}
			return nil
		})
		if err != nil {
			log.Fatal(err)
		}
	}
}
